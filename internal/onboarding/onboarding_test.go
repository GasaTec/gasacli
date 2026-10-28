package onboarding

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/discovery"
)

type keys []Key

func (k *keys) ReadKey() (Key, error) {
	if len(*k) == 0 {
		return KeyEnter, nil
	}
	v := (*k)[0]
	*k = (*k)[1:]
	return v, nil
}

func TestCatalogIsFixedAndSafe(t *testing.T) {
	allowed := map[string]bool{"Anthropic.ClaudeCode": true, "OpenAI.Codex": true, "Obsidian.Obsidian": true}
	for _, it := range Catalog() {
		for osname, methods := range it.Methods {
			for _, m := range methods {
				if !m.Auto {
					if m.Manager != "" || len(m.Args) > 0 {
						t.Errorf("manual method executable: %s", it.ID)
					}
					continue
				}
				if m.Manager == "winget" {
					if len(m.Args) == 0 || m.Args[0] != "install" || m.Args[len(m.Args)-1] != "-e" || !allowed[m.Args[2]] {
						t.Errorf("invalid winget command %+v", m)
					}
				}
				if m.Manager != "winget" && m.Manager != "brew" && m.Manager != "npm" && m.Manager != "flatpak" {
					t.Errorf("unapproved manager %s", m.Manager)
				}
				for _, arg := range m.Args {
					lower := strings.ToLower(arg)
					if lower == "sh" || lower == "cmd" || lower == "powershell" || strings.Contains(arg, "|") {
						t.Errorf("shell execution in argv: %v", m.Args)
					}
				}
				if osname == "linux" && m.Manager == "winget" {
					t.Fatal("wrong platform method")
				}
			}
		}
	}
}

func TestSelectorSequences(t *testing.T) {
	items := []Item{{ID: "claude", Name: "Claude Code"}, {ID: "codex", Name: "Codex"}, {ID: "cursor", Name: "Cursor CLI"}}
	for _, tc := range []struct {
		name     string
		sequence []Key
		want     map[string]bool
		err      bool
	}{
		{"defaults", []Key{KeyEnter}, map[string]bool{"claude": true, "codex": true}, false},
		{"down-down-space", []Key{KeyDown, KeyDown, KeySpace, KeyEnter}, map[string]bool{"claude": true, "codex": true, "cursor": true}, false},
		{"all", []Key{KeyAll, KeyEnter}, map[string]bool{"claude": true, "codex": true, "cursor": true}, false},
		{"none", []Key{KeyNone, KeyEnter}, map[string]bool{"claude": false, "codex": false}, false},
		// "n" clears the selection but must NOT leave the list: the user can keep choosing.
		{"none-stays-in-list", []Key{KeyNone, KeyDown, KeySpace, KeyEnter}, map[string]bool{"claude": false, "codex": true}, false},
		{"escape", []Key{KeyEscape}, map[string]bool{"claude": false, "codex": false}, false},
		{"cancel", []Key{KeyCancel}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected := map[string]bool{"claude": true, "codex": true}
			var out bytes.Buffer
			k := keys(tc.sequence)
			got, err := (Selector{Reader: &k, Out: &out}).Select(items, selected)
			if (err != nil) != tc.err {
				t.Fatalf("err=%v", err)
			}
			if err == nil && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
			if tc.name == "down-down-space" && !strings.Contains(out.String(), "> [ ] Cursor CLI") {
				t.Fatalf("render order/cursor: %s", out.String())
			}
		})
	}
}

func TestChooseMethodPrerequisites(t *testing.T) {
	item := findItem("kimi")
	lookup := func(name string) (string, error) {
		if name == "npm" || name == "node" {
			return "node", nil
		}
		return "", exec.ErrNotFound
	}
	for _, tc := range []struct {
		node, wantReason string
		auto             bool
	}{{"v22.19.0", "", true}, {"v20.10.0", "Node v20.10.0 < 22.19", false}} {
		m, reason := chooseMethod(context.Background(), item, "windows", lookup, func(context.Context, string, ...string) ([]byte, error) { return []byte(tc.node), nil })
		if m.Auto != tc.auto || reason != tc.wantReason {
			t.Fatalf("method=%+v reason=%q", m, reason)
		}
	}
	manual, reason := chooseMethod(context.Background(), findItem("cursor"), "windows", func(string) (string, error) { return "", exec.ErrNotFound }, func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("unexpected prereq run")
		return nil, nil
	})
	if manual.Auto || manual.Label != "manual" || reason == "" {
		t.Fatalf("manual=%+v reason=%q", manual, reason)
	}
}

