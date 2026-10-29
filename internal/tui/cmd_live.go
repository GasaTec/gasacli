package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/sessions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func init() {
	Register(Command{Name: "/sessions", Summary: "command.sessions", Usage: "/sessions", Category: "sessions", Handler: listSessionsCommand})
	Register(Command{Name: "/send", Summary: "command.send", Usage: "/send <sessao> <mensagem>", Category: "sessions", Handler: sendSessionCommand})
	Register(Command{Name: "/use", Summary: "command.use", Usage: "/use [sessao]", Category: "sessions", Handler: func(context.Context, CommandContext, []string) (string, error) {
		return "A seleção de sessão está disponível no modo interativo.", nil
	}})
}

type liveSessionContext interface {
	listSessions() []sessions.Session
	sendToSession(context.Context, sessions.SendRequest) (<-chan sessions.Event, error)
}

func listSessionsWithContext(ctx context.Context, provider liveSessionContext) ([]sessions.Session, error) {
	if contextual, ok := provider.(interface {
		listSessionsContext(context.Context) ([]sessions.Session, error)
	}); ok {
		return contextual.listSessionsContext(ctx)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	items := provider.listSessions()
	return items, ctx.Err()
}

func listSessionsCommand(ctx context.Context, cc CommandContext, _ []string) (string, error) {
	provider, ok := cc.(liveSessionContext)
	if !ok {
		return "Sessões locais indisponíveis.", nil
	}
	items, err := listSessionsWithContext(ctx, provider)
	if err != nil {
		return "", err
	}
	if len(items) == 0 {
		return "Nenhuma sessão encontrada para este projeto confiável.", nil
	}
	var output strings.Builder
	for _, item := range items {
		fmt.Fprintf(&output, "%s  %s  %s\n", shortSessionID(item.ID), item.Agent, item.Title)
	}
	return strings.TrimSpace(output.String()), nil
}

func sendSessionCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if len(args) < 2 {
		return "Uso: /send <sessao> <mensagem>", nil
	}
	return sendLiveSession(ctx, cc, args[0], strings.Join(args[1:], " "))
}

func sendLiveSession(ctx context.Context, cc CommandContext, query, message string) (string, error) {
	provider, ok := cc.(liveSessionContext)
	if !ok {
		return "Envio para sessões locais indisponível.", nil
	}
	items, err := listSessionsWithContext(ctx, provider)
	if err != nil {
		return "", err
	}
	id, matches := findSession(items, query)
	if len(matches) == 0 {
		return "Sessão não encontrada neste projeto confiável.", nil
	}
	if len(matches) > 1 {
		return "ID curto ambíguo; use mais caracteres do ID.", nil
	}
	if strings.TrimSpace(message) == "" {
		return "A mensagem não pode ficar vazia.", nil
	}
	commandSummary := "/send " + id + " [mensagem omitida do log]"
	if matches[0].Status == "working" {
		commandSummary += " (sessão com atividade recente; confirme que pode continuar)"
	}
	for _, action := range []permissions.Permission{"execute_commands", "network_access"} {
		approved, err := cc.Ask(PermissionRequest{Agent: agentPermissionID(matches[0].Agent), Action: string(action), Target: "Projeto: " + cc.ProjectPath() + "\nMensagem: " + message, ExactCommand: commandSummary + " (acessa o provedor de IA)"})
		if err != nil || !approved {
			if err != nil {
				return "Envio negado ou não aprovado.", nil
			}
			return "Envio negado.", nil
		}
	}
	// The sender records its own audit event before launching the provider.
	approvedContext := sessions.WithExpectedProject(sessions.WithApproval(ctx), cc.ProjectPath())
	stream, err := provider.sendToSession(approvedContext, sessions.SendRequest{SessionID: id, Text: message})
	if err != nil {
		return "Falha ao iniciar envio: " + terminalText(err.Error()), nil
	}
	var output strings.Builder
	var pending strings.Builder
	commandContext, sessionContext := cc.(*sessionCommandContext)
	collect := sessionContext && commandContext.emit == nil
	gotText := false
	flush := func() {
		if pending.Len() > 0 {
			cc.Emit(terminalText(pending.String()))
			pending.Reset()
		}
	}
	for event := range stream {
		switch event.Type {
		case "text":
			gotText = gotText || event.Text != ""
			if collect {
				// CLISender caps provider output at 1 MiB before emitting events.
				output.WriteString(event.Text)
			}
			for _, char := range event.Text {
				// Bound a single unterminated line without splitting UTF-8.
				if pending.Len() >= 64<<10 {
					flush()
				}
				pending.WriteRune(char)
				if char == '\n' {
					flush()
				}
			}
		case "error":
			flush()
			if collect && output.Len() > 0 {
				return strings.TrimSpace(terminalText(output.String())) + "\nFalha no envio: " + terminalText(event.Err), nil
			}
			return "Falha no envio: " + terminalText(event.Err), nil
		}
	}
	flush()
	if !gotText {
		return "Envio concluído sem texto de resposta.", nil
	}
	if collect {
		return strings.TrimSpace(terminalText(output.String())), nil
	}
	return "", nil
}

func (c *sessionCommandContext) listSessions() []sessions.Session {
	items, _ := c.listSessionsContext(context.Background())
	return items
}

func (c *sessionCommandContext) listSessionsContext(ctx context.Context) ([]sessions.Session, error) {
	if !c.trusted {
		return nil, nil
	}
	items, err := sessions.ScanMetadataContext(ctx, sessions.DefaultRoots())
	if err != nil {
		return nil, err
	}
	return filterSessionsByProject(ctx, items, c.project)
}

func filterSessionsByProject(ctx context.Context, items []sessions.Session, project string) ([]sessions.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	project, err := trust.Canonical(project)
	if err != nil {
		return nil, err
	}
	projectInfo, err := os.Stat(project)
	if err != nil {
		return nil, err
	}
	var filtered []sessions.Session
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		root, err := sessions.ProjectRootContext(ctx, item.CWD)
		if err != nil {
			return nil, err
		}
		if root == "" {
			continue
		}
		root, err = trust.Canonical(root)
		if err != nil {
			continue
		}
		rootInfo, err := os.Stat(root)
		if err == nil && os.SameFile(projectInfo, rootInfo) {
			filtered = append(filtered, item)
		}
	}
	return filtered, ctx.Err()
}

func (c *sessionCommandContext) sendToSession(ctx context.Context, request sessions.SendRequest) (<-chan sessions.Event, error) {
	if !c.trusted {
		return nil, fmt.Errorf("project is not trusted")
	}
	policy := permissions.New(c.session.ConfigHome, c.session.TrustStore)
	sender := sessions.NewCLISender(sessions.DefaultRoots(), policy, nil)
	return sender.Send(ctx, request)
}

func shortSessionID(id string) string {
	if index := strings.IndexByte(id, '-'); index >= 0 && len(id) > index+8 {
		return id[:index+1] + id[index+1:index+9]
	}
	return id
}

func findSession(items []sessions.Session, query string) (string, []sessions.Session) {
	var matches []sessions.Session
	for _, item := range items {
		if item.ID == query || shortSessionID(item.ID) == query || strings.HasPrefix(strings.TrimPrefix(item.ID, strings.SplitN(item.ID, "-", 2)[0]+"-"), query) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 1 {
		return matches[0].ID, matches
	}
	return "", matches
}

func agentPermissionID(agent string) string {
	if strings.EqualFold(agent, "Claude Code") {
		return "claude"
	}
	if strings.EqualFold(agent, "Codex") {
		return "codex"
	}
	return "unsupported"
}
