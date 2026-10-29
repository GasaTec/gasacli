package tui

import (
	"context"
	"errors"
	"strings"

	"github.com/GasaTec/gasacli/internal/sessions"
)

// selectLiveSession resolves only sessions belonging to the trusted project.
// Callers own the selection and must clear it when the project changes.
func selectLiveSession(ctx context.Context, session *Session, project string, trusted bool, query string) (sessions.Session, error) {
	if session == nil || !trusted || strings.TrimSpace(query) == "" {
		return sessions.Session{}, errors.New("Selecione uma sessão deste projeto confiável.")
	}
	cc := &sessionCommandContext{session: session, project: project, trusted: trusted}
	items, err := cc.listSessionsContext(ctx)
	if err != nil {
		return sessions.Session{}, err
	}
	_, matches := findSession(items, strings.TrimSpace(query))
	if len(matches) == 0 {
		return sessions.Session{}, errors.New("Sessão não encontrada neste projeto confiável.")
	}
	if len(matches) != 1 {
		return sessions.Session{}, errors.New("ID curto ambíguo; use mais caracteres do ID.")
	}
	return matches[0], nil
}

// selectRecentLiveSession selects only a session for the trusted project. If
// no provider was chosen, the newest resumable session determines the provider.
func selectRecentLiveSession(ctx context.Context, session *Session, project string, trusted bool, agent string) (sessions.Session, error) {
	if session == nil || !trusted {
		return sessions.Session{}, errors.New("Confirme a confiança neste projeto antes de acessar sessões.")
	}
	cc := &sessionCommandContext{session: session, project: project, trusted: trusted}
	items, err := cc.listSessionsContext(ctx)
	if err != nil {
		return sessions.Session{}, err
	}
	return recentSessionForAgent(items, agent)
}

func recentSessionForAgent(items []sessions.Session, agent string) (sessions.Session, error) {
	var newest sessions.Session
	for _, item := range items {
		if !sessions.Resumable(item) || item.Status == "working" || (agent != "" && !strings.EqualFold(item.Agent, agent)) {
			continue
		}
		if newest.ID == "" || item.UpdatedAt.After(newest.UpdatedAt) {
			newest = item
		}
	}
	if newest.ID == "" {
		return sessions.Session{}, errors.New("Nenhuma conversa recente disponível para este agente neste projeto.")
	}
	return newest, nil
}

// executeLiveChat shares /send's permission, trust and audit path. Text is not
// parsed as a command, preserving quotes and newlines.
func executeLiveChat(ctx context.Context, session *Session, project string, trusted bool, id, text string, emit func(string), ask func(context.Context, PermissionRequest) (ApprovalChoice, error)) (string, error) {
	if session == nil || !trusted || strings.TrimSpace(id) == "" {
		return "Selecione uma sessão deste projeto confiável com /use.", nil
	}
	cc := &sessionCommandContext{session: session, ctx: ctx, project: project, trusted: trusted, command: "/send [mensagem omitida do log]", emit: emit, ask: ask}
	return sendLiveSession(ctx, cc, id, text)
}
