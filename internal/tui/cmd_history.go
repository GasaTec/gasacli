package tui

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/GasaTec/gasacli/internal/sessions"
	"github.com/GasaTec/gasacli/internal/trust"
	"github.com/charmbracelet/x/ansi"
)

func init() {
	Register(Command{Name: "/history", Summary: "command.history", Usage: "/history <session>", Category: "sessions", Handler: historyCommand})
}

func historyCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if !cc.Trusted() {
		return "", fmt.Errorf("trust this project before reading session history")
	}
	if len(args) != 1 {
		return "Usage: /history <session>", nil
	}
	provider, ok := cc.(liveSessionContext)
	if !ok {
		return "Session history is unavailable.", nil
	}
	items, err := listSessionsWithContext(ctx, provider)
	if err != nil {
		return "", err
	}
	_, matches := findSession(items, args[0])
	if len(matches) != 1 {
		return "Session not found or ambiguous in this trusted project.", nil
	}
	item := matches[0]
	loaded, ok := sessions.LoadTranscript(item.Agent, item.SourcePath)
	if !ok || loaded.ID != item.ID {
		return "", fmt.Errorf("session history changed or is unavailable")
	}
	project, err := trust.Canonical(cc.ProjectPath())
	if err != nil {
		return "", err
	}
	if current, ok := cc.(*sessionCommandContext); ok {
		if current.session == nil || current.session.TrustStore == nil {
			return "", fmt.Errorf("project trust is unavailable")
		}
		approved, trustErr := current.session.TrustStore.IsTrusted(project)
		if trustErr != nil || !approved {
			return "", fmt.Errorf("project trust was revoked or cannot be verified")
		}
	}
	root, err := trust.Canonical(sessions.ProjectRoot(loaded.CWD))
	if err != nil || root != project {
		return "", fmt.Errorf("session history does not belong to this project")
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s (%s)\n", terminalText(loaded.Agent), shortSessionID(loaded.ID))
	for _, message := range loaded.Messages {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if out.Len() >= 512<<10 {
			out.WriteString("\n[history truncated]")
			break
		}
		if message.Text != "" {
			fmt.Fprintf(&out, "\n%s\n%s\n", terminalText(message.Role), terminalText(message.Text))
		}
		for _, call := range message.ToolCalls {
			fmt.Fprintf(&out, "\n%s: %s\n", terminalText(call.Kind), terminalText(call.Summary))
			if call.Output != "" {
				fmt.Fprintf(&out, "%s\n", terminalText(call.Output))
			}
		}
	}
	return strings.TrimSpace(out.String()), nil
}

// Provider transcripts are untrusted terminal input, including escape sequences.
func terminalText(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, ansi.Strip(value))
}