func TestNodeAtLeastMajorAndFullVersions(t *testing.T) {
	for _, tc := range []struct {
		got, min string
		want     bool
	}{{"v22.20.0", "22", true}, {"v20.20.0", "22", false}, {"22.19.0", "22.18", true}, {"22.18.0", "22.19.0", false}} {
		if got := nodeAtLeast(tc.got, tc.min); got != tc.want {
			t.Errorf("nodeAtLeast(%q,%q)=%v want %v", tc.got, tc.min, got, tc.want)
		}
	}
}

func TestPlanOnlyNeverRunsInstallRunner(t *testing.T) {
	var out bytes.Buffer
	missing := []discovery.Result{{ID: "claude", Name: "Claude Code", Status: discovery.NotFound}, {ID: "codex", Name: "Codex", Status: discovery.NotFound}, {ID: "gemini", Name: "Gemini", Status: discovery.Available}, {ID: "cursor-agent", Name: "Cursor", Status: discovery.Available}, {ID: "opencode", Name: "OpenCode", Status: discovery.Available}, {ID: "orca", Name: "Orca", Status: discovery.Available}, {ID: "kimi", Name: "Kimi", Status: discovery.Available}}
	err := Run(context.Background(), Options{In: strings.NewReader(""), Out: &out, Interactive: true, PlanOnly: true, Keys: &keys{KeyEnter}, Discover: func(context.Context) []discovery.Result { return missing }, Apps: map[string]discovery.Result{"obsidian": {ID: "obsidian", Name: "Obsidian", Status: discovery.Available}}, Lookup: func(string) (string, error) { return "tool", nil }, Run: func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("plan queried a command runner")
		return nil, nil
	}, Command: func(context.Context, string, []string, io.Reader, io.Writer, io.Writer) error {
		t.Fatal("plan executed installer")
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Comando: winget install") || !strings.Contains(out.String(), "Para instalar, rode `gasa setup` fora do chat.") {
		t.Fatalf("plan missing commands or handoff: %s", out.String())
	}
}

func TestRawSelectionCancellationDoesNotFallBack(t *testing.T) {
	var out bytes.Buffer
	in, inW, _ := os.Pipe()
	defer in.Close()
	defer inW.Close()
	outFile, outW, _ := os.Pipe()
	defer outFile.Close()
	defer outW.Close()
	seq := keys{KeyCancel}
	err := Run(context.Background(), Options{In: strings.NewReader("1\n"), Out: &out, InFile: in, OutFile: outFile, Interactive: true, Terminal: func(*os.File, *os.File) (KeyReader, func(), error) { return &seq, func() {}, nil }, Discover: func(context.Context) []discovery.Result {
		return []discovery.Result{{ID: "claude", Name: "Claude", Status: discovery.NotFound}}
	}, Apps: map[string]discovery.Result{"obsidian": {ID: "obsidian", Status: discovery.Available}}, Lookup: func(string) (string, error) { return "tool", nil }})
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("cancel error=%v", err)
	}
	if strings.Contains(out.String(), "Digite n") {
		t.Fatalf("cancel unexpectedly fell back to numbered input: %s", out.String())
	}
}

