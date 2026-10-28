package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func init() {
	command := Command{Name: "/permissions", Aliases: []string{"/permissoes"}, Summary: "command.permissions", Usage: "/permissions [show|set-profile|allow|deny|ask|reset]", Category: "general", Handler: permissionsCommand}
	Register(command)
}

func permissionsCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	engine := permissions.New(cc.ConfigHome(), trust.NewStore(cc.ConfigHome()))
	if len(args) == 0 || (len(args) == 1 && args[0] == "show") {
		return showPermissions(ctx, engine, cc.ProjectPath())
	}
	switch args[0] {
	case "set-profile":
		return setPermissionProfile(ctx, cc, engine, args[1:])
	case "allow", "deny", "ask":
		return setPermissionRule(ctx, cc, engine, permissions.Effect(args[0]), args[1:])
	case "reset":
		return resetPermissions(ctx, cc, engine, args[1:])
	default:
		return "", errors.New(i18n.T("permissions.usage"))
	}
}

func showPermissions(ctx context.Context, engine *permissions.Engine, project string) (string, error) {
	profile, err := engine.Profile()
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

func setPermissionProfile(ctx context.Context, cc CommandContext, engine *permissions.Engine, args []string) (string, error) {
	if len(args) != 1 || (args[0] != "restricted" && args[0] != "balanced" && args[0] != "autonomous") {
		return "", errors.New(i18n.T("permissions.usage"))
	}
	current, err := engine.Profile()
	if err != nil {
		return "", err
	}
	if profileRank(args[0]) > profileRank(current) {
		if err := confirmPolicyChange(ctx, cc, engine, permissions.UserScope, "execute_commands", "profile "+args[0]); err != nil {
			return "", err
		}
	}
	if err := engine.SetProfile(args[0]); err != nil {
		return "", err
	}
	return i18n.T("permissions.profile.changed", args[0]), nil
}

func setPermissionRule(ctx context.Context, cc CommandContext, engine *permissions.Engine, effect permissions.Effect, args []string) (string, error) {
	if len(args) < 1 || !permissions.ValidPermission(permissions.Permission(args[0])) {
		return "", errors.New(i18n.T("permissions.usage"))
	}
	permission := permissions.Permission(args[0])
	scope, project, agent, err := parsePermissionScope(cc, args[1:])
	if err != nil {
		return "", err
	}
	if scope != permissions.UserScope && effect == permissions.Allow {
		request := permissions.Request{Action: permission, Project: project, Agent: agent}
		if engine.Decide(ctx, request).Effect != permissions.Allow {
			return "", errors.New(i18n.T("permissions.no_self_elevation"))
		}
	}
	before := engine.Decide(ctx, permissions.Request{Action: permission, Project: project, Agent: agent})
	if scope == permissions.UserScope && permissionEffectRank(effect) > permissionEffectRank(before.Effect) {
		if err := confirmPolicyChange(ctx, cc, engine, scope, permission, string(effect)+" "+string(permission)); err != nil {
			return "", err
		}
	}
	if err := engine.SetRule(scope, project, agent, permission, effect); err != nil {
		return "", err
	}
	return i18n.T("permissions.rule.changed", effect, permission, scope), nil
}

func resetPermissions(ctx context.Context, cc CommandContext, engine *permissions.Engine, args []string) (string, error) {
	scope, project, agent, err := parsePermissionScope(cc, args)
	if err != nil {
		return "", err
	}
	if err := confirmPolicyChange(ctx, cc, engine, scope, "write_project", "reset permission rules"); err != nil {
		return "", err
	}
	if err := engine.Reset(scope, project, agent); err != nil {
		return "", err
	}
	return i18n.T("permissions.reset.done", scope), nil
}

func parsePermissionScope(cc CommandContext, args []string) (permissions.Scope, string, string, error) {
	scope, project, agent := permissions.UserScope, "", ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			if project != "" || agent != "" {
				return "", "", "", errors.New(i18n.T("permissions.usage"))
			}
			if !cc.Trusted() || cc.ProjectPath() == "" {
				return "", "", "", errors.New(i18n.T("permissions.project_not_trusted"))
			}
			scope, project = permissions.ProjectScope, cc.ProjectPath()
		case "--agent":
			if agent != "" || project != "" || i+1 >= len(args) {
				return "", "", "", errors.New(i18n.T("permissions.usage"))
			}
			i++
			agent = args[i]
			if agent == "" {
				return "", "", "", errors.New(i18n.T("permissions.usage"))
			}
			scope = permissions.AgentScope
		default:
			return "", "", "", errors.New(i18n.T("permissions.usage"))
		}
	}
	return scope, project, agent, nil
}

type policyConfirmer interface {
	Confirm(PermissionRequest) (bool, error)
}

func confirmPolicyChange(ctx context.Context, cc CommandContext, engine *permissions.Engine, scope permissions.Scope, permission permissions.Permission, summary string) error {
	confirmer, ok := cc.(policyConfirmer)
	if !ok {
		return errors.New(i18n.T("permissions.approval_unavailable"))
	}
	approved, err := confirmer.Confirm(PermissionRequest{Agent: "user", Action: string(permission), Target: summary, ExactCommand: "/permissions " + summary})
	if err != nil {
		return err
	}
	event := permissions.ApprovalEvent{Permission: permission, PolicyRevision: "current", Operation: struct {
		Summary string `json:"summary"`
		Digest  string `json:"digest,omitempty"`
	}{Summary: summary}}
	event.Decision = permissions.Deny
	if approved {
		event.Decision = permissions.Allow
	}
	event.Actor.Kind = "user"
	event.Requester.AgentID = "user"
	event.Scope.Kind = string(scope)
	event.Scope.ID = cc.ProjectPath()
	if err := engine.RecordApproval(event); err != nil {
		return fmt.Errorf("record permission change approval: %w", err)
	}
	if !approved {
		return errors.New(i18n.T("permissions.change.denied"))
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func profileRank(profile string) int {
	switch profile {
	case "restricted":
		return 0
	case "balanced":
		return 1
	case "autonomous":
		return 2
	default:
		return -1
	}
}

func permissionEffectRank(effect permissions.Effect) int {
	switch effect {
	case permissions.Deny:
		return 0
	case permissions.Ask:
		return 1
	case permissions.Allow:
		return 2
	default:
		return -1
	}
}
