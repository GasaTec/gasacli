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

func TestStderrFailureSummaryIsBoundedAndRedacted(t *testing.T) {
	var tail stderrTailBuffer
	for i := 0; i < 30; i++ {
		_, _ = tail.Write([]byte("diagnostic line\n"))
	}
	_, _ = tail.Write([]byte("request failed --api-key top-secret\n"))
	got := tail.String()
	if strings.Contains(got, "top-secret") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("stderr tail was not redacted: %q", got)
	}
	if len(got) > 2<<10 || strings.Count(got, "\n") > 19 {
		t.Fatalf("stderr tail is not bounded: bytes=%d lines=%d", len(got), strings.Count(got, "\n")+1)
	}
	if got := classifyFailure("codex", 1, "request timed out: authentication required"); !strings.Contains(got, "network/auth error") {
		t.Fatalf("network/auth failure was not identified: %q", got)
	}
	if got := classifyFailure("claude", 2, "bad request"); !strings.Contains(got, "exited with status 2") {
		t.Fatalf("exit failure was not identified: %q", got)
	}
}

func TestLaunchRetryIsLimitedToOnePreEventNonzeroExit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		result  Result
		retried bool
		want    bool
	}{
		{name: "first pre-event failure", result: Result{ExitCode: 1}, want: true},
		{name: "already retried", result: Result{ExitCode: 1}, retried: true},
		{name: "emitted event", result: Result{ExitCode: 1, Events: 1}},
		{name: "success", result: Result{ExitCode: 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldRetryLaunch(tc.result, tc.retried); got != tc.want {
				t.Fatalf("shouldRetryLaunch() = %v, want %v", got, tc.want)
			}
		})
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

func TestAllowedEnvironmentIsCaseInsensitiveAndKeepsSecretsOut(t *testing.T) {
	got := allowedEnvironmentFrom([]string{
		`Path=C:\Program Files\nodejs;C:\Windows`,
		`SystemRoot=C:\Windows`,
		`PATHEXT=.COM;.EXE;.CMD`,
		`ANTHROPIC_API_KEY=secret`,
		`openai_api_key=secret`,
		`UNRELATED_TOKEN=secret`,
	}, nil)
	joined := strings.Join(got, "\n")
	for _, want := range []string{`Path=C:\Program Files\nodejs;C:\Windows`, `SystemRoot=C:\Windows`, `PATHEXT=.COM;.EXE;.CMD`} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	for _, leaked := range []string{"ANTHROPIC_API_KEY", "openai_api_key", "UNRELATED_TOKEN", "secret"} {
		if strings.Contains(joined, leaked) {
			t.Fatalf("environment leaked %q: %v", leaked, got)
		}
	}
}

func TestAllowedEnvironmentKeepsAgentConfigDirectories(t *testing.T) {
	got := strings.Join(allowedEnvironmentFrom([]string{
		`CLAUDE_CONFIG_DIR=C:\profiles\claude`,
		`GH_CONFIG_DIR=C:\profiles\gh`,
		`XDG_CONFIG_HOME=C:\cfg`,
		`GITHUB_TOKEN=secret`,
	}, nil), "\n")
	for _, want := range []string{`CLAUDE_CONFIG_DIR=C:\profiles\claude`, `GH_CONFIG_DIR=C:\profiles\gh`, `XDG_CONFIG_HOME=C:\cfg`} {
		if !strings.Contains(got, want) {
			t.Fatalf("config directory %q was dropped: %s", want, got)
		}
	}
	if strings.Contains(got, "GITHUB_TOKEN") {
		t.Fatalf("token leaked: %s", got)
	}
}

func TestCaptureMetricsDistinguishesZeroFromMissing(t *testing.T) {
	var result Result
	captureMetrics([]byte(`{"usage":{"input_tokens":0},"cost_usd":0}`), &result)
	if !result.InputTokensKnown || result.InputTokens != 0 || result.OutputTokensKnown || !result.CostKnown || result.CostUSD != 0 {
		t.Fatalf("unexpected metrics: %#v", result)
	}
	captureMetrics([]byte(`{"usage":{"output_tokens":3}}`), &result)
	if !result.InputTokensKnown || !result.OutputTokensKnown || result.OutputTokens != 3 {
		t.Fatalf("unexpected merged metrics: %#v", result)
	}
	var missing Result
	captureMetrics([]byte(`{"usage":{}}`), &missing)
	if missing.InputTokensKnown || missing.OutputTokensKnown || missing.CostKnown {
		t.Fatalf("absent metrics marked known: %#v", missing)
	}
}
