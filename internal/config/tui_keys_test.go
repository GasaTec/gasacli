package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTUIKeysValidation(t *testing.T) {
	defaults, err := (TUIKeys{}).Resolved()
	if err != nil || defaults.Projects != "ctrl+p" || defaults.Palette != "ctrl+k" {
		t.Fatal("invalid defaults")
	}
	for _, keys := range []TUIKeys{{Projects: "ctrl+c"}, {Theme: "enter"}, {Projects: "f1", Palette: "F1"}, {Projects: "x"}} {
		if _, err := keys.Resolved(); err == nil {
			t.Fatalf("unsafe shortcut accepted: %+v", keys)
		}
	}
	keys, err := (TUIKeys{Projects: "F1", Theme: "f12"}).Resolved()
	if err != nil || keys.Projects != "f1" || keys.Theme != "f12" {
		t.Fatalf("function keys: %+v %v", keys, err)
	}
}

func TestLoadTUIKeys(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("tui:\n  keys:\n    projects: F1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFrom(home)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := cfg.TUI.Keys.Resolved()
	if err != nil || keys.Projects != "f1" {
		t.Fatalf("keys=%+v err=%v", keys, err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("tui:\n  keys:\n    projects: ctrl+c\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFrom(home); err == nil {
		t.Fatal("unsafe binding was accepted")
	}
}
