package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrainHandlerConnectsReadOnlyAndSearches(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	t.Setenv("GASACLI_HOME", home)
	t.Setenv("GASACLI_LANG", "en")
	vault := filepath.Join(t.TempDir(), "vault")
	if err := os.Mkdir(vault, 0700); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(vault, "note.md")
	if err := os.WriteFile(note, []byte("# ADR\npermissions stay local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(note)
	if err != nil {
		t.Fatal(err)
	}
	command, ok := lookupCommand("/brain")
	if !ok || command.Handler == nil {
		t.Fatal("/brain handler is not registered")
	}
	got, err := command.Handler(context.Background(), nil, []string{"connect", vault, "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "read-only") {
		t.Fatalf("connect output=%q", got)
	}
	got, err = command.Handler(context.Background(), nil, []string{"search", "permissions", "ADR"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, note) || !strings.Contains(got, "ADR") {
		t.Fatalf("search output=%q", got)
	}
	after, err := os.ReadFile(note)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("brain handler changed vault contents")
	}
}
