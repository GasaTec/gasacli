package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/config"
)

func TestConfigDisplaysDeveloperAndReviewerPreferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	if err := config.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := config.Set(home, "agents.developer", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := config.Set(home, "agents.reviewer", "codex"); err != nil {
		t.Fatal(err)
	}
	cc := &engineCommandContext{home: home}
	got, err := configCommand(context.Background(), cc, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"agents.developer: claude (user)", "agents.reviewer: codex (user)"} {
		if !strings.Contains(got, want) {
			t.Errorf("config output %q is missing %q", got, want)
		}
	}
}
