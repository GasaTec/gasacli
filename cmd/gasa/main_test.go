package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestVersionCommand(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "1.2.3-rc.1", "abc123", "2026-10-27T12:34:56Z"
	t.Cleanup(func() { version, commit, date = oldVersion, oldCommit, oldDate })

	for _, args := range [][]string{{"--version"}, {"version"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("GASACLI_HOME", home)
			output, err := captureStdout(t, func() error { return run(context.Background(), args) })
			if err != nil {
				t.Fatal(err)
			}
			want := "gasa 1.2.3-rc.1 (abc123, 2026-10-27T12:34:56Z, " + runtime.GOOS + "/" + runtime.GOARCH + ")\n"
			if output != want {
				t.Fatalf("output = %q, want %q", output, want)
			}
			entries, err := os.ReadDir(home)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("version command created files in GASACLI_HOME: %v", entries)
			}
		})
	}
}

func captureStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	old := os.Stdout
	os.Stdout = w
	callErr := f()
	_ = w.Close()
	os.Stdout = old
	output, readErr := io.ReadAll(r)
	_ = r.Close()
	if readErr != nil {
		return "", readErr
	}
	return string(output), callErr
}

func TestBrowserPreference(t *testing.T) {
	for _, tc := range []struct {
		mode           string
		disabled, want bool
	}{
		{"manual", false, false}, {"manual", true, false},
		{"auto", false, true}, {"auto", true, false},
		{"always", false, true}, {"always", true, false},
	} {
		if got := shouldOpen(tc.mode, tc.disabled); got != tc.want {
			t.Errorf("mode=%s no-browser=%v: got %v, want %v", tc.mode, tc.disabled, got, tc.want)
		}
	}
}

func TestUnreadableOnboardingStateIsIncompleteWithoutOverwrite(t *testing.T) {
	home := t.TempDir()
	statePath := filepath.Join(home, "state.json")
	if err := os.Mkdir(statePath, 0700); err != nil {
		t.Fatal(err)
	}
	var warning strings.Builder
	state, writable := loadStartupState(home, &warning)
	if writable || state.Onboarding.Completed || !strings.Contains(warning.String(), "treating onboarding as incomplete") {
		t.Fatalf("state=%+v writable=%v warning=%q", state, writable, warning.String())
	}
	info, err := os.Stat(statePath)
	if err != nil || !info.IsDir() {
		t.Fatalf("state directory changed: %v", err)
	}
}

func TestCancellationUnblocksIdleChat(t *testing.T) {
	testCancellation(t, nil, '>')
}

func TestCancellationStopsStandaloneDashboard(t *testing.T) {
	testCancellation(t, []string{"web", "open"}, '\n')
}

func testCancellation(t *testing.T, args []string, delimiter byte) {
	t.Helper()
	t.Setenv("GASACLI_HOME", t.TempDir())
	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer inputWriter.Close()
	output, outputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	defer outputWriter.Close()
	originalInput, originalOutput := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = input, outputWriter
	defer func() { os.Stdin, os.Stdout = originalInput, originalOutput }()
	// Wait for the idle chat prompt, then drain without recording credentials.
	ready := make(chan struct{})
	go func() {
		reader := bufio.NewReader(output)
		if _, err := reader.ReadString(delimiter); err == nil {
			close(ready)
		}
		_, _ = io.Copy(io.Discard, reader)
	}()
	if _, err := inputWriter.Write([]byte("1\n")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- run(ctx, append([]string{"--no-browser"}, args...)) }()
	select {
	case err := <-finished:
		t.Fatalf("chat exited before cancellation: %v", err)
	case <-ready:
	case <-time.After(5 * time.Second):
		cancel()
		<-finished
		t.Fatal("chat never reached idle prompt")
	}
	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		inputWriter.Close()
		<-finished
		t.Fatal("chat did not stop while stdin was idle")
	}
}
