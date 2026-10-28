package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

type permissionCommandContext struct {
	home, project string
	trusted       bool
	confirmed     bool
	confirmCount  int
}

func (c *permissionCommandContext) ProjectPath() string               { return c.project }
func (c *permissionCommandContext) Trusted() bool                     { return c.trusted }
func (c *permissionCommandContext) ConfigHome() string                { return c.home }
func (*permissionCommandContext) DiscoveryResults() any               { return nil }
func (*permissionCommandContext) Storage() any                        { return nil }
func (*permissionCommandContext) Notify(string)                       {}
func (*permissionCommandContext) AppendMessage(string)                {}
func (*permissionCommandContext) Emit(string)                         {}
func (*permissionCommandContext) Ask(PermissionRequest) (bool, error) { return false, nil }
func (c *permissionCommandContext) Confirm(PermissionRequest) (bool, error) {
	c.confirmCount++
	return c.confirmed, nil
}

func TestPermissionsCommandRequiresApprovalForLoosening(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	i18n.Configure("")
	home := t.TempDir()
	cc := &permissionCommandContext{home: home, project: t.TempDir(), trusted: true, confirmed: true}
	result, err := permissionsCommand(context.Background(), cc, []string{"set-profile", "autonomous"})
	if err != nil || !strings.Contains(result, "autonomous") || cc.confirmCount != 1 {
		t.Fatalf("profile result=%q err=%v confirmations=%d", result, err, cc.confirmCount)
	}
	engine := permissions.New(home, trust.NewStore(home))
	profile, err := engine.Profile()
	if err != nil || profile != "autonomous" {
		t.Fatalf("profile=%q err=%v", profile, err)
	}
}

func TestPermissionsCommandConfirmsDenyToAskLoosening(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	i18n.Configure("")
	home := t.TempDir()
	cc := &permissionCommandContext{home: home, confirmed: true}
	engine := permissions.New(home, trust.NewStore(home))
	if err := engine.SetRule(permissions.UserScope, "", "", "execute_commands", permissions.Deny); err != nil {
		t.Fatal(err)
	}
	result, err := permissionsCommand(context.Background(), cc, []string{"ask", "execute_commands"})
	if err != nil || !strings.Contains(result, "Rule saved") || cc.confirmCount != 1 {
		t.Fatalf("result=%q err=%v confirmations=%d", result, err, cc.confirmCount)
	}
}

func TestPermissionsCommandRejectsLowerScopeElevation(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	i18n.Configure("")
	home := t.TempDir()
	cc := &permissionCommandContext{home: home, project: t.TempDir(), trusted: true, confirmed: true}
	engine := permissions.New(home, trust.NewStore(home))
	if err := engine.SetProfile("restricted"); err != nil {
		t.Fatal(err)
	}
	if err := ccProjectTrust(cc, home); err != nil {
		t.Fatal(err)
	}
	_, err := permissionsCommand(context.Background(), cc, []string{"allow", "execute_commands", "--project"})
	if err == nil || !strings.Contains(err.Error(), "cannot loosen") || cc.confirmCount != 0 {
		t.Fatalf("err=%v confirmations=%d", err, cc.confirmCount)
	}
}

func TestPermissionsAliasAndReadOnlyShow(t *testing.T) {
	t.Setenv("GASACLI_LANG", "pt-BR")
	i18n.SetLanguage("pt-BR")
	command, ok := lookupCommand("/permissoes")
	if !ok || command.Name != "/permissions" || command.Planned {
		t.Fatalf("alias command=%+v found=%v", command, ok)
	}
	cc := &permissionCommandContext{home: t.TempDir(), project: t.TempDir(), trusted: true}
	if err := ccProjectTrust(cc, cc.home); err != nil {
		t.Fatal(err)
	}
	result, err := permissionsCommand(context.Background(), cc, nil)
	if err != nil || !strings.Contains(result, "Perfil ativo: balanced") || cc.confirmCount != 0 {
		t.Fatalf("show result=%q err=%v confirmations=%d", result, err, cc.confirmCount)
	}
}

func ccProjectTrust(cc *permissionCommandContext, home string) error {
	return trust.NewStore(home).Grant(cc.project)
}
