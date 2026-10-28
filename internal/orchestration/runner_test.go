package orchestration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
)

func TestSelectReviewerPrefersDifferentReadyAgentThenFreshDeveloperSession(t *testing.T) {
	results := []discovery.Result{{ID: "claude", Status: discovery.Ready}, {ID: "codex", Status: discovery.Installed}}
	if got := SelectReviewer(results, "claude"); got != "codex" {
		t.Fatalf("reviewer = %q", got)
	}
	if got := SelectReviewer(results[:1], "claude"); got != "claude" {
		t.Fatalf("single-agent reviewer = %q", got)
	}
	if got := SelectReviewer(nil, ""); got != "" {
		t.Fatalf("no-agent reviewer = %q", got)
	}
}

func TestGateCommandsPreserveArgvAndOrder(t *testing.T) {
	configured := config.ProjectGates{Test: []string{"go", "test"}, Lint: []string{"lint", "run"}}
	got := gateCommands(configured)
	if len(got) != 2 || got[0].Name != "test" || got[1].Name != "lint" || got[0].Argv[1] != "test" {
		t.Fatalf("commands = %#v", got)
	}
	got[0].Argv[0] = "mutated"
	if configured.Test[0] != "go" {
		t.Fatal("gate conversion aliased project configuration")
	}
}

func TestSnapshotTracksFilesButIgnoresGitMetadata(t *testing.T) {
	project := t.TempDir()
	file := filepath.Join(project, "hello.txt")
	if err := os.WriteFile(file, []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := snapshot(project)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(project, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, ".git", "ignored"), []byte("metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := snapshot(project)
	if err != nil || second != first {
		t.Fatalf("git metadata changed snapshot %q -> %q: %v", first, second, err)
	}
	if err = os.WriteFile(file, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := snapshot(project)
	if err != nil || third == second {
		t.Fatalf("file content did not change snapshot: %q %q %v", second, third, err)
	}
}