func TestRealCommandDropsNonFileStdin(t *testing.T) {
	path, args := "true", []string{}
	if runtime.GOOS == "windows" {
		path, args = "cmd", []string{"/c", "exit", "0"}
	}
	start := time.Now()
	if err := realCommand(context.Background(), path, args, strings.NewReader("never consumed"), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed >= 2*time.Second {
		t.Fatalf("command took %s", elapsed)
	}
}

func TestCommandTreeCancellation(t *testing.T) {
	if os.Getenv("GASA_TREE_GRANDCHILD") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if os.Getenv("GASA_TREE_CHILD") == "1" {
		time.Sleep(150 * time.Millisecond)
		cmd := exec.Command(os.Args[0], "-test.run=TestCommandTreeCancellation")
		cmd.Env = append(os.Environ(), "GASA_TREE_GRANDCHILD=1")
		if err := cmd.Start(); err != nil {
			t.Fatalf("start grandchild process: %v", err)
		}
		_ = os.WriteFile(os.Getenv("GASA_TREE_PID_FILE"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600)
		for {
			time.Sleep(time.Hour)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "-test.run=TestCommandTreeCancellation")
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	cmd.Env = append(os.Environ(), "GASA_TREE_CHILD=1", "GASA_TREE_PID_FILE="+pidFile)
	cmd.Stdout = io.Discard
	var childErr bytes.Buffer
	cmd.Stderr = &childErr
	start := time.Now()
	err := runCommandTree(ctx, cmd)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("tree cancellation err=%v child stderr=%s", err, childErr.String())
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Fatalf("tree cancellation took %s", elapsed)
	}
	if _, err := os.Stat(pidFile); err != nil {
		t.Fatalf("test grandchild was not started: %v", err)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for processAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if processAlive(pid) {
		t.Fatalf("grandchild process %d survived cancellation", pid)
	}
}

func TestCommandTreeSuccessLeavesGrandchildRunning(t *testing.T) {
	if os.Getenv("GASA_TREE_GRANDCHILD") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
	if os.Getenv("GASA_TREE_SUCCESS_CHILD") == "1" {
		cmd := exec.Command(os.Args[0], "-test.run=TestCommandTreeSuccessLeavesGrandchildRunning")
		cmd.Env = append(os.Environ(), "GASA_TREE_GRANDCHILD=1")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv("GASA_TREE_PID_FILE"), []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	cmd := exec.Command(os.Args[0], "-test.run=TestCommandTreeSuccessLeavesGrandchildRunning")
	cmd.Env = append(os.Environ(), "GASA_TREE_SUCCESS_CHILD=1", "GASA_TREE_PID_FILE="+pidFile)
	if err := runCommandTree(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("grandchild was not started: %v", err)
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if p, e := os.FindProcess(pid); e == nil {
			_ = p.Kill()
		}
	}()
	if !processAlive(pid) {
		t.Fatalf("grandchild process %d was killed after success", pid)
	}
}

func TestRunConfirmationAndFixedArgv(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		wantRun      int
	}{{"default-no", "N\n", 0}, {"save", "s\n", 0}, {"confirmed", "y\n", 1}} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			var calls int
			var gotArgs []string
			discover := func(context.Context) []discovery.Result {
				return []discovery.Result{{ID: "claude", Name: "Claude Code", Status: discovery.NotFound}, {ID: "codex", Name: "Codex", Status: discovery.Available}, {ID: "gemini", Name: "Gemini CLI", Status: discovery.Available}, {ID: "cursor-agent", Name: "Cursor CLI", Status: discovery.Available}, {ID: "opencode", Name: "OpenCode", Status: discovery.Available}, {ID: "orca", Name: "Orca", Status: discovery.Available}, {ID: "kimi", Name: "Kimi Code CLI", Status: discovery.Available}}
			}
			o := Options{In: strings.NewReader(tc.answer), Out: &out, Interactive: true, GOOS: "windows", Keys: &keys{KeyEnter}, Discover: discover, Apps: map[string]discovery.Result{"obsidian": {ID: "obsidian", Name: "Obsidian", Status: discovery.Available}}, Lookup: func(s string) (string, error) { return "C:/tools/" + s, nil }, Run: func(context.Context, string, ...string) ([]byte, error) { return nil, nil }, Command: func(ctx context.Context, path string, args []string, in io.Reader, stdout, stderr io.Writer) error {
				calls++
				gotArgs = append([]string{path}, args...)
				return nil
			}}
			if err := Run(context.Background(), o); err != nil {
				t.Fatal(err)
			}
			if calls != tc.wantRun {
				t.Fatalf("calls=%d want=%d output=%s", calls, tc.wantRun, out.String())
			}
			if !strings.Contains(out.String(), "winget install --id Anthropic.ClaudeCode -e") || !strings.Contains(out.String(), "https://code.claude.com/docs/en/setup") {
				t.Fatalf("plan missing argv/source: %s", out.String())
			}
			if tc.wantRun == 1 && !reflect.DeepEqual(gotArgs, []string{"C:/tools/winget", "install", "--id", "Anthropic.ClaudeCode", "-e"}) {
				t.Fatalf("argv=%v", gotArgs)
			}
		})
	}
}

