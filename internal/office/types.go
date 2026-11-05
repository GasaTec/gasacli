// Package office persists project-local office identities and coordination requests.
// Saving office data never launches a provider or grants execution permission.
package office

import (
	"encoding/json"
	"errors"
)

var (
	ErrValidation = errors.New("office: invalid input")
	ErrConflict   = errors.New("office: revision or idempotency conflict")
	ErrNotFound   = errors.New("office: not found")
	ErrBusy       = errors.New("office: active work requires reconciliation")
)

type Avatar struct {
	FamilyID    string   `json:"familyId"`
	Color       string   `json:"color"`
	Eyes        string   `json:"eyes"`
	Accessories []string `json:"accessories"`
}

type ExecutionChoice struct {
	ToolID string `json:"toolId"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

type AgentInput struct {
	Status              string            `json:"status"`
	Name                string            `json:"name"`
	Role                string            `json:"role"`
	WhenToCall          string            `json:"whenToCall"`
	Instructions        string            `json:"instructions"`
	Avatar              Avatar            `json:"avatar"`
	Primary             ExecutionChoice   `json:"primary"`
	Fallbacks           []ExecutionChoice `json:"fallbacks"`
	FallbackMode        string            `json:"fallbackMode"`
	AllowedCapabilities []string          `json:"allowedCapabilities"`
	TeamIDs             []string          `json:"teamIds"`
}

type Agent struct {
	AgentInput
	ID       string `json:"id"`
	Revision int    `json:"revision"`
	DeskID   string `json:"deskId"`
}

type Team struct {
	ID        string   `json:"id"`
	Revision  int      `json:"revision"`
	Name      string   `json:"name"`
	Purpose   string   `json:"purpose"`
	MemberIDs []string `json:"memberIds"`
	LeaderID  string   `json:"leaderId"`
	Archived  bool     `json:"archived"`
}

type Settings struct {
	Revision      int    `json:"revision"`
	Autonomy      string `json:"autonomy"`
	CoordinatorID string `json:"coordinatorId"`
	Paused        bool   `json:"paused"`
	ReducedMotion bool   `json:"reducedMotion"`
	Sound         bool   `json:"sound"`
	Quality       string `json:"quality"`
}

type MessageInput struct {
	RecipientKind string `json:"recipientKind"` // team, coordinator, agent
	RecipientID   string `json:"recipientId"`
	Text          string `json:"text"`
}

type Message struct {
	MessageInput
	ID         string `json:"id"`
	IntentID   string `json:"intentId"`
	SenderKind string `json:"senderKind"`
	Status     string `json:"status"`
	CreatedAt  string `json:"createdAt"`
}

type Intent struct {
	ID        string    `json:"id"`
	Revision  int       `json:"revision"`
	MessageID string    `json:"messageId"`
	Status    string    `json:"status"`
	CreatedAt string    `json:"createdAt"`
	Decision  *Decision `json:"decision,omitempty"`
}

// Decision is recorded only by a trusted coordinator bridge. HTTP clients must
// never receive an endpoint that lets them impersonate this authority.
type Decision struct {
	CoordinatorID       string   `json:"coordinatorId"`
	CoordinatorRevision int      `json:"coordinatorRevision"`
	AgentRevision       int      `json:"agentRevision"`
	Outcome             string   `json:"outcome"` // AUTHORIZED, DEFERRED, DENIED
	Reason              string   `json:"reason"`
	AgentID             string   `json:"agentId"`
	Resources           []string `json:"resources"`
	Capabilities        []string `json:"capabilities"`
	ExpiresAt           string   `json:"expiresAt"`
}

// Tasks are evidence-backed projections of the existing engine, not workers
// manufactured from submitted messages. This implementation does not dispatch.
type Task struct {
	ID      string `json:"id"`
	AgentID string `json:"agentId"`
	Status  string `json:"status"`
	RunID   string `json:"runId"`
	Title   string `json:"title"`
}

type Metrics struct {
	Requests    int      `json:"requests"`
	Tasks       int      `json:"tasks"`
	Verified    int      `json:"verified"`
	Blocked     int      `json:"blocked"`
	Cancelled   int      `json:"cancelled"`
	Closed      int      `json:"closed"`
	SuccessRate *float64 `json:"successRate"`
	Tokens      *int64   `json:"tokens"`
}

type Snapshot struct {
	ProjectID string    `json:"projectId"`
	Cursor    int64     `json:"cursor"`
	Agents    []Agent   `json:"agents"`
	Teams     []Team    `json:"teams"`
	Settings  Settings  `json:"settings"`
	Messages  []Message `json:"messages"`
	Intents   []Intent  `json:"intents"`
	Tasks     []Task    `json:"tasks"`
	Metrics   Metrics   `json:"metrics"`
}

type Event struct {
	Cursor    int64           `json:"cursor"`
	ProjectID string          `json:"projectId"`
	Type      string          `json:"type"`
	Origin    string          `json:"origin"`
	CreatedAt string          `json:"createdAt"`
	Payload   json.RawMessage `json:"payload"`
}
