package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

const planningSchema = `{"type":"object","additionalProperties":false,"required":["decision"],"properties":{"decision":{"type":"string","enum":["deferred","denied","allowed"]}}}`

func TestOutputSchemaRejectsExternalResourcesAndInvalidRoots(t *testing.T) {
	for _, input := range []string{
		`null`, `[]`, `{}`, `{"type":"array"}`, `{"type":"object"} trailing`,
		`{"type":"object","$ref":"https://example.invalid/schema"}`,
		`{"type":"object","properties":{"x":{"$ref":"file:///private/schema"}}}`,
		`{"type":"object","$dynamicRef":"other.json#anchor"}`,
		`{"type":"object","$id":"https://example.invalid/base/"}`,
		`{"type":"object","$schema":"https://example.invalid/meta-schema"}`,
		`{"type":"object","$ref":"https://example.invalid/schema","$ref":"#"}`,
		strings.Repeat(" ", maxOutputSchemaBytes) + `{"type":"object"}`,
	} {
		if _, err := ValidateOutputSchema(json.RawMessage(input)); err == nil {
			t.Fatalf("accepted invalid/external schema: %.200s", input)
		}
	}
	local := `{"type":"object","properties":{"item":{"$ref":"#/$defs/item"}},"$defs":{"item":{"type":"string"}}}`
	if _, err := ValidateOutputSchema(json.RawMessage(local)); err != nil {
		t.Fatalf("rejected self-contained schema: %v", err)
	}
	deep := `{"type":"object","value":` + strings.Repeat("[", 40) + `0` + strings.Repeat("]", 40) + `}`
	if _, err := ValidateOutputSchema(json.RawMessage(deep)); err == nil {
		t.Fatal("accepted excessive nesting")
	}
}

func TestPlanningSchemaUsesProviderSpecificArgumentsAndReadOnlyMode(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		request := Request{Agent: agent, ReadOnly: true, Profile: "autonomous", Prompt: "Plan only", Model: "configured-model", OutputSchema: json.RawMessage(planningSchema)}
		_, args, err := commandFor(request)
		if err != nil {
			t.Fatal(err)
		}
		if agent == "claude" {
			if !has(args, "--json-schema") || !has(args, planningSchema) || !has(args, "stream-json") || !has(args, "plan") {
				t.Fatalf("Claude planner args: %#v", args)
			}
		} else {
			args, path, err := addOutputSchema(args, request.OutputSchema)
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(path)
			if !has(args, "--output-schema") || !has(args, "read-only") || has(args, "danger-full-access") || args[len(args)-1] != request.Prompt {
				t.Fatalf("Codex planner args: %#v", args)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(contents) != planningSchema {
				t.Fatalf("explicit schema replaced by reviewer schema: %s", contents)
			}
		}
		request.ReadOnly = false
		if _, _, err := commandFor(request); err == nil {
			t.Fatal("structured planner accepted writable request")
		}
	}
}

func TestSchemaFileIsPrivateAndInvalidInputLeavesNoTemporaryFile(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	if _, _, err := addOutputSchema([]string{"exec", "prompt"}, json.RawMessage(`{"type":"object","$ref":"other.json"}`)); err == nil {
		t.Fatal("invalid schema produced a command")
	}
	files, err := filepath.Glob(filepath.Join(tempDir, "gasacli-output-schema-*.json"))
	if err != nil || len(files) != 0 {
		t.Fatalf("invalid schema leaked tempfile: %v %v", files, err)
	}
	path, err := writeOutputSchema(json.RawMessage(planningSchema))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("schema file permissions: %v", info.Mode().Perm())
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestStructuredPayloadUsesOnlyDocumentedSuccessfulProviderFields(t *testing.T) {
	for _, tc := range []struct {
		agent, line     string
		observed, valid bool
	}{
		{"claude", `{"type":"result","subtype":"success","is_error":false,"structured_output":{"decision":"deferred"}}`, true, true},
		{"claude", `{"type":"result","subtype":"error_max_turns","structured_output":{"decision":"allowed"}}`, true, false},
		{"claude", `{"type":"result","subtype":"success","is_error":true,"structured_output":{"decision":"allowed"}}`, true, false},
		{"claude", `{"type":"result","subtype":"success","result":"{\"decision\":\"allowed\"}"}`, true, false},
		{"claude", `{"type":"tool.started","structured_output":{"decision":"allowed"}}`, false, false},
		{"codex", `{"type":"item.completed","item":{"type":"agent_message","text":"{\"decision\":\"deferred\"}"}}`, true, true},
		{"codex", `{"type":"item.completed","item":{"type":"agent_message","text":"not JSON"}}`, true, false},
		{"codex", `{"type":"item.completed","item":{"type":"agent_message","text":"[]"}}`, true, false},
		{"codex", `{"type":"item.completed","item":{"type":"command_execution","text":"{\"decision\":\"allowed\"}"}}`, false, false},
	} {
		payload, observed := structuredPayload(tc.agent, []byte(tc.line))
		if observed != tc.observed || (len(payload) > 0) != tc.valid {
			t.Fatalf("%s payload=%s observed=%v for %s", tc.agent, payload, observed, tc.line)
		}
	}
}

func TestExplicitSchemaTempfileIsRemovedWhenExecutableIsMissing(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	home, project := t.TempDir(), t.TempDir()
	policy := permissions.New(home, trust.NewStore(home))
	if err := policy.SetProfile("balanced"); err != nil {
		t.Fatal(err)
	}
	sawSchema := false
	executor := Executor{Policy: policy, LookPath: func(string) (string, error) {
		files, err := filepath.Glob(filepath.Join(tempDir, "gasacli-output-schema-*.json"))
		if err != nil {
			t.Fatal(err)
		}
		sawSchema = len(files) == 1
		return "", errors.New("fixture executable absent")
	}}
	_, err := executor.Execute(context.Background(), Request{Agent: "codex", Project: project, Prompt: "Plan only", ReadOnly: true, OutputSchema: json.RawMessage(planningSchema)})
	if err == nil || !sawSchema {
		t.Fatalf("schema setup was not exercised: saw=%v err=%v", sawSchema, err)
	}
	files, err := filepath.Glob(filepath.Join(tempDir, "gasacli-output-schema-*.json"))
	if err != nil || len(files) != 0 {
		t.Fatalf("schema leaked after executable lookup failure: %v %v", files, err)
	}
}
