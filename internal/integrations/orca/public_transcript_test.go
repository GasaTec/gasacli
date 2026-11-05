package orca

import (
	"encoding/json"
	"strings"
	"testing"
)

func transcriptTestReceipt(t *testing.T, messages ...map[string]any) Receipt {
	t.Helper()
	data, err := json.Marshal(map[string]any{"dispatchId": "dispatch-one", "source": "transcript", "sourceIdentity": "source-one", "provider": "codex", "cursor": "opaque-next", "fallbackReason": nil, "sourceExact": true, "contentComplete": true, "transcript": map[string]any{"messages": messages, "nextCursor": "opaque-next", "limited": false, "returnedMessageCount": len(messages)}})
	if err != nil {
		t.Fatal(err)
	}
	return Receipt{OK: true, RuntimeID: "runtime-one", Result: data}
}
func transcriptTestMessage(id, role, text string) map[string]any {
	return map[string]any{"id": id, "role": role, "source": "transcript", "timestamp": 1000, "blocks": []map[string]any{{"type": "text", "text": text}}}
}

func TestPublicTranscriptSeparatesDialogueFromPrivateAndToolRows(t *testing.T) {
	user := transcriptTestMessage("u", "user", "Ajuste o botão.")
	assistant := transcriptTestMessage("a", "assistant", "O botão foi ajustado.")
	assistant["blocks"] = []map[string]any{{"type": "tool-call", "name": "shell", "input": "PRIVATE TOOL INPUT"}, {"type": "text", "text": "O botão foi ajustado."}, {"type": "tool-result", "output": "PRIVATE TOOL OUTPUT"}, {"type": "thinking", "text": "PRIVATE UNKNOWN BLOCK"}, {"type": "text", "text": "PRIVATE FRAME", "providerFrame": map[string]any{"provider": "codex"}}}
	receipt := transcriptTestReceipt(t, user, assistant, transcriptTestMessage("r", "reasoning", "PRIVATE REASONING"), transcriptTestMessage("s", "system", "PRIVATE SYSTEM"), transcriptTestMessage("t", "tool", "PRIVATE TOOL"))
	receipt.Process.Stdout = "PRIVATE STDOUT"
	receipt.Process.Stderr = "PRIVATE STDERR"
	page, err := ProjectWorkerTranscript(receipt, "runtime-one", "dispatch-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.Messages[1].Text != "O botão foi ajustado." || page.Suppressed != 3 {
		t.Fatalf("incorrect public projection: %#v", page)
	}
	serialized, _ := json.Marshal(page)
	if strings.Contains(string(serialized), "PRIVATE") {
		t.Fatal("private/tool evidence crossed projection")
	}
}

func TestPublicTranscriptSuppressesCredentialShapedContent(t *testing.T) {
	for _, secret := range []string{"API_KEY=example-private-value", "password: example-private-value", "Authorization: Bearer private-value", "https://user:private-value@example.invalid", "client_secret is private-value", "pa\u200bssword: private-value"} {
		t.Run(secret[:4], func(t *testing.T) {
			receipt := transcriptTestReceipt(t, transcriptTestMessage("a", "assistant", secret))
			page, err := ProjectWorkerTranscript(receipt, "runtime-one", "dispatch-one")
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Messages) != 0 || page.Suppressed != 1 {
				t.Fatal("credential-shaped row was exposed")
			}
		})
	}
	row := transcriptTestMessage("a", "assistant", "")
	row["blocks"] = []map[string]any{{"type": "text", "text": "password"}, {"type": "text", "text": ": private-value"}}
	page, err := ProjectWorkerTranscript(transcriptTestReceipt(t, row), "runtime-one", "dispatch-one")
	if err != nil || len(page.Messages) != 0 {
		t.Fatal("credential split across blocks exposed")
	}
}

func TestPublicTranscriptRejectsFallbackIdentityAndUnknownVisibility(t *testing.T) {
	for _, change := range []func(*Receipt){func(r *Receipt) { r.Unknown = true }, func(r *Receipt) { r.RuntimeID = "other-runtime" }, func(r *Receipt) { r.Process.Truncated = true }, func(r *Receipt) {
		r.Result = json.RawMessage(`{"source":"terminal","terminal":{"tail":["PRIVATE RAW OUTPUT"]}}`)
	}} {
		receipt := transcriptTestReceipt(t, transcriptTestMessage("a", "assistant", "Public answer"))
		change(&receipt)
		page, err := ProjectWorkerTranscript(receipt, "runtime-one", "dispatch-one")
		if err == nil || len(page.Messages) != 0 {
			t.Fatal("unproven transcript was projected")
		}
	}
	row := transcriptTestMessage("a", "assistant", "PRIVATE HIDDEN")
	row["hidden"] = true
	page, err := ProjectWorkerTranscript(transcriptTestReceipt(t, row), "runtime-one", "dispatch-one")
	if err != nil || len(page.Messages) != 0 {
		t.Fatal("unknown visibility semantics guessed")
	}
}

func TestPublicTranscriptClipsUnicodeWithExplicitEvidence(t *testing.T) {
	row := transcriptTestMessage("a", "assistant", strings.Repeat("á", maxPublicMessageBytes))
	page, err := ProjectWorkerTranscript(transcriptTestReceipt(t, row), "runtime-one", "dispatch-one")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || len(page.Messages[0].Text) > maxPublicMessageBytes || !page.Messages[0].Truncated || !page.Clipped || page.ContentComplete {
		t.Fatal("clipping was not bounded and explicit")
	}
}
