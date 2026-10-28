package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/sessions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func sendAPI(home string, sender sessions.Sender, getSnapshot func() snapshot, w http.ResponseWriter, r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/sessions/") || !strings.HasSuffix(r.URL.Path, "/send") || r.Method != http.MethodPost {
		return false
	}
	if sender == nil || !sender.Capabilities().Send {
		http.Error(w, "sending is unavailable", http.StatusNotImplemented)
		return true
	}
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/send")
	if id == "" || strings.ContainsAny(id, `/\\`) {
		http.NotFound(w, r)
		return true
	}
	var payload struct {
		Text     string `json:"text"`
		Approved bool   `json:"approved"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 40<<10))
	if decoder.Decode(&payload) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "invalid message", http.StatusBadRequest)
		return true
	}
	if strings.TrimSpace(payload.Text) == "" || len(payload.Text) > 32<<10 {
		http.Error(w, "message must contain 1 to 32768 bytes", http.StatusBadRequest)
		return true
	}
	snapshot := getSnapshot()
	var target sessions.Session
	var project projectView
	for _, candidate := range snapshot.projects {
		if !candidate.Trusted {
			continue
		}
		for _, item := range candidate.Sessions {
			if item.ID == id {
				target, project = item, candidate
				break
			}
		}
	}
	if target.ID == "" {
		http.NotFound(w, r)
		return true
	}
	policy := permissions.New(home, trust.NewStore(home))
	needsApproval := false
	for _, action := range []permissions.Permission{"execute_commands", "network_access"} {
		decision := policy.Decide(r.Context(), permissions.Request{Agent: sendAgentID(target.Agent), Action: action, Project: project.Path, Target: "Send a message to " + target.Agent + " session " + target.ID})
		if decision.Effect == permissions.Deny {
			http.Error(w, "sending is denied by policy", http.StatusForbidden)
			return true
		}
		needsApproval = needsApproval || decision.Effect == permissions.Ask
	}
	if needsApproval && !payload.Approved {
		w.WriteHeader(http.StatusPreconditionRequired)
		jsonResponse(w, map[string]string{"agent": target.Agent, "session": target.Title, "sessionId": target.ID, "project": project.Name, "text": payload.Text})
		return true
	}
	ctx := r.Context()
	if payload.Approved {
		ctx = sessions.WithApproval(ctx)
	}
	stream, err := sender.Send(ctx, sessions.SendRequest{SessionID: target.ID, Text: payload.Text})
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "denied") || strings.Contains(err.Error(), "approval") {
			status = http.StatusForbidden
		}
		http.Error(w, safeSendError(err), status)
		return true
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unavailable", http.StatusInternalServerError)
		return true
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	for {
		select {
		case <-r.Context().Done():
			return true
		case event, open := <-stream:
			if !open {
				return true
			}
			data, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				return true
			}
			if _, writeErr := fmt.Fprintf(w, "data: %s\n\n", data); writeErr != nil {
				return true
			}
			flusher.Flush()
			if event.Type == "done" || event.Type == "error" {
				return true
			}
		}
	}
}

func safeSendError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "sending the message timed out"
	}
	return err.Error()
}

func sendAgentID(agent string) string {
	if strings.EqualFold(agent, "Claude Code") {
		return "claude"
	}
	if strings.EqualFold(agent, "Codex") {
		return "codex"
	}
	return "unsupported"
}
