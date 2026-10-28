package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
	ghplugin "github.com/GasaTec/gasacli/internal/plugin/github"
	"github.com/GasaTec/gasacli/internal/trust"
)

type terminalIssueApprover struct {
	in  io.Reader
	out io.Writer
}

func (a terminalIssueApprover) Ask(_ context.Context, req permissions.Request) (permissions.Decision, error) {
	fmt.Fprintf(a.out, "GitHub write requested (%s):\n%s\nApprove exactly this operation? [y/N]: ", req.Action, req.Target)
	line, err := bufio.NewReader(a.in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return permissions.Decision{}, err
	}
	if strings.EqualFold(strings.TrimSpace(line), "y") {
		return permissions.Decision{Effect: permissions.Allow, Rule: req.Action}, nil
	}
	return permissions.Decision{Effect: permissions.Deny, Rule: req.Action}, nil
}

func runGitHub(ctx context.Context, args []string, in io.Reader, out io.Writer) error {
	dryRun := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--dry-run" {
			dryRun = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	args = filtered
	if len(args) == 0 {
		args = []string{"status"}
	}
	project, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	language := ""
	if state, stateErr := config.LoadState(home); stateErr == nil {
		language = state.Onboarding.Language
	}
	i18n.Configure(language)
	policy, err := permissions.Default()
	if err != nil {
		return err
	}
	client, err := ghplugin.New(ghplugin.Options{Project: project, Home: home, Trust: trust.NewStore(home), Policy: policy, Approver: terminalIssueApprover{in: in, out: out}, DryRun: dryRun})
	if err != nil {
		return err
	}
	action := strings.ToLower(args[0])
	args = args[1:]
	var result string
	switch action {
	case "enable", "disable":
		if len(args) != 0 {
			return errors.New("usage: gasa github enable|disable")
		}
		if _, err := client.Repository(ctx); err != nil {
			return err
		}
		if err := client.SetOptIn(action == "enable"); err != nil {
			return err
		}
		result = ghplugin.FormatOptInChanged(action == "enable")
	case "auth":
		if len(args) != 0 {
			return errors.New("usage: gasa github auth")
		}
		status, err := client.Auth(ctx)
		if err != nil {
			return err
		}
		result = ghplugin.FormatAuth(status)
	case "status":
		repo, err := client.Repository(ctx)
		if err != nil {
			return err
		}
		enabled, err := client.OptedIn()
		if err != nil {
			return err
		}
		status, err := client.Auth(ctx)
		if err != nil {
			return err
		}
		result = ghplugin.FormatStatus(repo, enabled, status)
	case "issues":
		filters, err := ghplugin.ParseIssueFilters(args)
		if err != nil {
			return err
		}
		issues, err := client.ListIssuesWithFilters(ctx, filters)
		if err != nil {
			return err
		}
		result = ghplugin.FormatIssues(issues)
	case "issue":
		if len(args) != 1 {
			return errors.New("usage: gasa github issue <number>")
		}
		number, err := strconv.Atoi(args[0])
		if err != nil {
			return errors.New("issue number must be a positive integer")
		}
		issue, err := client.ViewIssue(ctx, number)
		if err != nil {
			return err
		}
		result = formatGitHubIssue(issue)
	case "comment":
		if len(args) < 2 {
			return errors.New("usage: gasa github comment <number> <text> [--dry-run]")
		}
		number, err := strconv.Atoi(args[0])
		if err != nil {
			return errors.New("issue number must be a positive integer")
		}
		result, err = client.Comment(ctx, number, strings.Join(args[1:], " "))
		if err != nil {
			return err
		}
	case "create":
		fs := flag.NewFlagSet("gasa github create", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		title := fs.String("title", "", "issue title")
		body := fs.String("body", "", "issue body")
		var labels, assignees string
		fs.StringVar(&labels, "label", "", "comma-separated labels")
		fs.StringVar(&assignees, "assignee", "", "comma-separated assignees")
		if err := fs.Parse(args); err != nil {
			return err
		}
		result, err = client.CreateIssue(ctx, *title, *body, splitCSV(labels), splitCSV(assignees))
		if err != nil {
			return err
		}
	default:
		return errors.New("usage: gasa github [status|auth|issues [state]|issue <n>|create --title T --body B|comment <n> <text>|enable|disable] [--dry-run]")
	}
	_, err = fmt.Fprintln(out, result)
	return err
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	output := parts[:0]
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			output = append(output, part)
		}
	}
	return output
}

func formatGitHubIssue(issue ghplugin.Issue) string {
	data, _ := json.MarshalIndent(issue, "", "  ")
	return string(data)
}
