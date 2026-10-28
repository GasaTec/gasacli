package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/trust"
	tea "github.com/charmbracelet/bubbletea"
)

func TestInteractiveTrustAndKeys(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	session := &Session{ProjectPath: t.TempDir(), TrustStore: trust.NewStore(t.TempDir())}
	model := newInteractiveModel(session, session.ProjectPath, false)
	model.width, model.height = 80, 24
	view := model.View()
	for _, want := range []string{"GasaCLI needs your approval", "Exact folder:", "Default: No", "> No, leave it untrusted"} {
		if !strings.Contains(view, want) {
			t.Errorf("trust view missing %q: %s", want, view)
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	model = updated.(interactiveModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	if !model.trusted {
		t.Fatal("y then enter did not grant trust")
	}
	trusted, err := session.TrustStore.IsTrusted(session.ProjectPath)
	if err != nil || !trusted {
		t.Fatalf("persisted trust=%v err=%v", trusted, err)
	}
}

func TestInteractiveChatGoldenAndCommandPalette(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	model := newInteractiveModel(&Session{ProjectPath: t.TempDir(), ReadyAgents: 2}, t.TempDir(), true)
	model.width, model.height = 100, 28
	model.append("Session ready. Type / to browse commands.")
	view := model.View()
	for _, want := range []string{"G> GasaCLI", "Trust: granted", "Agents ready: 2", "Session ready.", "Ctrl+S sends"} {
		if !strings.Contains(view, want) {
			t.Errorf("chat view missing %q: %s", want, view)
		}
	}
	model.chat.SetValue("/go")
	matches := model.matches()
	if len(matches) != 1 || matches[0].name != "/goal" {
		t.Fatalf("matches=%v", matches)
	}
	model.submit("/goal refactor")
	if !strings.Contains(strings.Join(model.lines, "\n"), "planned (issue #11)") {
		t.Fatal("planned command did not show its issue")
	}
}

func TestSetupCommandRequestsWizardWithoutDroppingChat(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.submit("/setup")
	if !model.setupRequested || model.closed {
		t.Fatalf("setup state=%+v", model)
	}
}

func TestInteractiveCtrlCAndCtrlDKeepDraft(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.chat.SetValue("unsent draft")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(interactiveModel)
	if model.chat.Value() != "unsent draft" {
		t.Fatal("Ctrl+C lost the current draft")
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(interactiveModel)
	if model.chat.Value() != "unsent draft" || command != nil {
		t.Fatal("Ctrl+D discarded the draft or exited")
	}
}

func TestAsyncDiscoveryShowsSpinnerAndReturnsResult(t *testing.T) {
	session := &Session{Agents: func(w io.Writer) { _, _ = io.WriteString(w, "Codex available") }}
	model := newInteractiveModel(session, t.TempDir(), true)
	model.chat.SetValue("/agents")
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(interactiveModel)
	if command == nil || !model.busy {
		t.Fatal("diagnostic did not start asynchronously")
	}
	view := model.View()
	if !strings.Contains(view, "Discovering installed tools") {
		t.Fatal("busy status was not rendered")
	}
	result := model.runDiagnostic("/agents")()
	updated, _ = model.Update(result)
	model = updated.(interactiveModel)
	if model.busy || !strings.Contains(strings.Join(model.lines, "\n"), "Codex available") {
		t.Fatal("diagnostic result did not update the conversation")
	}
}
