package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

const maxOutputSchemaBytes = 64 << 10
const maxStructuredOutputBytes = 64 << 10

// ValidateOutputSchema applies bounded structural checks without resolving any
// external resource. The provider remains responsible for its supported JSON
// Schema dialect; callers must validate domain decisions before authorizing work.
func ValidateOutputSchema(schema json.RawMessage) (json.RawMessage, error) {
	if len(schema) == 0 || len(schema) > maxOutputSchemaBytes {
		return nil, errors.New("adapter: output schema must contain 1 to 65536 bytes")
	}
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil || root == nil || root["type"] != "object" {
		return nil, errors.New("adapter: output schema must be a JSON object with type object")
	}
	// Preserve one interpretation across validators. JSON permits parsers to
	// disagree about duplicate names, including a hidden external $ref.
	decoder := json.NewDecoder(bytes.NewReader(schema))
	var uniqueKeys func(int) error
	uniqueKeys = func(depth int) error {
		if depth > 32 {
			return errors.New("adapter: output schema exceeds structural limits")
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("adapter: output schema contains duplicate keys")
				}
				seen[name] = true
				if err := uniqueKeys(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case json.Delim('['):
			for decoder.More() {
				if err := uniqueKeys(depth + 1); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		}
		return nil
	}
	if err := uniqueKeys(0); err != nil {
		return nil, err
	}
	nodes := 0
	var walk func(any, int) error
	walk = func(value any, depth int) error {
		nodes++
		if depth > 32 || nodes > 4096 {
			return errors.New("adapter: output schema exceeds structural limits")
		}
		switch value := value.(type) {
		case map[string]any:
			for key, child := range value {
				if key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef" {
					ref, ok := child.(string)
					if !ok || !strings.HasPrefix(ref, "#") {
						return errors.New("adapter: output schema cannot reference external resources")
					}
				}
				// External base identities and meta-schema declarations can cause
				// some validators to resolve remote resources. Do not pass them on.
				if key == "$schema" || key == "$id" {
					ref, ok := child.(string)
					if !ok || (ref != "" && !strings.HasPrefix(ref, "#")) {
						return errors.New("adapter: output schema cannot declare an external schema identity")
					}
				}
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range value {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, schema); err != nil {
		return nil, err
	}
	return json.RawMessage(compact.Bytes()), nil
}

func writeOutputSchema(schema json.RawMessage) (string, error) {
	validated, err := ValidateOutputSchema(schema)
	if err != nil {
		return "", err
	}
	file, err := os.CreateTemp("", "gasacli-output-schema-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	remove := func() { _ = file.Close(); _ = os.Remove(path) }
	if err := file.Chmod(0600); err != nil {
		remove()
		return "", err
	}
	if _, err := file.Write(validated); err != nil {
		remove()
		return "", err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func addOutputSchema(args []string, schema json.RawMessage) ([]string, string, error) {
	if len(args) == 0 {
		return nil, "", errors.New("adapter: schema command is missing its prompt")
	}
	path, err := writeOutputSchema(schema)
	if err != nil {
		return nil, "", err
	}
	result := make([]string, 0, len(args)+2)
	result = append(result, args[:len(args)-1]...)
	result = append(result, "--output-schema", path, args[len(args)-1])
	return result, path, nil
}

// structuredPayload reads only documented public output fields. It does not
// scrape tool events, hidden reasoning, fenced code, or arbitrary JSON strings.
// observed lets the caller invalidate an older candidate when a later final
// message is missing or invalid, instead of authorizing stale model output.
func structuredPayload(agent string, line []byte) (json.RawMessage, bool) {
	var event struct {
		Type       string          `json:"type"`
		Subtype    string          `json:"subtype"`
		IsError    *bool           `json:"is_error"`
		Structured json.RawMessage `json:"structured_output"`
		Item       struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}
	if json.Unmarshal(line, &event) != nil {
		return nil, false
	}
	var candidate json.RawMessage
	switch strings.ToLower(agent) {
	case "claude":
		if event.Type != "result" {
			return nil, false
		}
		if event.Subtype != "success" || event.IsError == nil || *event.IsError {
			return nil, true
		}
		candidate = event.Structured
	case "codex":
		if event.Type != "item.completed" || event.Item.Type != "agent_message" {
			return nil, false
		}
		candidate = json.RawMessage(event.Item.Text)
	default:
		return nil, false
	}
	if len(candidate) == 0 || len(candidate) > maxStructuredOutputBytes {
		return nil, true
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(candidate, &object); err != nil || object == nil {
		return nil, true
	}
	// Copy the event-owned bytes; the scanner reuses its line buffer.
	return append(json.RawMessage(nil), candidate...), true
}

func explicitSchema(request Request) (json.RawMessage, error) {
	if len(request.OutputSchema) == 0 {
		return nil, nil
	}
	if !request.ReadOnly {
		return nil, fmt.Errorf("adapter: structured planning requires a read-only request")
	}
	return ValidateOutputSchema(request.OutputSchema)
}
