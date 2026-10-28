package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/GasaTec/gasacli/internal/permissions"
	ghplugin "github.com/GasaTec/gasacli/internal/plugin/github"
	"github.com/GasaTec/gasacli/internal/trust"
)

func init() {
	Register(Command{Name: "/github", Summary: "command.github", Usage: "/github [auth|issues [state] [filters]|issue <n>|comment <n> <text>|enable|disable]", Category: "integrations", Handler: githubCommand})
}

type commandApprover struct{ command CommandContext }

func (a commandApprover) Ask(ctx context.Context, req permissions.Request) (permissions.Decision, error) {
	approved, err := a.command.Ask(PermissionRequest{Action: string(req.Action), Target: req.Target})
	decision := permissions.Decision{Rule: req.Action}
	if approved {
		decision.Effect = permissions.Allow
	} else {
		decision.Effect = permissions.Deny
	}
	return decision, err
}

func githubCommand(ctx context.Context, command CommandContext, args []string) (string, error) {
	if command == nil || !command.Trusted() {
		return "", errors.New("GitHub commands require a trusted project")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	client, err := newGitHubClient(command)
	if err != nil {
		return "", err
	}
	action := "status"
	if len(args) > 0 {
		action, args = strings.ToLower(args[0]), args[1:]
	}
	switch action {
	case "status":
		repo, err := client.Repository(ctx)
		if err != nil {
			return "", err
		}
		enabled, err := client.OptedIn()
		if err != nil {
			return "", err
		}
		status, err := client.Auth(ctx)
		if err != nil {
			return "", err
		}
		return ghplugin.FormatStatus(repo, enabled, status), nil
	case "enable", "disable":
		if len(args) != 0 {
			return "", errors.New("usage: /github enable|disable")
		}
		if _, err := client.Repository(ctx); err != nil {
			return "", err
		}
		enabled := action == "enable"
		if err := client.SetOptIn(enabled); err != nil {
			return "", err
		}
		return ghplugin.FormatOptInChanged(enabled), nil
	case "auth":
		status, err := client.Auth(ctx)
		if err != nil {
			return "", err
		}
		return ghplugin.FormatAuth(status), nil
	case "issues":
		filters, err := ghplugin.ParseIssueFilters(args)
		if err != nil {
			return "", err
		}
		issues, err := client.ListIssuesWithFilters(ctx, filters)
		if err != nil {
			return "", err
		}
		return ghplugin.FormatIssues(issues), nil
	case "issue":
		if len(args) != 1 {
			return "", errors.New("usage: /github issue <number>")
		}
		number, err := strconv.Atoi(args[0])
		if err != nil {
			return "", errors.New("issue number must be a positive integer")
		}
		issue, err := client.ViewIssue(ctx, number)
		if err != nil {
			return "", err
		}
		return formatIssue(issue), nil
	case "comment":
		if len(args) < 2 {
			return "", errors.New("usage: /github comment <number> <text>")
		}
		number, err := strconv.Atoi(args[0])
		if err != nil {
			return "", errors.New("issue number must be a positive integer")
		}
		return client.Comment(ctx, number, strings.Join(args[1:], " "))
	default:
		return "", errors.New("usage: /github [auth|issues [state]|issue <n>|comment <n> <text>|enable|disable]")
	}
}

func newGitHubClient(command CommandContext) (*ghplugin.Client, error) {
	policy, err := permissions.Default()
	if err != nil {
		return nil, err
	}
	return ghplugin.New(ghplugin.Options{Project: command.ProjectPath(), Home: command.ConfigHome(), Trust: trust.NewStore(command.ConfigHome()), Policy: policy, Approver: commandApprover{command: command}})
}

func formatIssue(issue ghplugin.Issue) string {
	data, _ := json.MarshalIndent(issue, "", "  ")
	return string(data)
}
