package orca

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GasaTec/gasacli/internal/sessions"
)

// Public transcript projection is deliberately narrower than Orca's CLI text
// formatter, which includes reasoning, tool inputs/results and raw provider frames.
// Wire evidence: Orca a19de980854e1c875a4555e1bd24d832c9f96657,
// src/shared/orchestration-worker-output.ts;
// src/shared/native-chat/native-chat-types.ts;
// src/main/runtime/rpc/methods/orchestration/worker/worker-output.ts;
// src/main/runtime/rpc/methods/orchestration-structured-worker-lifecycle.ts.
const (
	maxTranscriptReceiptBytes = 256 * 1024
	maxTranscriptMessages     = 1000
	maxPublicTranscriptBytes  = 64 * 1024
	maxPublicMessageBytes     = 16 * 1024
)

type PublicTranscriptMessage struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Timestamp *int64 `json:"timestamp"`
	Truncated bool   `json:"truncated"`
}
type PublicTranscriptPage struct {
	DispatchID      string                    `json:"dispatchId"`
	SourceIdentity  string                    `json:"sourceIdentity"`
	Provider        string                    `json:"provider"`
	Messages        []PublicTranscriptMessage `json:"messages"`
	NextCursor      string                    `json:"nextCursor"`
	Limited         bool                      `json:"limited"`
	ContentComplete bool                      `json:"contentComplete"`
	Suppressed      int                       `json:"suppressed"`
	Clipped         bool                      `json:"clipped"`
}

type publicTranscriptWirePage struct {
	DispatchID     string `json:"dispatchId"`
	Source         string `json:"source"`
	SourceIdentity string `json:"sourceIdentity"`
	Provider       string `json:"provider"`
	Transcript     *struct {
		Messages             []json.RawMessage `json:"messages"`
		NextCursor           string            `json:"nextCursor"`
		Limited              *bool             `json:"limited"`
		ReturnedMessageCount *int              `json:"returnedMessageCount"`
	} `json:"transcript"`
	Cursor          string          `json:"cursor"`
	FallbackReason  json.RawMessage `json:"fallbackReason"`
	SourceExact     *bool           `json:"sourceExact"`
	ContentComplete *bool           `json:"contentComplete"`
}

// ProjectWorkerTranscript accepts only a successful structured transcript read
// for the exact persisted runtime and Dispatch. It never reads Process stdout,
// terminal tails, warnings, tool results, provider frames or hidden reasoning.
// Cursor/source identity are opaque provenance, not message content. Filtering
// does not prove that arbitrary prose contains no secrets: credential-shaped
// content is suppressed conservatively and callers must keep raw receipts private.
func ProjectWorkerTranscript(receipt Receipt, expectedRuntimeID, expectedDispatchID string) (PublicTranscriptPage, error) {
	fail := func() (PublicTranscriptPage, error) {
		return PublicTranscriptPage{}, errors.New("public worker transcript unavailable or unverified")
	}
	if !receipt.OK || receipt.Unknown || receipt.Process.Truncated || expectedRuntimeID == "" || expectedDispatchID == "" || receipt.RuntimeID != expectedRuntimeID || len(receipt.Result) == 0 || len(receipt.Result) > maxTranscriptReceiptBytes || !utf8.Valid(receipt.Result) {
		return fail()
	}
	var wire publicTranscriptWirePage
	if json.Unmarshal(receipt.Result, &wire) != nil || wire.Source != "transcript" || wire.DispatchID != expectedDispatchID || !opaqueTranscriptValue(wire.DispatchID) || !opaqueTranscriptValue(wire.SourceIdentity) || !opaqueTranscriptValue(wire.Provider) || wire.Transcript == nil || wire.Transcript.Messages == nil || wire.Transcript.Limited == nil || wire.Transcript.ReturnedMessageCount == nil || len(wire.Transcript.Messages) > maxTranscriptMessages || *wire.Transcript.ReturnedMessageCount != len(wire.Transcript.Messages) || !opaqueTranscriptValue(wire.Cursor) || wire.Cursor != wire.Transcript.NextCursor || string(wire.FallbackReason) != "null" || (wire.SourceExact != nil && !*wire.SourceExact) {
		return fail()
	}
	page := PublicTranscriptPage{DispatchID: wire.DispatchID, SourceIdentity: wire.SourceIdentity, Provider: wire.Provider, Messages: []PublicTranscriptMessage{}, NextCursor: wire.Cursor, Limited: *wire.Transcript.Limited}
	// SourceExact is optional for the structured worker branch in the pinned
	// contract. It remains exact by the Dispatch/sourceIdentity read; absence is
	// never promoted to host-declared content completeness.
	if wire.ContentComplete != nil {
		page.ContentComplete = *wire.ContentComplete && !page.Limited
	}
	seen := map[string]bool{}
	usedBytes := 0
	for _, raw := range wire.Transcript.Messages {
		message, emit, err := projectPublicMessage(raw)
		if err != nil {
			return fail()
		}
		if !emit {
			page.Suppressed++
			continue
		}
		if seen[message.ID] {
			return fail()
		}
		seen[message.ID] = true
		if message.Truncated {
			page.Clipped = true
		}
		if len(message.Text) > maxPublicMessageBytes {
			message.Text = truncateTranscriptUTF8(message.Text, maxPublicMessageBytes)
			message.Truncated = true
			page.Clipped = true
		}
		if usedBytes+len(message.Text) > maxPublicTranscriptBytes {
			return fail()
		}
		usedBytes += len(message.Text)
		page.Messages = append(page.Messages, message)
	}
	if page.Clipped {
		page.ContentComplete = false
	}
	return page, nil
}

