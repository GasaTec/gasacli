package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
)

// sessionCommandContext is the capability set shared by interactive and plain
// command execution. Optional integrations are deliberately passed in by the
// owning Session so tui does not depend on application packages.
type sessionCommandContext struct {
	session *Session
	ctx     context.Context
	project string
	trusted bool
	command string
	emit    func(string)
	ask     func(context.Context, PermissionRequest) (bool, error)
}

func (c *sessionCommandContext) ProjectPath() string { return c.project }
func (c *sessionCommandContext) Trusted() bool       { return c.trusted }
func (c *sessionCommandContext) ConfigHome() string  { return c.session.ConfigHome }
func (c *sessionCommandContext) DiscoveryResults() any {
	if c.session.DiscoveryResults != nil {
		return c.session.DiscoveryResults()
	}
	return nil
}
func (c *sessionCommandContext) Storage() any { return c.session.Storage }
func (c *sessionCommandContext) Notify(message string) {
	if c.session.Notify != nil {
		c.session.Notify(message)
	}
}
func (c *sessionCommandContext) AppendMessage(message string) { c.Emit(message) }
func (c *sessionCommandContext) Emit(message string) {
	if c.emit != nil && message != "" {
		c.emit(message)
	}
}
func (c *sessionCommandContext) Ask(request PermissionRequest) (bool, error) {
	if request.ExactCommand == "" {
		request.ExactCommand = c.command
	}
	engine := permissions.New(c.session.ConfigHome, c.session.TrustStore)
	approver := sessionApprover{engine: engine, prompt: c.ask, project: c.project, request: request}
	decision, err := approver.Ask(c.ctx, permissions.Request{Agent: request.Agent, Action: permissions.Permission(request.Action), Target: request.Target, Project: c.project})
	if err != nil {
		return false, err
	}
	return decision.Effect == permissions.Allow, nil
}

type sessionApprover struct {
	engine  *permissions.Engine
	prompt  func(context.Context, PermissionRequest) (bool, error)
	project string
	request PermissionRequest
}

func (a sessionApprover) Ask(ctx context.Context, request permissions.Request) (permissions.Decision, error) {
	decision := a.engine.Decide(ctx, request)
	switch decision.Effect {
	case permissions.Allow:
		return decision, nil
	case permissions.Deny:
		return decision, fmt.Errorf("permission denied: %s", decision.Reason)
	case permissions.Ask:
		if a.prompt == nil {
			return decision, fmt.Errorf("approval is unavailable")
		}
		approved, err := a.prompt(ctx, a.request)
		if err != nil {
			return decision, err
		}
		event := permissions.ApprovalEvent{Decision: permissions.Deny, Permission: request.Action, PolicyRevision: "current"}
		if approved {
			event.Decision = permissions.Allow
		}
		event.Actor.Kind = "user"
		event.Requester.AgentID = request.Agent
		event.Scope.Kind = string(decision.Scope)
		event.Scope.ID = a.project
		event.Operation.Summary = a.request.ExactCommand
		if event.Operation.Summary == "" {
			event.Operation.Summary = string(request.Action) + " " + request.Target
		}
		if err := a.engine.RecordApproval(event); err != nil {
			return decision, fmt.Errorf("record permission decision: %w", err)
		}
		if approved {
			decision.Effect = permissions.Allow
		} else {
			decision.Effect = permissions.Deny
		}
		decision.Reason = "user decision"
		return decision, nil
	default:
		return decision, fmt.Errorf("permission denied: invalid policy effect")
	}
}

func splitCommand(input string) (string, []string, error) {
	fields, err := shellWords(input)
	if err != nil {
		return "", nil, err
	}
	if len(fields) == 0 {
		return "", nil, nil
	}
	return fields[0], fields[1:], nil
}

// shellWords implements the small quoting subset used by slash commands. It
// supports single and double quotes and backslash escapes without invoking a
// shell or expanding environment variables.
func shellWords(input string) ([]string, error) {
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, r := range input {
		if escaped {
			word.WriteRune(r)
			escaped, started = false, true
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped, started = true, true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote, started = r, true
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if started {
				words = append(words, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("unterminated escape or quote")
	}
	if started {
		words = append(words, word.String())
	}
	return words, nil
}

func executeCommand(ctx context.Context, session *Session, project string, trusted bool, input string, emit func(string), ask func(context.Context, PermissionRequest) (bool, error)) (result string, err error) {
	name, args, err := splitCommand(input)
	if err != nil {
		return "", err
	}
	command, ok := lookupCommand(name)
	if !ok {
		return unknownCommandMessage(name), nil
	}
	handler := command.Handler
	if handler == nil {
		handler = builtinHandler(command.Name, session)
	}
	if handler == nil {
		return "", nil
	}
	commandContext := &sessionCommandContext{session: session, ctx: ctx, project: project, trusted: trusted, command: input, emit: emit, ask: ask}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = ""
			err = fmt.Errorf("command handler panic: %v", recovered)
		}
	}()
	return handler(ctx, commandContext, args)
}

func builtinHandler(name string, session *Session) CommandHandler {
	switch name {
	case "/help":
		return func(context.Context, CommandContext, []string) (string, error) { return helpText(), nil }
	case "/agents":
		return func(_ context.Context, _ CommandContext, _ []string) (string, error) {
			return callbackText(session.Agents, "Agent discovery is not configured."), nil
		}
	case "/doctor":
		return func(_ context.Context, _ CommandContext, _ []string) (string, error) {
			return callbackText(session.Doctor, "Diagnostics are not configured."), nil
		}
	case "/setup":
		return func(_ context.Context, _ CommandContext, _ []string) (string, error) {
			if session.Interactive {
				return "", errSetupCommand
			}
			if session.Setup == nil {
				return "Setup is not configured.", nil
			}
			var output strings.Builder
			err := session.Setup(&output)
			return output.String(), err
		}
	case "/status":
		return func(context.Context, CommandContext, []string) (string, error) {
			return i18n.T("command.status.summary", session.ProjectPath, valueOr(session.Dashboard, "not started"), valueOr(session.DashboardState, "local"), fmt.Sprint(session.ReadyAgents), "#11"), nil
		}
	case "/web":
		return func(_ context.Context, _ CommandContext, args []string) (string, error) {
			if session.Web == nil {
				return "Dashboard is not configured.", nil
			}
			var output strings.Builder
			if err := session.Web(&output, args); err != nil {
				return output.String(), err
			}
			return output.String(), nil
		}
	case "/exit", "/quit":
		return func(context.Context, CommandContext, []string) (string, error) { return "", errExitCommand }
	default:
		return nil
	}
}

var errExitCommand = fmt.Errorf("exit command")
var errSetupCommand = fmt.Errorf("setup command")

func callbackText(callback func(io.Writer), fallback string) string {
	if callback == nil {
		return fallback
	}
	var output strings.Builder
	callback(&output)
	return strings.TrimRight(output.String(), "\n")
}
