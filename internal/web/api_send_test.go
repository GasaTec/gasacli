package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/sessions"
	"github.com/GasaTec/gasacli/internal/trust"
)

type fakeSessionSender struct{ requests []sessions.SendRequest }

func (s *fakeSessionSender) Capabilities() sessions.Capabilities {
	return sessions.Capabilities{Send: true, Agents: []string{"Claude Code"}}
}
func (s *fakeSessionSender) Send(_ context.Context, request sessions.SendRequest) (<-chan sessions.Event, error) {
	s.requests = append(s.requests, request)
	stream := make(chan sessions.Event, 2)
	stream <- sessions.Event{Type: "text", Text: "pong"}
	stream <- sessions.Event{Type: "done"}
	close(stream)
	return stream, nil
}

func TestSendAPIRequiresApprovalAndStreamsReply(t *testing.T) {
	home, projectPath := t.TempDir(), t.TempDir()
	if err := trust.NewStore(home).Grant(projectPath); err != nil {
		t.Fatal(err)
	}
	project := projectView{ID: projectID(projectPath), Name: "trusted", Path: projectPath, Trusted: true, Sessions: []sessions.Session{{ID: "claude-12345678", Agent: "Claude Code", Title: "Review", CWD: projectPath}}}
	getSnapshot := func() snapshot { return snapshot{projects: []projectView{project}} }
	sender := &fakeSessionSender{}
	call := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/sessions/claude-12345678/send", strings.NewReader(body))
		w := httptest.NewRecorder()
		sendAPI(home, sender, getSnapshot, w, r)
		return w
	}
	approval := call(`{"text":"say pong"}`)
	if approval.Code != http.StatusPreconditionRequired || !strings.Contains(approval.Body.String(), `"text":"say pong"`) || len(sender.requests) != 0 {
		t.Fatalf("approval response = %d %s, requests=%#v", approval.Code, approval.Body.String(), sender.requests)
	}
	reply := call(`{"text":"say pong","approved":true}`)
	if reply.Code != http.StatusOK || !strings.Contains(reply.Body.String(), `"text":"pong"`) || !strings.Contains(reply.Body.String(), `"type":"done"`) || len(sender.requests) != 1 {
		t.Fatalf("send response = %d %s, requests=%#v", reply.Code, reply.Body.String(), sender.requests)
	}
}

func TestSendAPIDeniesPolicyAndUntrustedProjects(t *testing.T) {
	home, projectPath := t.TempDir(), t.TempDir()
	store := trust.NewStore(home)
	if err := store.Grant(projectPath); err != nil {
		t.Fatal(err)
	}
	policy := permissions.New(home, store)
	if err := policy.Save(permissions.UserScope, "", "", permissions.Policy{Profile: "restricted"}); err != nil {
		t.Fatal(err)
	}
	sender := &fakeSessionSender{}
	project := projectView{ID: projectID(projectPath), Name: "trusted", Path: projectPath, Trusted: true, Sessions: []sessions.Session{{ID: "claude-12345678", Agent: "Claude Code", Title: "Review"}}}
	call := func(projects []projectView) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/sessions/claude-12345678/send", strings.NewReader(`{"text":"say pong","approved":true}`))
		sendAPI(home, sender, func() snapshot { return snapshot{projects: projects} }, w, r)
		return w
	}
	if response := call([]projectView{project}); response.Code != http.StatusForbidden {
		t.Fatalf("restricted policy status=%d body=%s", response.Code, response.Body.String())
	}
	if response := call([]projectView{{ID: "untrusted", Trusted: false}}); response.Code != http.StatusNotFound {
		t.Fatalf("untrusted project status=%d body=%s", response.Code, response.Body.String())
	}
	if len(sender.requests) != 0 {
		t.Fatalf("policy denials reached sender: %#v", sender.requests)
	}
}

func TestHandlerRoutesSessionSendPost(t *testing.T) {
	home, projectPath, transcriptRoot := t.TempDir(), t.TempDir(), t.TempDir()
	if err := trust.NewStore(home).Grant(projectPath); err != nil {
		t.Fatal(err)
	}
	providerID := "550e8400-e29b-41d4-a716-446655440000"
	row, _ := json.Marshal(map[string]any{"type": "user", "sessionId": providerID, "cwd": projectPath, "message": map[string]any{"role": "user", "content": "existing"}})
	if err := os.WriteFile(filepath.Join(transcriptRoot, "session.jsonl"), append(row, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	indexed := sessions.ScanMetadata(sessions.Roots{Claude: transcriptRoot})
	if len(indexed) != 1 {
		t.Fatalf("indexed sessions = %d", len(indexed))
	}
	server := httptest.NewUnstartedServer(nil)
	defer server.Close()
	port := server.Listener.Addr().(*net.TCPAddr).Port
	origin := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	sender := &fakeSessionSender{}
	launch := &launchSession{secret: "test-token"}
	server.Config.Handler = handler(origin, port, "test-token", launch, Options{Home: home, SessionRoots: sessions.Roots{Claude: transcriptRoot}, Sender: sender})
	server.Start()
	request, _ := http.NewRequest(http.MethodPost, origin+"/api/sessions/"+indexed[0].ID+"/send", strings.NewReader(`{"text":"say pong","approved":true}`))
	request.Header.Set("Origin", origin)
	request.Header.Set("X-Gasa-Token", "test-token")
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || len(sender.requests) != 1 {
		t.Fatalf("send route status=%d request=%#v", response.StatusCode, sender.requests)
	}
}
