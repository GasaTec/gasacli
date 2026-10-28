package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
)

func init() {
	command := Command{Name: "/permissions", Summary: "command.permissions", Usage: "/permissions", Category: "general", Planned: true, IssueRef: "#31", Handler: permissionsCommand}
	Register(command)
	commandRegistry["/permissoes"] = command
}

func permissionsCommand(ctx context.Context, _ CommandContext, args []string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for _, arg := range args {
		if arg != "show" {
			return i18n.T("permissions.view_only"), nil
		}
	}
	engine, err := permissions.Default()
	if err != nil {
		return "", err
	}
	profile, err := engine.Profile()
	if err != nil {
		return "", err
	}
	project, err := os.Getwd()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s: %s\n%s: %s\n", i18n.T("permissions.profile"), profile, i18n.T("permissions.project"), project)
	for _, rule := range engine.Explain(ctx, project, "") {
		fmt.Fprintf(&out, "- %s — %s: %s (%s)", rule.Permission, i18n.T("permission."+string(rule.Permission)), rule.Effect, rule.Scope)
		if rule.Effect == permissions.Ask {
			fmt.Fprintf(&out, " — %s", i18n.T("permissions.approval_required"))
		}
		out.WriteByte('\n')
	}
	return strings.TrimSpace(out.String()), nil
}
