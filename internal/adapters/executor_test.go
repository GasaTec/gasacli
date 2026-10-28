package adapters

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

func TestCommandForMapsSupportedNonInteractiveModes(t *testing.T) {
	claude, args, err := commandFor(Request{Agent: "claude", Prompt: "do work", Profile: "restricted"})
	if err != nil {
		t.Fatal(err)
	}
	if claude != "claude" || !has(args, "-p") || !has(args, "stream-json") || !has(args, "acceptEdits") {
		t.Fatalf("claude args = %#v", args)
	}
	codex, args, err := commandFor(Request{Agent: "codex", Prompt: "do work", Profile: "balanced"})
	if err != nil {
		t.Fatal(err)
	}
	if codex != "codex" || !has(args, "exec") || !has(args, "--json") || !has(args, "workspace-write") || !has(args, "--skip-git-repo-check") {
		t.Fatalf("codex args = %#v", args)
	}
	if _, _, err = commandFor(Request{Agent: "opencode"}); err == nil {
		t.Fatal("OpenCode without a documented stream mode must fail closed")
	}
	claude, args, err = commandFor(Request{Agent: "claude", Prompt: "review", ReadOnly: true})
	if err != nil || claude != "claude" || !has(args, "plan") || has(args, "acceptEdits") {
		t.Fatalf("read-only claude args=%#v err=%v", args, err)
	}
	codex, args, err = commandFor(Request{Agent: "codex", Prompt: "review", Project: ".", ReadOnly: true})
	if err != nil || codex != "codex" || !has(args, "read-only") {
		t.Fatalf("read-only codex args=%#v err=%v", args, err)
	}
	codex, args, err = commandFor(Request{Agent: "codex", Prompt: "do work", Project: ".", Profile: "autonomous"})
	if err != nil || codex != "codex" || !has(args, "danger-full-access") {
		t.Fatalf("autonomous codex args=%#v err=%v", args, err)
	}
}

func TestWriteReviewSchemaCreatesStructuredVerdictSchema(t *testing.T) {
	args, path, err := addReviewSchema([]string{"exec", "--json", "review prompt"})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	if !has(args, "--output-schema") || args[len(args)-1] != "review prompt" || args[len(args)-2] != path {
		t.Fatalf("review schema arguments = %#v", args)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err = json.Unmarshal(contents, &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("schema does not require a closed object: %#v", schema)
	}
}

func TestStructuredTextExtractsReviewResponseWithoutEventData(t *testing.T) {
	if got := structuredText([]byte(`{"type":"result","result":"{\"verdict\":\"approve\"}"}`), "result"); got != `{"verdict":"approve"}` {
		t.Fatalf("Claude result = %q", got)
	}
	if got := structuredText([]byte(`{"type":"item.completed","item":{"type":"agent_message","text":"{\"verdict\":\"approve\"}"}}`), "item.completed"); got != `{"verdict":"approve"}` {
		t.Fatalf("Codex result = %q", got)
	}
	if got := structuredText([]byte(`{"type":"tool.started","data":"ignore"}`), "tool.started"); got != "" {
		t.Fatalf("unexpected structured result %q", got)
	}
}

func TestCaptureMetricsReadsOnlyNumericUsageFields(t *testing.T) {
	var result Result
	captureMetrics([]byte(`{"type":"result","cost_usd":0.0125,"usage":{"input_tokens":42,"output_tokens":7},"result":"secret text"}`), &result)
	if !result.CostKnown || result.CostUSD != 0.0125 || result.InputTokens != 42 || result.OutputTokens != 7 {
		t.Fatalf("metrics = %#v", result)
	}
}

func TestAllowedEnvironmentExcludesCredentials(t *testing.T) {
	t.Setenv("PATH", "safe-path")
	t.Setenv("ANTHROPIC_API_KEY", "never-copy")
	got := strings.Join(allowedEnvironment(map[string]string{"PATH": "override", "ANTHROPIC_API_KEY": "also-never"}), "\n")
	if !strings.Contains(got, "PATH=override") {
		t.Fatalf("explicit allowed PATH missing: %q", got)
	}
	if strings.Contains(got, "never-copy") || strings.Contains(got, "ANTHROPIC_API_KEY") {
		t.Fatalf("secret leaked to child environment: %q", got)
	}
}

func TestExecuteFailsClosedForUntrustedProject(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	policy := permissions.New(home, trust.NewStore(home))
	started := false
	executor := Executor{Policy: policy, LookPath: func(string) (string, error) { return filepath.Join(project, "fake-cli"), nil }}
	_, err := executor.Execute(context.Background(), Request{Agent: "claude", Project: project, Prompt: "create file"})
	if err == nil || started {
		t.Fatal("untrusted project should be rejected before execution")
	}
	if _, statErr := os.Stat(filepath.Join(project, "fake-cli")); statErr == nil {
		t.Fatal("unexpected fake executable")
	}
}

func has(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestCodexExecArgsAcceptedByNonInteractiveCLI(t *testing.T) {
	for _, profile := range []string{"restricted", "balanced", "autonomous"} {
		t.Run(profile, func(t *testing.T) {
			_, args, err := commandFor(Request{Agent: "codex", Prompt: "do work", Profile: profile, Project: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			if !has(args, "--skip-git-repo-check") {
				t.Fatalf("codex exec must run in non-git trusted folders: %#v", args)
			}
			if has(args, "--ask-for-approval") {
				t.Fatalf("codex exec rejects --ask-for-approval: %#v", args)
			}
		})
	}
}
