package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetSupportedValuesAndValidation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, setting := range [][2]string{{"web.mode", "manual"}, {"web.port", "4040"}, {"language", "en"}, {"permissions.profile", "restricted"}, {"agents.defaults.developer", "codex"}} {
		if err := Set(home, setting[0], setting[1]); err != nil {
			t.Fatalf("Set(%s): %v", setting[0], err)
		}
	}
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Web.Mode != "manual" || cfg.Web.Port != 4040 || cfg.Language != "en" || cfg.Agents.Defaults["developer"] != "codex" {
		t.Fatalf("config=%+v", cfg)
	}
	policy, err := os.ReadFile(filepath.Join(home, "permissions.yaml"))
	if err != nil || !strings.Contains(string(policy), "restricted") {
		t.Fatalf("permission profile=%q err=%v", policy, err)
	}
	if err := Set(home, "language", "fr"); err == nil {
		t.Fatal("unsupported language accepted")
	}
}

func TestEditDoesNotPublishInvalidConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	path := ConfigPath(home)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = Edit(home, []byte("web:\n  mode: dangerous\n")); err == nil {
		t.Fatal("invalid config accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatalf("invalid config changed original: %q err=%v", after, err)
	}
}
