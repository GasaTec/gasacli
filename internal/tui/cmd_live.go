package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/sessions"
)

func init() {
	Register(Command{Name: "sessions", Summary: "command.sessions", Usage: "/sessions", Category: "sessions", Handler: listSessionsCommand})
	Register(Command{Name: "send", Summary: "command.send", Usage: "/send <sessao> <mensagem>", Category: "sessions", Handler: sendSessionCommand})
}

type liveSessionContext interface {
	listSessions() []sessions.Session
	sendToSession(context.Context, sessions.SendRequest) (<-chan sessions.Event, error)
}

func listSessionsCommand(_ context.Context, cc CommandContext, _ []string) (string, error) {
	provider, ok := cc.(liveSessionContext)
	if !ok {
		return "Sessões locais indisponíveis.", nil
	}
	items := provider.listSessions()
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
	provider, ok := cc.(liveSessionContext)
	if !ok {
		return "Envio para sessões locais indisponível.", nil
	}
	id, matches := findSession(provider.listSessions(), args[0])
	if len(matches) == 0 {
		return "Sessão não encontrada neste projeto confiável.", nil
	}
	if len(matches) > 1 {
		return "ID curto ambíguo; use mais caracteres do ID.", nil
	}
	message := strings.Join(args[1:], " ")
	if strings.TrimSpace(message) == "" {
		return "A mensagem não pode ficar vazia.", nil
	}
	commandSummary := "/send " + id + " [mensagem omitida do log]"
	if matches[0].Status == "working" {
		commandSummary += " (sessão com atividade recente; confirme que pode continuar)"
	}
	approved, err := cc.Ask(PermissionRequest{Agent: agentPermissionID(matches[0].Agent), Action: string(permissions.Permission("execute_commands")), Target: "Projeto: " + cc.ProjectPath() + "\nMensagem: " + message, ExactCommand: commandSummary + " (acessa o provedor de IA)"})
	if err != nil || !approved {
		if err != nil {
			return "Envio negado ou não aprovado.", nil
		}
		return "Envio negado.", nil
	}
	approvedContext := sessions.WithAuditedApproval(sessions.WithApproval(ctx))
	stream, err := provider.sendToSession(approvedContext, sessions.SendRequest{SessionID: id, Text: message})
	if err != nil {
		return "Falha ao iniciar envio: " + err.Error(), nil
	}
	var output strings.Builder
	for event := range stream {
		switch event.Type {
		case "text":
			output.WriteString(event.Text)
			cc.Emit(event.Text)
		case "error":
			return "Falha no envio: " + event.Err, nil
		}
	}
	return strings.TrimSpace(output.String()), nil
}

func (c *sessionCommandContext) listSessions() []sessions.Session {
	if !c.trusted {
		return nil
	}
	items := sessions.ScanMetadata(sessions.DefaultRoots())
	var filtered []sessions.Session
	for _, item := range items {
		if sessions.ProjectRoot(item.CWD) == c.project {
			filtered = append(filtered, item)
		}
	}
	return filtered
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
