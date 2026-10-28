package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/storage"
)

type engineCommandContext struct {
	home, project string
	trusted       bool
	discover      any
	messages      []string
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
func (c *engineCommandContext) Emit(message string)                 { c.messages = append(c.messages, message) }
func (c *engineCommandContext) Ask(PermissionRequest) (bool, error) { return false, nil }

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
	run, err := core.CreateRun(context.Background(), project, "test goal")
	if err != nil {
		t.Fatal(err)
	}
	if err = core.AddTasks(context.Background(), run.ID, []engine.Task{{ID: "a", Name: "task"}}); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	cc := &engineCommandContext{home: home, project: project, trusted: true}
	status, err := statusCommand(context.Background(), cc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, string(engine.Planning)) || !strings.Contains(status, run.ID) {
		t.Fatalf("status = %q", status)
	}
	list, err := runsCommand(context.Background(), cc, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(list, run.ID) || !strings.Contains(list, "test goal") {
		t.Fatalf("runs = %q", list)
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
