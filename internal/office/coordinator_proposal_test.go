package office

import (
	"testing"
	"time"
)

func TestProposalCannotChooseAuthorityOrExpandScope(t *testing.T) {
	session := CoordinatorSession{ID: "session", CoordinatorID: "coordinator", CoordinatorRevision: 4, ExpiresAt: time.Now().Add(time.Hour), Resources: []string{"internal/login.go"}, Capabilities: []string{"read"}}
	message := Message{ID: "message", IntentID: "intent"}
	intent := Intent{ID: "intent", Revision: 2}
	expiry := time.Now().Add(time.Minute)
	valid := `{"outcome":"AUTHORIZED","reason":"Inspect login","agentId":"specialist","resources":["internal/login.go"],"capabilities":["read"]}`
	request, err := PrepareCoordinatorDecision([]byte(valid), session, message, intent, "key", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if request.Decision.CoordinatorID != session.CoordinatorID || request.IntentID != intent.ID || request.IntentRevision != 2 || request.Decision.CoordinatorRevision != 4 {
		t.Fatal("authority not bound to owned context")
	}
	for _, raw := range []string{
		`{"outcome":"AUTHORIZED","reason":"x","agentId":"a","resources":["../secret"],"capabilities":["read"]}`,
		`{"outcome":"AUTHORIZED","reason":"x","agentId":"a","resources":["internal/login.go"],"capabilities":["write"]}`,
		`{"outcome":"DENIED","reason":"x","agentId":"a","resources":["internal/login.go"],"capabilities":[]}`,
		`{"outcome":"DEFERRED","reason":"x","agentId":"","resources":null,"capabilities":[]}`,
		`{"outcome":"DEFERRED","reason":"x","agentId":"","resources":[],"capabilities":[],"coordinatorId":"forged"}`,
		valid + ` {"extra":true}`,
	} {
		if _, err := PrepareCoordinatorDecision([]byte(raw), session, message, intent, "key", expiry); err == nil {
			t.Fatalf("unsafe proposal accepted: %s", raw)
		}
	}
	if _, err := PrepareCoordinatorDecision([]byte(valid), session, message, intent, "key", session.ExpiresAt.Add(time.Minute)); err == nil {
		t.Fatal("expiry expanded")
	}
	message.IntentID = "other"
	if _, err := PrepareCoordinatorDecision([]byte(valid), session, message, intent, "key", expiry); err == nil {
		t.Fatal("request rebound to another intent")
	}
}
