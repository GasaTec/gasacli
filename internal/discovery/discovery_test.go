package discovery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestDefaultCatalog(t *testing.T) {
	catalog := DefaultCatalog()
	var ids []string
	for _, a := range catalog {
		ids = append(ids, a.ID)
	}
	if want := []string{"claude", "codex", "gemini", "cursor-agent", "opencode", "orca", "gh"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("catalog IDs = %v, want %v", ids, want)
	}
	if !reflect.DeepEqual(catalog[3].Binaries, []string{"cursor-agent", "cursor"}) {
		t.Fatal("missing cursor fallback")
	}
	catalog[0].Binaries[0] = "changed"
	if DefaultCatalog()[0].Binaries[0] != "claude" {
		t.Fatal("catalog shares mutable state")
	}
}

func TestNotFoundNeverRuns(t *testing.T) {
	results := Discover(context.Background(), DefaultCatalog(), Deps{
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Error("Run called for missing binary")
			return nil, nil
		},
	})
	for _, r := range results {
		if r.Status != NotFound || r.Path != "" || r.Note != authNote {
			t.Errorf("unexpected missing result: %+v", r)
		}
	}
}

func TestAvailableAndCursorFallback(t *testing.T) {
	var looked []string
	results := Discover(context.Background(), []Agent{DefaultCatalog()[3]}, Deps{
		LookPath: func(name string) (string, error) {
			looked = append(looked, name)
			if name == "cursor-agent" {
				return "", exec.ErrNotFound
			}
			return filepath.Join("tools", "cursor"), nil
		},
		Run: func(ctx context.Context, path string, args ...string) ([]byte, error) {
			if !filepath.IsAbs(path) || !reflect.DeepEqual(args, []string{"--version"}) {
				t.Errorf("unexpected invocation: %s %v", path, args)
			}
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) > probeTimeout {
				t.Error("missing per-probe deadline")
			}
			return []byte("Cursor 1.2.3\nmore details"), nil
		},
	})
	r := results[0]
	if r.Status != Available || r.Version != "Cursor 1.2.3" || r.Note != authNote || !reflect.DeepEqual(looked, []string{"cursor-agent", "cursor"}) {
		t.Fatalf("unexpected result: %+v; lookups %v", r, looked)
	}
}

func TestProbeFailuresAreSafe(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		err    error
		want   string
	}{
		{"failure", "secret stdout", errors.New("secret stderr TOKEN=private"), "version probe failed"},
		{"deadline", "", context.DeadlineExceeded, "version probe timed out"},
		{"canceled", "", context.Canceled, "version probe canceled"},
		{"empty", "\x1b[31m\x00", nil, "version output empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := Discover(context.Background(), DefaultCatalog()[:1], Deps{
				LookPath: func(string) (string, error) { return "agent", nil },
				Run:      func(context.Context, string, ...string) ([]byte, error) { return []byte(tc.output), tc.err },
			})[0]
			if r.Status != VersionUnknown || r.Err != tc.want || r.Version != "" {
				t.Fatalf("unexpected result: %+v", r)
			}
		})
	}
}

func TestLookupErrorAndInvalidPath(t *testing.T) {
	for _, tc := range []struct {
		path string
		err  error
		want string
	}{
		{"", errors.New("secret env"), "binary lookup failed"},
		{"", nil, "invalid binary path"},
	} {
		r := Discover(context.Background(), DefaultCatalog()[:1], Deps{
			LookPath: func(string) (string, error) { return tc.path, tc.err },
			Run:      func(context.Context, string, ...string) ([]byte, error) { t.Error("unexpected run"); return nil, nil },
		})[0]
		if r.Status != Error || r.Err != tc.want {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
}

func TestSanitizedVersion(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"\x1b[31magent\x1b[0m\x00\t 1.0\r\nignored", "agent 1.0"},
		{"\x1b]0;hidden title\aagent\u202e 2", "agent 2"},
		{"\x1b]hidden\x1b\\agent", "agent"},
		{"\u009b31magent\u009b0m", "agent"},
		{strings.Repeat("界", 300), strings.Repeat("界", 200)},
		{"\nsecond line", ""},
	} {
		r := Discover(context.Background(), DefaultCatalog()[:1], Deps{
			LookPath: func(string) (string, error) { return "agent", nil },
			Run:      func(context.Context, string, ...string) ([]byte, error) { return []byte(tc.input), nil },
		})[0]
		if r.Version != tc.want || utf8.RuneCountInString(r.Version) > versionLimit {
			t.Errorf("version = %q, want %q", r.Version, tc.want)
		}
	}
}

func TestTimeout(t *testing.T) {
	start := time.Now()
	r := Discover(context.Background(), DefaultCatalog()[:1], Deps{
		LookPath: func(string) (string, error) { return "agent", nil },
		Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	})[0]
	if r.Status != VersionUnknown || r.Err != "version probe timed out" || time.Since(start) > 5*time.Second {
		t.Fatalf("timeout failed: %+v, duration %v", r, time.Since(start))
	}
}

func TestConcurrencyCancellationAndOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	catalog := make([]Agent, 20)
	for i := range catalog {
		catalog[i] = Agent{ID: fmt.Sprint(i), Binaries: []string{"agent"}}
	}
	var active, maximum, calls atomic.Int32
	started := make(chan struct{}, workerLimit)
	done := make(chan []Result, 1)
	go func() {
		done <- Discover(ctx, catalog, Deps{
			LookPath: func(string) (string, error) { return "agent", nil },
			Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
				calls.Add(1)
				n := active.Add(1)
				defer active.Add(-1)
				for old := maximum.Load(); n > old; old = maximum.Load() {
					if maximum.CompareAndSwap(old, n) {
						break
					}
				}
				started <- struct{}{}
				<-ctx.Done()
				return nil, ctx.Err()
			},
		})
	}()
	for i := 0; i < workerLimit; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("workers did not start concurrently")
		}
	}
	cancel()
	select {
	case results := <-done:
		if len(results) != len(catalog) || maximum.Load() != workerLimit || calls.Load() != workerLimit || active.Load() != 0 {
			t.Fatalf("unexpected concurrency: max=%d calls=%d active=%d", maximum.Load(), calls.Load(), active.Load())
		}
		for i, r := range results {
			if r.ID != catalog[i].ID || r.Err == "" {
				t.Fatalf("order/cancellation failed at %d: %+v", i, r)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("discovery did not respect cancellation")
	}
}

func TestAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := Discover(ctx, DefaultCatalog(), Deps{
		LookPath: func(string) (string, error) { t.Error("lookup after cancellation"); return "", nil },
	})
	for _, r := range results {
		if r.Status != Error || r.Err != "discovery canceled" {
			t.Fatalf("unexpected result: %+v", r)
		}
	}
}

func TestStableOrderWhenProbesFinishOutOfOrder(t *testing.T) {
	secondFinished := make(chan struct{})
	catalog := []Agent{
		{ID: "first", Binaries: []string{"first"}, VersionFlag: "-v"},
		{ID: "second", Binaries: []string{"second"}},
	}
	results := Discover(context.Background(), catalog, Deps{
		LookPath: func(name string) (string, error) { return name, nil },
		Run: func(ctx context.Context, path string, args ...string) ([]byte, error) {
			if filepath.Base(path) == "first" {
				if !reflect.DeepEqual(args, []string{"-v"}) {
					t.Error("custom version flag ignored")
				}
				select {
				case <-secondFinished:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			} else {
				close(secondFinished)
			}
			return []byte(filepath.Base(path)), nil
		},
	})
	for i, r := range results {
		if r.ID != catalog[i].ID || r.Version != catalog[i].ID || r.Status != Available {
			t.Fatalf("unstable order at %d: %+v", i, r)
		}
	}
	if got := Discover(context.Background(), nil, Deps{}); len(got) != 0 {
		t.Fatal("nonempty result for empty catalog")
	}
}

func TestFormat(t *testing.T) {
	var out bytes.Buffer
	err := Format(&out, []Result{
		{Name: "Claude Code", Status: Available, Version: "1.0", Path: "/tools/claude"},
		{Name: "Codex", Status: NotFound, Err: "\x1b[31merror\nINJECTED"},
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 || strings.Index(lines[0], "available") != strings.Index(lines[1], "not_found") || strings.Count(out.String(), authNote) != 2 || strings.ContainsAny(out.String(), "\x1b\t") || strings.Contains(out.String(), "INJECTED") {
		t.Fatalf("unexpected format: %q", out.String())
	}
	if err := Format(failingWriter{}, []Result{{Name: "agent"}}); err == nil {
		t.Fatal("writer error swallowed")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestProbeProcess is a real executable fixture without shell scripts. Its mode
// is selected by fixed test arguments because the runner strips custom env vars.
func TestProbeProcess(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "--discovery-fixture" || i+1 >= len(os.Args) {
			continue
		}
		switch os.Args[i+1] {
		case "output":
			fmt.Fprint(os.Stdout, strings.Repeat("v", outputLimit*8))
			fmt.Fprint(os.Stderr, "SECRET_STDERR")
		case "env":
			fmt.Fprint(os.Stdout, os.Getenv("DISCOVERY_TEST_SECRET"))
		case "wait":
			time.Sleep(10 * time.Second)
		}
		os.Exit(0)
	}
}

func TestDefaultRunnerBoundedOutputAndEnvironment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISCOVERY_TEST_SECRET", "PRIVATE_TOKEN")
	for _, mode := range []string{"output", "env"} {
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		output, err := runVersion(ctx, executable, "-test.run=^TestProbeProcess$", "--", "--discovery-fixture", mode)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if mode == "output" && (len(output) != outputLimit || string(output) != strings.Repeat("v", outputLimit)) {
			t.Fatalf("unbounded/contaminated output: %d bytes", len(output))
		}
		if mode == "env" && len(output) != 0 {
			t.Fatal("secret inherited by probe")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = runVersion(ctx, executable, "-test.run=^TestProbeProcess$", "--", "--discovery-fixture", "wait")
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("real subprocess cancellation failed: %v", err)
	}
}
