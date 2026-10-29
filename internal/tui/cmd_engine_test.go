package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/storage"
)

type engineCommandContext struct {
	home, project string
	trusted       bool
	discover      any
	messages      []string
	confirm       bool
}

func (c *engineCommandContext) ProjectPath() string   { return c.project }
func (c *engineCommandContext) Trusted() bool         { return c.trusted }
func (c *engineCommandContext) ConfigHome() string    { return c.home }
func (c *engineCommandContext) DiscoveryResults() any { return c.discover }
func (c *engineCommandContext) Storage() any          { return nil }
func (c *engineCommandContext) Notify(string)         {}
func (c *engineCommandContext) AppendMessage(message string) {
	c.messages = append(c.messages, message)
}
func (c *engineCommandContext) Emit(message string)                     { c.messages = append(c.messages, message) }
func (c *engineCommandContext) Ask(PermissionRequest) (bool, error)     { return false, nil }
func (c *engineCommandContext) Confirm(PermissionRequest) (bool, error) { return c.confirm, nil }

func TestStatusAndRunsReadPersistedProjectState(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store, err := storage.Open(home + "/gasacli.db")
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	cc := &engineCommandContext{home: home, project: project, trusted: true}
	status, err := statusCommand(context.Background(), cc, nil)
	if err != nil || status != "No orchestration run for this project." {
		t.Fatalf("empty project status = %q err=%v", status, err)
	}
	store, err = storage.Open(home + "/gasacli.db")
	if err != nil {
		t.Fatal(err)
	}
	core, err = engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	run, err := core.CreateRun(context.Background(), project, "test goal")
	if err != nil {
		t.Fatal(err)
	}
	if err = core.RecordChangedFiles(context.Background(), run.ID, []string{"hello.txt"}); err != nil {
		t.Fatal(err)
	}
	if err = core.RecordAgentMetrics(context.Background(), run.ID, engine.AgentMetrics{Role: "developer", Agent: "claude", ExitCode: 1, DurationMS: 1500, FailureReason: "network/auth error: request timed out"}); err != nil {
		t.Fatal(err)
	}
	if err = core.AddTasks(context.Background(), run.ID, []engine.Task{{ID: "a", Name: "task"}}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	_ = store.Close()
	status, err = statusCommand(context.Background(), cc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, string(engine.Planning)) || !strings.Contains(status, shortRunID(run.ID)) {
		t.Fatalf("status = %q", status)
	}
	list, err := runsCommand(context.Background(), cc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, shortRunID(run.ID)) || !strings.Contains(list, "test goal") {
		t.Fatalf("runs = %q", list)
	}
	detail, err := runsCommand(context.Background(), cc, []string{shortRunID(run.ID)})
	if err != nil || !strings.Contains(detail, "Details:") || !strings.Contains(detail, shortRunID(run.ID)) || !strings.Contains(detail, "hello.txt") || !strings.Contains(detail, "network/auth error") {
		t.Fatalf("run detail = %q err=%v", detail, err)
	}
}

func TestFormatRunEventsUsesHumanTextAndShortId(t *testing.T) {
	t.Setenv("GASACLI_LANG", "pt-BR")
	i18n.SetLanguage("pt-BR")
	events := []engine.Event{
		{Type: "run.state", Payload: []byte(`{"from":"REVIEW","to":"GATES","reason":"independent review approved"}`)},
		{Type: "gate.finished", Payload: []byte(`{"cycle":"a","result":{"name":"test","passed":true,"duration":1200000000}}`)},
		{Type: "agent.finished", Payload: []byte(`{"role":"developer","agent":"claude","duration_ms":29000,"cost_usd":0.02,"cost_known":true}`)},
	}
	if got := formatRunEvent(events[0]); !strings.Contains(got, "Revisão independente aprovada") || strings.Contains(got, "{") {
		t.Fatalf("review event = %q", got)
	}
	if got := formatRunEvent(events[1]); !strings.Contains(got, "Verificação test: passou (1,2 s)") || strings.Contains(got, "{") {
		t.Fatalf("gate event = %q", got)
	}
	if got := formatRunEvent(events[2]); !strings.Contains(got, "Desenvolvedor (claude) terminou: 29,0 s, US$ 0,02") || strings.Contains(got, "{") {
		t.Fatalf("agent event = %q", got)
	}
	if got := shortRunID("0123456789abcdef"); got != "01234567" {
		t.Fatalf("short run id = %q", got)
	}
}

func TestReviewFindingsRenderImmediatelyAfterChangesRequested(t *testing.T) {
	t.Setenv("GASACLI_LANG", "pt-BR")
	i18n.SetLanguage("pt-BR")
	got := formatRunEvent(engine.Event{Type: "review.finished", Payload: []byte(`{"reviewer":"codex","verdict":"changes_requested","findings":["O objetivo pede exatamente hi; o arquivo contém hello.","Falta atender ao gate configurado."]}`)})
	if !strings.HasPrefix(got, "Revisão independente: alterações solicitadas por codex\n- O objetivo") || !strings.Contains(got, "\n- Falta atender ao gate configurado.") {
		t.Fatalf("review findings were not rendered after the verdict: %q", got)
	}
}

func TestGoalRequiresTrustedProjectAndInstalledAgent(t *testing.T) {
	cc := &engineCommandContext{home: t.TempDir(), project: t.TempDir(), trusted: false}
	if _, err := goalCommand(context.Background(), cc, []string{"create", "file"}); err == nil || !strings.Contains(err.Error(), "trust") {
		t.Fatalf("untrusted goal error = %v", err)
	}
	cc.trusted = true
	if _, err := goalCommand(context.Background(), cc, []string{"create", "file"}); err == nil || !strings.Contains(err.Error(), "coding CLI") {
		t.Fatalf("missing agent error = %v", err)
	}
}

func TestGoalReportsConfiguredAgentThatIsMissing(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	if err := os.WriteFile(home+"/config.yaml", []byte("agents:\n  developer: claude\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cc := &engineCommandContext{home: home, project: project, trusted: true, discover: []discovery.Result{{ID: "claude", Status: discovery.Missing}}}
	if _, err := goalCommand(context.Background(), cc, []string{"--agent", "claude", "create", "file"}); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unavailable configured agent error = %v", err)
	}
}

func TestParseGoalArgsSupportsAgentOptions(t *testing.T) {
	goal, agent, reviewer, err := parseGoalArgs([]string{"--agent", "claude", "create", "hello.txt", "--reviewer", "codex"})
	if err != nil || goal != "create hello.txt" || agent != "claude" || reviewer != "codex" {
		t.Fatalf("parsed goal=%q agent=%q reviewer=%q err=%v", goal, agent, reviewer, err)
	}
	if _, _, _, err := parseGoalArgs([]string{"--agent"}); err == nil {
		t.Fatal("missing agent id accepted")
	}
}

func TestCancelMarksRunAndRejectsOtherProject(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store, err := storage.Open(home + "/gasacli.db")
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	run, err := core.CreateRun(context.Background(), project, "goal")
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	cc := &engineCommandContext{home: home, project: project, trusted: true}
	if _, err = cancelCommand(context.Background(), cc, []string{run.ID}); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(home + "/gasacli.db")
	if err != nil {
		t.Fatal(err)
	}
	core, err = engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := core.GetRun(context.Background(), run.ID)
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Status != engine.Cancelled {
		t.Fatalf("status = %s", saved.Status)
	}
	cc.project = t.TempDir()
	if _, err = cancelCommand(context.Background(), cc, []string{run.ID}); err == nil {
		t.Fatal("cross-project cancellation was accepted")
	}
}

func TestCancelWithoutArgsRequiresConfirmation(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	store, err := storage.Open(home + "/gasacli.db")
	if err != nil {
		t.Fatal(err)
	}
	core, err := engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	run, err := core.CreateRun(context.Background(), project, "goal")
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	cc := &engineCommandContext{home: home, project: project, trusted: true}
	if message, err := cancelCommand(context.Background(), cc, nil); err != nil || message != "Cancellation declined." {
		t.Fatalf("unconfirmed cancellation = %q err=%v", message, err)
	}
	cc.confirm = true
	if _, err := cancelCommand(context.Background(), cc, nil); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(home + "/gasacli.db")
	if err != nil {
		t.Fatal(err)
	}
	core, err = engine.New(store)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := core.GetRun(context.Background(), run.ID)
	_ = store.Close()
	if err != nil || saved.Status != engine.Cancelled {
		t.Fatalf("cancelled run = %s err=%v", saved.Status, err)
	}
}
