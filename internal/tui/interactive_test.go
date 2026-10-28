package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	for _, want := range []string{"Confirme a confiança do workspace", "Pasta:", "Padrão: manter sem confiança", "> Manter sem confiança"} {
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
	for _, want := range []string{"+G>+ gasacli", "Confiança: concedida", "Agentes prontos: 2", "Session ready.", "Enter envia"} {
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

func TestSetupCommandStillSuspendsChatBeforeSequentialWizard(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.submit("/setup")
	if !model.setupRequested || model.closed {
		t.Fatalf("setup state=%+v", model)
	}
	// The wizard still runs as a second sequential Bubble Tea program; the chat model retains its state.
}

func TestInteractiveCtrlCAndCtrlDKeepDraftAndExit(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.chat.SetValue("unsent draft")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(interactiveModel)
	if model.chat.Value() != "unsent draft" {
		t.Fatal("Ctrl+C lost the current draft")
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	model = updated.(interactiveModel)
	if model.chat.Value() != "unsent draft" || command == nil {
		t.Fatal("Ctrl+D must exit while retaining the in-memory draft")
	}
}

func TestInteractiveWelcomeAndEnterSends(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	model := newInteractiveModel(&Session{ProjectPath: "C:/workspace", ReadyAgents: 1}, "C:/workspace", true)
	model.width, model.height = 80, 24
	view := model.View()
	for _, want := range []string{"+G>+ gasacli", "Projeto: workspace", "Confiança: concedida", "Agentes prontos: 1", "/help", "/agents", "/doctor"} {
		if !strings.Contains(view, want) {
			t.Errorf("welcome card missing %q: %s", want, view)
		}
	}
	model.chat.SetValue("hello")
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	if command != nil || model.chat.Value() != "" || !strings.Contains(strings.Join(model.lines, "\n"), "Model chat is not connected yet") {
		t.Fatalf("Enter did not send the message: model=%+v", model)
	}
}

func TestInteractiveWelcomeGoldenTerminalSizes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 36}} {
		name := fmt.Sprintf("welcome-%dx%d.golden", size.width, size.height)
		t.Run(name, func(t *testing.T) {
			model := newInteractiveModel(&Session{ProjectPath: "C:/workspace", ReadyAgents: 1}, "C:/workspace", true)
			model.width, model.height = size.width, size.height
			got := normalizeInteractiveGolden(model.View())
			path := filepath.Join("testdata", name)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("welcome view differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
			}
		})
	}
}

func normalizeInteractiveGolden(view string) string {
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

func TestInteractiveAgentsStayCheckingUntilDiscoveryResult(t *testing.T) {
	model := newInteractiveModel(&Session{ProjectPath: "C:/workspace", DiscoverReady: func() int { return 3 }}, "C:/workspace", true)
	if !strings.Contains(model.welcomeCard(), "checking...") {
		t.Fatal("discovery did not start in checking state")
	}
	updated, _ := model.Update(readyAgentsResult{count: 2})
	model = updated.(interactiveModel)
	if model.agentsChecking || !strings.Contains(model.welcomeCard(), "Agentes prontos: 2") {
		t.Fatal("completed discovery result was not shown")
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
