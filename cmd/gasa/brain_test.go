package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestBrainStatusWithoutVaultIsActionable(t *testing.T) {
	for _, test := range []struct{ language, want string }{
		{"en", "Run `gasa brain connect <path>`"},
		{"pt-BR", "Execute `gasa brain connect <caminho>`"},
	} {
		t.Run(test.language, func(t *testing.T) {
			t.Setenv("GASACLI_HOME", t.TempDir())
			t.Setenv("GASACLI_LANG", test.language)
			var out bytes.Buffer
			if err := runBrain(context.Background(), []string{"status"}, &out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), test.want) || strings.Contains(out.String(), "no rows in result set") {
				t.Fatalf("status output=%q", out.String())
			}
		})
	}
}
