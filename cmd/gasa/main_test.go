package main

import (
	"bufio"
	"context"
	"io"
	"os"
	"testing"
	"time"
)

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
	if _, err := inputWriter.Write([]byte("y\n")); err != nil {
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