func TestRunFailureDoesNotBlockNextAndInheritsStreams(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	stdin := strings.NewReader("y\n")
	allInstalled := []discovery.Result{{ID: "claude", Name: "Claude Code", Status: discovery.NotFound}, {ID: "codex", Name: "Codex", Status: discovery.NotFound}, {ID: "gemini", Name: "Gemini CLI", Status: discovery.Available}, {ID: "cursor-agent", Name: "Cursor", Status: discovery.Available}, {ID: "opencode", Name: "OpenCode", Status: discovery.Available}, {ID: "orca", Name: "Orca", Status: discovery.Available}, {ID: "kimi", Name: "Kimi", Status: discovery.Available}}
	o := Options{In: stdin, Out: &out, Interactive: true, GOOS: "windows", Keys: &keys{KeyEnter}, Discover: func(context.Context) []discovery.Result { return allInstalled }, Apps: map[string]discovery.Result{"obsidian": {ID: "obsidian", Name: "Obsidian", Status: discovery.Available}}, Lookup: func(n string) (string, error) { return n, nil }, Command: func(ctx context.Context, path string, args []string, in io.Reader, stdout, stderr io.Writer) error {
		calls++
		if in != stdin || stdout != &out || stderr != &out {
			t.Error("child streams were not inherited")
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > installTimeout {
			t.Error("per-item timeout missing")
		}
		if calls == 1 {
			return &exec.ExitError{ProcessState: nil}
		}
		return nil
	}}
	// A synthetic ExitError without ProcessState is still enough to exercise continue behavior.
	if err := Run(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(out.String(), "exit code") {
		t.Fatalf("calls=%d output=%s", calls, out.String())
	}
}

func TestCancellationAndTimeout(t *testing.T) {
	called := false
	var out bytes.Buffer
	o := Options{In: strings.NewReader("y\n"), Out: &out, Interactive: true, GOOS: "windows", Timeout: 10 * time.Millisecond, Keys: &keys{KeyEnter}, Discover: func(context.Context) []discovery.Result {
		return []discovery.Result{{ID: "claude", Name: "Claude Code", Status: discovery.NotFound}}
	}, Apps: map[string]discovery.Result{"obsidian": {ID: "obsidian", Name: "Obsidian", Status: discovery.Available}}, Lookup: func(s string) (string, error) { return s, nil }, Command: func(ctx context.Context, _ string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		called = true
		if _, ok := ctx.Deadline(); !ok {
			t.Error("timeout absent")
		}
		<-ctx.Done()
		return ctx.Err()
	}}
	err := Run(context.Background(), o)
	if err != nil || !called || !strings.Contains(out.String(), "timeout") {
		t.Fatalf("timed out command: err=%v called=%v output=%s", err, called, out.String())
	}
}

func TestInstallContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out bytes.Buffer
	called := false
	o := Options{In: strings.NewReader("y\n"), Out: &out, Interactive: true, GOOS: "windows", Keys: &keys{KeyEnter}, Discover: func(context.Context) []discovery.Result {
		return []discovery.Result{{ID: "claude", Name: "Claude Code", Status: discovery.NotFound}}
	}, Apps: map[string]discovery.Result{"obsidian": {ID: "obsidian", Name: "Obsidian", Status: discovery.Available}}, Lookup: func(s string) (string, error) { return s, nil }, Command: func(child context.Context, _ string, _ []string, _ io.Reader, _ io.Writer, _ io.Writer) error {
		called = true
		cancel()
		<-child.Done()
		return child.Err()
	}}
	if err := Run(ctx, o); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run after cancellation err=%v", err)
	}
	if !called || !strings.Contains(out.String(), "cancelado") {
		t.Fatalf("command cancellation not reported: %s", out.String())
	}
}

func TestDetectAppsWindowsAndLinuxFlatpak(t *testing.T) {
	deps := AppDeps{Getenv: func(string) string { return "C:/Users/test/AppData/Local" }, Stat: func(p string) (os.FileInfo, error) {
		if strings.HasSuffix(p, "Obsidian.exe") {
			return nil, nil
		}
		return nil, os.ErrNotExist
	}, LookPath: func(string) (string, error) { return "", exec.ErrNotFound }}
	if got := DetectApps(context.Background(), "windows", deps)["obsidian"]; got.Status != discovery.Available {
		t.Fatalf("windows=%+v", got)
	}
	called := false
	deps = AppDeps{LookPath: func(n string) (string, error) {
		if n == "flatpak" {
			return "/usr/bin/flatpak", nil
		}
		return "", exec.ErrNotFound
	}, Run: func(ctx context.Context, path string, args ...string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Error("flatpak probe timeout missing")
		}
		called = path == "/usr/bin/flatpak" && reflect.DeepEqual(args, []string{"info", "md.obsidian.Obsidian"})
		return nil, nil
	}}
	if got := DetectApps(context.Background(), "linux", deps)["obsidian"]; got.Status != discovery.Available || !called {
		t.Fatalf("linux=%+v called=%v", got, called)
	}
}

