package adapters

import "testing"

func TestStructuredText(t *testing.T) {
	for _, tc := range []struct{ event, line, want string }{
		{event: "result", line: `{"type":"result","result":"reply"}`, want: "reply"},
		{event: "content_block_delta", line: `{"type":"content_block_delta","delta":{"text":"part"}}`, want: "part"},
		{event: "item.completed", line: `{"type":"item.completed","item":{"type":"agent_message","text":"done"}}`, want: "done"},
		{event: "tool.started", line: `{"type":"tool.started","data":"hidden"}`},
	} {
		t.Run(tc.event, func(t *testing.T) {
			if got := StructuredText([]byte(tc.line), tc.event); got != tc.want {
				t.Fatalf("StructuredText() = %q, want %q", got, tc.want)
			}
		})
	}
}
