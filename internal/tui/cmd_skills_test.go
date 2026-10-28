package tui

import (
	"context"
	"github.com/GasaTec/gasacli/internal/i18n"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsHandlerShowsCatalogRecommendations(t *testing.T) {
	t.Setenv("GASACLI_HOME", filepath.Join(t.TempDir(), "home"))
	t.Setenv("GASACLI_LANG", "en")
	i18n.Configure("")
	command, ok := lookupCommand("/skills")
	if !ok || command.Handler == nil {
		t.Fatal("/skills handler is not registered")
	}
	got, err := command.Handler(context.Background(), nil, []string{"recommendations"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Ponytail 5", "Caveman"} {
		if !strings.Contains(got, name) {
			t.Errorf("recommendations omit %s: %s", name, got)
		}
	}
}

func TestSkillsHandlerListsLocalSource(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("GASACLI_HOME", home)
	t.Setenv("GASACLI_LANG", "en")
	i18n.Configure("")
	root := filepath.Join(home, "skills", "example")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: example\nversion: 1\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command, _ := lookupCommand("/skills")
	got, err := command.Handler(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "example") || !strings.Contains(got, "gasacli") {
		t.Fatalf("skills output=%q", got)
	}
}
