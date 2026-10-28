package adapters

import (
	"encoding/json"
	"os/exec"
)

// StructuredText extracts user-visible text from a provider stream event.
func StructuredText(line []byte, eventType string) string {
	var event struct {
		Result string `json:"result"`
		Delta  struct {
			Text string `json:"text"`
		} `json:"delta"`
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}
	if json.Unmarshal(line, &event) != nil {
		return ""
	}
	if event.Result != "" {
		return event.Result
	}
	if eventType == "content_block_delta" && event.Delta.Text != "" {
		return event.Delta.Text
	}
	if eventType == "item.completed" && event.Item.Type == "agent_message" {
		return event.Item.Text
	}
	return ""
}

// ConfigureChildProcess and KillProcessTree share the executor's platform
// specific child-process lifecycle with session transports.
func ConfigureChildProcess(cmd *exec.Cmd) { configureProcessTree(cmd) }

func KillProcessTree(cmd *exec.Cmd) error { return terminateProcessTree(cmd) }
