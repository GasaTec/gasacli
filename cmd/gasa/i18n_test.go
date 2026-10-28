package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/i18n"
)

func TestFormatDiagnosticOutputUsesSelectedCatalog(t *testing.T) {
	previous := i18n.Language()
	t.Cleanup(func() { i18n.SetLanguage(previous) })
	result := []discovery.Result{{
		Name: "Codex", Status: discovery.Installed, Auth: discovery.AuthUnknown,
		Version: "1.2.3", Note: "authentication not checked",
	}}
	for _, test := range []struct {
		language string
		want     string
	}{
		{language: "en", want: "installed"},
		{language: "pt-BR", want: "instalado"},
	} {
		t.Run(test.language, func(t *testing.T) {
			i18n.SetLanguage(test.language)
			var output bytes.Buffer
			if err := formatDiagnosticOutput(&output, result); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), test.want) {
				t.Fatalf("diagnostics %q omit %q", output.String(), test.want)
			}
		})
	}
}