func TestManualFallbackAndSelectedOrder(t *testing.T) {
	var out bytes.Buffer
	items := []Item{{ID: "claude", Name: "Claude Code"}, {ID: "cursor", Name: "Cursor CLI"}}
	selected, err := (Selector{Reader: NewByteKeyReader(strings.NewReader("\x1b[B \r")), Out: &out}).Select(items, map[string]bool{"claude": true})
	if err != nil || !reflect.DeepEqual(selected, map[string]bool{"claude": true, "cursor": true}) {
		t.Fatalf("selection=%v err=%v", selected, err)
	}
	if _, err := SelectNumbers(strings.NewReader("n\n"), io.Discard, items, map[string]bool{"claude": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(t.TempDir(), "never")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestSelectNumbersRejectsNonDigitTokens(t *testing.T) {
	items := []Item{{ID: "one", Name: "One"}}
	for _, input := range []string{"1e9\n", "2abc\n", "+1\n", "1,2\n"} {
		selected, err := SelectNumbers(strings.NewReader(input), io.Discard, items, nil)
		if err == nil || selected != nil {
			t.Errorf("input %q selected=%v err=%v", input, selected, err)
		}
	}
	if _, err := SelectNumbers(strings.NewReader("1\n"), io.Discard, items, nil); err != nil {
		t.Fatal(err)
	}
}

func TestNumberedSelectionSharesBufferedInputWithConfirmation(t *testing.T) {
	items := []Item{{ID: "one"}, {ID: "two"}}
	for _, input := range []struct{ value, selected, confirm string }{
		{"1 2\nY\n", "one,two", "Y"},
		{"\nn\n", "", "n"},
	} {
		t.Run(input.value, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(input.value))
			selected, err := selectNumbers(reader, io.Discard, items, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if selected["one"] {
				got = "one"
			}
			if selected["two"] {
				if got != "" {
					got += ","
				}
				got += "two"
			}
			if got != input.selected {
				t.Fatalf("selected=%q want=%q", got, input.selected)
			}
			confirm, err := reader.ReadString('\n')
			if err != nil || strings.TrimSpace(confirm) != input.confirm {
				t.Fatalf("confirmation=%q err=%v", confirm, err)
			}
		})
	}
}

func TestExtraPathsGivesNpmAndBrewIndependentTimeouts(t *testing.T) {
	var calls []string
	o := Options{GOOS: "linux", Run: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Errorf("%s has no timeout", name)
		} else if remaining := time.Until(deadline); remaining > 5*time.Second || remaining < 4*time.Second {
			t.Errorf("%s timeout=%s", name, remaining)
		}
		calls = append(calls, name)
		return nil, errors.New("not installed")
	}}
	_ = extraPaths(context.Background(), Item{Binaries: []string{"tool"}}, o)
	if strings.Join(calls, ",") != "npm,brew" {
		t.Fatalf("probes=%v", calls)
	}
}

func TestTerminalRestoredAfterSelectionAndCancel(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		in, inW, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		out, outW, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		restored := 0
		open := func(*os.File, *os.File) (KeyReader, func(), error) {
			seq := keys{KeyEnter}
			if cancel {
				seq = keys{KeyCancel}
			}
			return &seq, func() { restored++ }, nil
		}
		_, runErr := SelectRaw([]Item{{ID: "a", Name: "Agent"}}, nil, in, out, io.Discard, open)
		if cancel && runErr == nil {
			t.Fatal("expected cancel")
		}
		if !cancel && runErr != nil {
			t.Fatal(runErr)
		}
		if restored != 1 {
			t.Fatalf("cancel=%v restore calls=%d", cancel, restored)
		}
		in.Close()
		inW.Close()
		out.Close()
		outW.Close()
	}
}

func TestPostInstallSearchesExtraDirectory(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(binary, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	o := Options{GOOS: "windows", Lookup: func(string) (string, error) { return "", exec.ErrNotFound }, Run: func(_ context.Context, path string, args ...string) ([]byte, error) {
		if path == "npm" && reflect.DeepEqual(args, []string{"prefix", "-g"}) {
			return []byte(dir), nil
		}
		return nil, errors.New("not found")
	}, Stat: os.Stat}
	verifyInstalled(context.Background(), findItem("claude"), o, &out)
	if !strings.Contains(out.String(), "Instalado em "+binary+"; abra um novo terminal.") {
		t.Fatalf("extra path not reported: %s", out.String())
	}
}

func findItem(id string) Item {
	for _, it := range Catalog() {
		if it.ID == id {
			return it
		}
	}
	return Item{}
}
