package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/trust"
)

func TestSidebarSnapshotUsesTrustedProjectsAndDiscoveredAgents(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	store := trust.NewStore(home)
	if err := store.Grant(project); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("agents:\n  developer: codex\n"), 0600); err != nil {
		t.Fatal(err)
	}
	active, err := trust.Canonical(project)
	if err != nil {
		t.Fatal(err)
	}
	s := &Session{TrustStore: store, ConfigHome: home, DiscoveryResults: func() any {
		return []discovery.Result{{ID: "codex", Name: "Codex", Status: discovery.Ready, Auth: discovery.AuthReady}, {ID: "missing", Status: discovery.Missing}}
	}}
	got := loadSidebarSnapshot(s, active)
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if len(got.Projects) != 1 || !got.Projects[0].Active || got.Projects[0].Path != active {
		t.Fatalf("projects: %+v", got.Projects)
	}
	if len(got.Agents) != 1 || got.Agents[0].Role != "developer" || got.Agents[0].Status != discovery.Ready {
		t.Fatalf("agents: %+v", got.Agents)
	}
}

func TestSidebarSnapshotReportsTrustErrors(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "trust.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	got := loadSidebarSnapshot(&Session{TrustStore: trust.NewStore(home), ConfigHome: home}, "")
	if got.Err == nil || len(got.Projects) != 0 {
		t.Fatalf("snapshot: %+v", got)
	}
}

func TestSidebarSnapshotShowsConfiguredMissingAgent(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("agents:\n  reviewer: codex\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &Session{TrustStore: trust.NewStore(home), ConfigHome: home, DiscoveryResults: func() any {
		return []discovery.Result{{ID: "codex", Name: "Codex", Status: discovery.Missing}, {ID: "other", Status: discovery.Missing}}
	}}
	got := loadSidebarSnapshot(s, "")
	if got.Err != nil || len(got.Agents) != 1 || got.Agents[0].ID != "codex" || got.Agents[0].Status != discovery.Missing || got.Agents[0].Role != "reviewer" {
		t.Fatalf("snapshot: %+v", got)
	}
}
