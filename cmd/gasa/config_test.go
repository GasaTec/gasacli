package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/config"
)

func TestConfigShowPrintsEffectiveLanguageAndSource(t *testing.T) {
	t.Setenv("GASACLI_HOME", t.TempDir())
	t.Setenv("GASACLI_LANG", "en")
	if err := config.Ensure(); err != nil {
		t.Fatal(err)
	}
	output, err := os.CreateTemp(t.TempDir(), "config-output-")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if err := runConfig(context.Background(), []string{"show"}, output); err != nil {
		t.Fatal(err)
	}
	if _, err := output.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output.Name())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "language: en (env)") || strings.Contains(string(data), "language:  (user)") {
		t.Fatalf("config output=%s", data)
	}
}