func opaqueTranscriptValue(value string) bool {
	if value == "" || len(value) > 4096 || !utf8.ValidString(value) || sensitivePublicText(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

// Unknown row fields may carry visibility semantics in a future host. Suppress
// that row instead of guessing; known operational annotations are not dialogue.
var publicMessageFields = map[string]bool{
	"id": true, "role": true, "blocks": true, "timestamp": true, "source": true,
	"transcriptOffset": true, "transcriptOrder": true, "model": true, "provider": true, "usage": true,
	"turnId": true, "parentId": true, "sentAs": true, "shortened": true, "state": true, "completedAt": true,
	"command": true, "from": true, "agentStartDispatchId": true, "agentStart": true, "agentHandoff": true,
	"queued": true, "pendingSend": true, "deliveryState": true, "queuedMessageId": true, "unsent": true,
	"sentWhileStopping": true, "awaitsRetry": true, "stoppedBeforeStart": true, "journalPosition": true,
	"foldedJournalPosition": true, "unpairedToolResults": true, "agentId": true, "parentAgentId": true,
	"providerParentRef": true, "producerKind": true, "attempt": true,
}

func projectPublicMessage(raw json.RawMessage) (PublicTranscriptMessage, bool, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return PublicTranscriptMessage{}, false, errors.New("invalid transcript message")
	}
	for key := range fields {
		if !publicMessageFields[key] {
			return PublicTranscriptMessage{}, false, nil
		}
	}
	var row struct {
		ID        string            `json:"id"`
		Role      string            `json:"role"`
		Source    string            `json:"source"`
		Timestamp json.RawMessage   `json:"timestamp"`
		Blocks    []json.RawMessage `json:"blocks"`
		Shortened bool              `json:"shortened"`
	}
	if json.Unmarshal(raw, &row) != nil || !opaqueTranscriptValue(row.ID) || row.Blocks == nil {
		return PublicTranscriptMessage{}, false, errors.New("invalid transcript message structure")
	}
	if !contains([]string{"user", "assistant", "tool", "reasoning", "system"}, row.Role) {
		return PublicTranscriptMessage{}, false, errors.New("unknown transcript role")
	}
	if row.Role != "user" && row.Role != "assistant" {
		return PublicTranscriptMessage{}, false, nil
	}
	if row.Source != "transcript" {
		return PublicTranscriptMessage{}, false, nil
	}
	for _, key := range []string{"agentStartDispatchId", "agentStart", "agentHandoff", "queued", "pendingSend", "deliveryState", "queuedMessageId", "unsent", "sentWhileStopping", "awaitsRetry", "stoppedBeforeStart", "unpairedToolResults", "command", "from", "agentId", "parentAgentId", "providerParentRef", "producerKind"} {
		if value, ok := fields[key]; ok && string(value) != "null" && string(value) != "false" {
			return PublicTranscriptMessage{}, false, nil
		}
	}
	message := PublicTranscriptMessage{ID: row.ID, Role: row.Role, Truncated: row.Shortened}
	if len(row.Timestamp) == 0 {
		return PublicTranscriptMessage{}, false, errors.New("timestamp absent from transcript row")
	}
	if string(row.Timestamp) != "null" {
		var timestamp int64
		if json.Unmarshal(row.Timestamp, &timestamp) != nil || timestamp < 0 {
			return PublicTranscriptMessage{}, false, errors.New("invalid transcript timestamp")
		}
		message.Timestamp = &timestamp
	}
	texts := []string{}
	for _, rawBlock := range row.Blocks {
		var block map[string]json.RawMessage
		if json.Unmarshal(rawBlock, &block) != nil || block == nil {
			return PublicTranscriptMessage{}, false, errors.New("invalid transcript block")
		}
		var kind string
		if json.Unmarshal(block["type"], &kind) != nil {
			return PublicTranscriptMessage{}, false, errors.New("invalid transcript block discriminator")
		}
		if kind != "text" {
			continue
		}
		// presentation/providerFrame/failure/contextClear and future metadata may
		// denote synthetic summaries or raw provider frames. Only plain text fits.
		if len(block) != 2 || block["text"] == nil {
			continue
		}
		var text string
		if json.Unmarshal(block["text"], &text) != nil {
			return PublicTranscriptMessage{}, false, errors.New("invalid transcript text")
		}
		if !utf8.ValidString(text) || sensitivePublicText(text) {
			return PublicTranscriptMessage{}, false, nil
		}
		texts = append(texts, stripTranscriptControls(text))
	}
	message.Text = strings.TrimSpace(strings.Join(texts, "\n"))
	if sensitivePublicText(message.Text) {
		return PublicTranscriptMessage{}, false, nil
	}
	if message.Text == "" {
		return PublicTranscriptMessage{}, false, nil
	}
	return message, true, nil
}

var publicCredentialAssignment = regexp.MustCompile(`(?i)(?:["']?\b(?:password|passwd|pwd|secret|token|access[_ -]?token|refresh[_ -]?token|api[_ -]?key|private[_ -]?key|client[_ -]?secret|authorization|cookie|set-cookie|credential)[A-Z0-9_ -]*["']?\s*(?::|=|\bis\b)\s*\S|https?://[^\s/@:]+:[^\s/@]+@|\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,})`)

func sensitivePublicText(text string) bool {
	return sessions.Redact(text) != text || publicCredentialAssignment.MatchString(text)
}
func stripTranscriptControls(text string) string {
	return strings.Map(func(r rune) rune {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
}
func truncateTranscriptUTF8(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit]
}
