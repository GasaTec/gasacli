package orchestration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/prompts"
)

func TestSelectReviewerPrefersDifferentReadyAgentThenFreshDeveloperSession(t *testing.T) {
	results := []discovery.Result{{ID: "codex", Status: discovery.Installed}, {ID: "claude", Status: discovery.Ready}}
	if got := SelectReviewer(results, "claude"); got != "codex" {
		t.Fatalf("reviewer = %q", got)
	}
	if got := SelectReviewer([]discovery.Result{{ID: "claude", Status: discovery.Ready}}, "claude"); got != "claude" {
		t.Fatalf("single-agent reviewer = %q", got)
	}
	if got := SelectReviewer(nil, ""); got != "" {
		t.Fatalf("no-agent reviewer = %q", got)
	}
}

func TestParseReviewOutputShowsBoundedRawTailForUnstructuredAnswer(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	result := parseReviewOutput("answer --password hunter2", false)
	if result.Verdict != engine.ReviewIndeterminate || len(result.Findings) != 1 || strings.Contains(result.Findings[0], "hunter2") {
		t.Fatalf("unstructured review = %#v", result)
	}
	long := strings.Repeat("x", 2000)
	result = parseReviewOutput(long, true)
	if result.Verdict != engine.ReviewIndeterminate || len([]rune(result.Findings[0])) > 1201 {
		t.Fatalf("truncated review = %#v", result)
	}
}

func TestReviewerDefaultPromptLimitsReviewToGoalAndGates(t *testing.T) {
	for _, language := range []string{"pt-BR", "en"} {
		t.Setenv("GASACLI_LANG", language)
		effective, err := prompts.Effective(t.TempDir(), "", "tech-lead", language)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{"goal", "gate", "trailing newline"} {
			if language == "pt-BR" {
				if required == "goal" {
					required = "objetivo"
				}
				if required == "gate" {
					required = "gates"
				}
				if required == "trailing newline" {
					required = "quebra de linha final"
				}
			}
			if !strings.Contains(strings.ToLower(effective.Body), strings.ToLower(required)) {
				t.Errorf("%s reviewer prompt missing %q", language, required)
			}
		}
	}
}

func TestReviewerTimeoutConfigIsValidated(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("agents:\n  reviewer_timeout: 90s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadFrom(home)
	if err != nil || cfg.Agents.ReviewerTimeout != "90s" {
		t.Fatalf("reviewer timeout config = %+v, %v", cfg.Agents, err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("agents:\n  reviewer_timeout: invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFrom(home); err == nil {
		t.Fatal("invalid reviewer timeout was accepted")
	}
}

func TestFakeReviewerTimeoutAndProgress(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	var progress atomic.Int32
	_, err := executeReviewWithTimeout(context.Background(), 30*time.Millisecond, 5*time.Millisecond, func(time.Duration) {
		progress.Add(1)
	}, func(ctx context.Context) (adapters.Result, error) {
		<-ctx.Done()
		return adapters.Result{Text: "partial reviewer output"}, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fake reviewer timeout error = %v", err)
	}
	if progress.Load() == 0 {
		t.Fatal("review progress callback was not called")
	}
}

func TestSelectDeveloperPrefersReadyAndHonorsAvailablePreference(t *testing.T) {
	results := []discovery.Result{{ID: "codex", Status: discovery.Installed}, {ID: "claude", Status: discovery.Ready}}
	if got := SelectDeveloper(results, ""); got != "claude" {
		t.Fatalf("default developer = %q, want ready claude", got)
	}
	if got := SelectDeveloper(results, "codex"); got != "codex" {
		t.Fatalf("preferred developer = %q", got)
	}
	if got := SelectDeveloper(results, "gemini"); got != "" {
		t.Fatalf("unavailable preference = %q", got)
	}
	if AgentStatus(results, "gemini") != discovery.Missing {
		t.Fatalf("missing agent status = %q", AgentStatus(results, "gemini"))
	}
}

func TestGateCommandsPreserveArgvAndOrder(t *testing.T) {
	configured := config.ProjectGates{Test: []string{"go", "test"}, Lint: []string{"lint", "run"}}
	got := gateCommands(configured)
	if len(got) != 2 || got[0].Name != "test" || got[1].Name != "lint" || got[0].Argv[1] != "test" {
		t.Fatalf("commands = %#v", got)
	}
	got[0].Argv[0] = "mutated"
	if configured.Test[0] != "go" {
		t.Fatal("gate conversion aliased project configuration")
	}
}

func TestSnapshotTracksFilesButIgnoresGitMetadata(t *testing.T) {
	project := t.TempDir()
	file := filepath.Join(project, "hello.txt")
	if err := os.WriteFile(file, []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := snapshot(project)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(project, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, ".git", "ignored"), []byte("metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := snapshot(project)
	if err != nil || second != first {
		t.Fatalf("git metadata changed snapshot %q -> %q: %v", first, second, err)
	}
	if err = os.WriteFile(file, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	third, err := snapshot(project)
	if err != nil || third == second {
		t.Fatalf("file content did not change snapshot: %q %q %v", second, third, err)
	}
}

func TestUnconfirmedLoginIsALastResortNotARefusal(t *testing.T) {
	results := []discovery.Result{{ID: "claude", Status: discovery.NeedsLogin}}
	if !AgentAvailable(results, "claude") {
		t.Fatal("an explicit choice must not be refused because the login probe was negative")
	}
	if got := SelectDeveloper(results, ""); got != "claude" {
		t.Fatalf("default developer = %q, want claude as the last resort", got)
	}
	results = []discovery.Result{{ID: "claude", Status: discovery.NeedsLogin}, {ID: "codex", Status: discovery.Ready}}
	if got := SelectDeveloper(results, ""); got != "codex" {
		t.Fatalf("a confirmed login must win, got %q", got)
	}
}
