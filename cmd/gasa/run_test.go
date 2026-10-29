package main

import "testing"

func TestParseRunArgs(t *testing.T) {
	goal, agent, reviewer, err := parseRunArgs([]string{"--agent", "claude", "--reviewer", "codex", "create", "hello.txt", "containing", "hi"})
	if err != nil || goal != "create hello.txt containing hi" || agent != "claude" || reviewer != "codex" {
		t.Fatalf("parsed goal=%q agent=%q reviewer=%q err=%v", goal, agent, reviewer, err)
	}
	if _, _, _, err := parseRunArgs([]string{"--agent"}); err == nil {
		t.Fatal("missing agent id accepted")
	}
	if _, _, _, err := parseRunArgs([]string{"--other", "goal"}); err == nil {
		t.Fatal("unknown option accepted")
	}
}
