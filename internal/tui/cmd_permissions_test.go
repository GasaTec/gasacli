package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/trust"
)

func TestPermissionsCommandAndAlias(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	i18n.SetLanguage("pt-BR")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.NewStore(home).Grant(cwd); err != nil {
		t.Fatal(err)
	}
	command, ok := lookupCommand("/permissoes")
	if !ok || command.Name != "/permissions" || command.Handler == nil {
		t.Fatalf("alias registration=%+v, %v", command, ok)
	}
	got, err := command.Handler(context.Background(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Perfil ativo: balanced") || !strings.Contains(got, "read_project — Ler arquivos do projeto confiável: allow") {
		t.Fatalf("permissions view=%s", got)
	}
}
