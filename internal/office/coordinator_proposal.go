package office

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// CoordinatorProposalSchema constrains model output to a public conclusion.
// Identity, authority, request IDs and expiry are supplied by the trusted
// coordinator process, not by model output. A proposal itself authorizes nothing.
func CoordinatorProposalSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"properties":{"outcome":{"type":"string","enum":["AUTHORIZED","DEFERRED","DENIED"]},"reason":{"type":"string","minLength":1,"maxLength":16384},"agentId":{"type":"string"},"resources":{"type":"array","items":{"type":"string"},"maxItems":100},"capabilities":{"type":"array","items":{"type":"string"},"maxItems":100}},"required":["outcome","reason","agentId","resources","capabilities"]}`)
}

type coordinatorProposal struct {
	Outcome      *string   `json:"outcome"`
	Reason       *string   `json:"reason"`
	AgentID      *string   `json:"agentId"`
	Resources    *[]string `json:"resources"`
	Capabilities *[]string `json:"capabilities"`
}

// PrepareCoordinatorDecision decodes a bounded structured response. The caller
// must authenticate and use IngestCoordinatorDecision before dispatch planning.
// In particular, valid JSON or a model saying AUTHORIZED is not a dispatch grant.
func PrepareCoordinatorDecision(raw json.RawMessage, session CoordinatorSession, message Message, intent Intent, key string, expiry time.Time) (CoordinatorDecisionRequest, error) {
	var out CoordinatorDecisionRequest
	if len(raw) == 0 || len(raw) > 64*1024 || strings.TrimSpace(key) == "" || len(key) > 200 || session.ID == "" || session.CoordinatorID == "" || session.CoordinatorRevision < 1 || message.ID == "" || message.IntentID != intent.ID || intent.ID == "" || intent.Revision < 1 {
		return out, ErrValidation
	}
	if !expiry.After(time.Now()) || expiry.After(session.ExpiresAt) {
		return out, fmt.Errorf("%w: proposal expiry exceeds coordinator session", ErrValidation)
	}
	var proposal coordinatorProposal
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&proposal) != nil || decoder.Decode(new(any)) != io.EOF || proposal.Outcome == nil || proposal.Reason == nil || proposal.AgentID == nil || proposal.Resources == nil || proposal.Capabilities == nil {
		return out, fmt.Errorf("%w: incomplete structured coordinator proposal", ErrValidation)
	}
	reason := strings.TrimSpace(*proposal.Reason)
	if reason == "" || len(reason) > 16384 || !boundedUnique(*proposal.Resources) || !boundedUnique(*proposal.Capabilities) || !subset(*proposal.Resources, session.Resources) || !subset(*proposal.Capabilities, session.Capabilities) {
		return out, fmt.Errorf("%w: proposal exceeds coordinator session", ErrValidation)
	}
	switch *proposal.Outcome {
	case "AUTHORIZED":
		if *proposal.AgentID == "" || len(*proposal.Resources) == 0 {
			return out, ErrValidation
		}
	case "DEFERRED", "DENIED":
		// A refusal/deferment cannot carry an executable resource grant.
		if len(*proposal.Resources) != 0 || len(*proposal.Capabilities) != 0 {
			return out, ErrValidation
		}
	default:
		return out, ErrValidation
	}
	for _, resource := range *proposal.Resources {
		if !validCoordinatorResource(resource) {
			return out, ErrValidation
		}
	}
	return CoordinatorDecisionRequest{
		MessageID: message.ID, IntentID: intent.ID, IntentRevision: intent.Revision, IdempotencyKey: key,
		Decision: Decision{
			CoordinatorID: session.CoordinatorID, CoordinatorRevision: session.CoordinatorRevision,
			Outcome: *proposal.Outcome, Reason: reason, AgentID: *proposal.AgentID,
			Resources: append([]string{}, (*proposal.Resources)...), Capabilities: append([]string{}, (*proposal.Capabilities)...),
			ExpiresAt: expiry.UTC().Format(time.RFC3339Nano),
		},
	}, nil
}
