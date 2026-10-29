package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/engine"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/trust"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestFooterShowsOnlyReportedMetrics(t *testing.T) {
	t.Setenv("TERM", "dumb")
	m := newInteractiveModel(&Session{}, t.TempDir(), true)
	m.width, m.height = 120, 36
	m.lastAgentMetrics = &engine.AgentMetrics{Agent: "codex", InputTokens: 0, InputTokensKnown: true, OutputTokens: 12, CostUSD: 0.25}
	view := m.View()
	if !strings.Contains(view, "Últ. codex in 0") || strings.Contains(view, "out 12") || strings.Contains(view, "US$ 0.2500") {
		t.Fatalf("unreported metrics in footer: %s", view)
	}
	m.lastAgentMetrics.OutputTokensKnown, m.lastAgentMetrics.CostKnown = true, true
	view = m.View()
	if !strings.Contains(view, "out 12") || !strings.Contains(view, "US$ 0.2500") {
		t.Fatalf("reported metrics missing: %s", view)
	}
}

func TestAgentAttemptStatusRejectsStaleEvents(t *testing.T) {
	m := newInteractiveModel(&Session{}, t.TempDir(), true)
	activeRuns.Store("attempt-test", activeRun{project: m.project, cancel: func() {}})
	defer activeRuns.Delete("attempt-test")
	update := func(attempt int, status string) {
		updated, _ := m.Update(attemptMsg{run: "attempt-test", agent: "codex", role: "developer", event: adapters.AttemptEvent{ExecutionID: 7, Attempt: attempt, Status: status}})
		m = updated.(interactiveModel)
	}
	update(1, "started")
	update(2, "started")
	update(1, "failed")
	if len(m.attempts) != 1 || m.attempts[attemptKey{"attempt-test", 7}].attempt != 2 {
		t.Fatal("stale completion removed active attempt")
	}
	if !strings.Contains(m.sidebarText(), "codex [running]") {
		t.Fatal("active agent is missing from sidebar")
	}
	update(2, "succeeded")
	if len(m.attempts) != 0 {
		t.Fatal("completed attempt remains active")
	}
}

func TestSidebarEmptyStateAndPlainTerminal(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	m := newInteractiveModel(&Session{}, "workspace", true)
	m.width, m.height = 120, 36
	view := m.View()
	if !strings.Contains(view, "No agents discovered") || strings.Contains(view, "\x1b[") {
		t.Fatalf("plain terminal must have an explicit empty state and no ANSI styling: %q", view)
	}
	m.agentsChecking = true
	if !strings.Contains(m.sidebarText(), "Discovering...") {
		t.Fatal("discovery in progress must not be presented as an empty result")
	}
	m.agentsChecking = false
	m.sidebar.Err = errors.New("discovery failed")
	if text := m.sidebarText(); !strings.Contains(text, "Discovery unavail") || strings.Contains(text, "No agents") {
		t.Fatalf("failed discovery must not look like an empty result: %q", text)
	}
	m.width, m.height = 80, 24
	view = m.View()
	if !strings.Contains(view, "CONVERSA") || !strings.Contains(view, "Ctrl+P projects") || !strings.Contains(view, "Ctrl+K commands") {
		t.Fatalf("chat heading or shortcuts missing at 80 columns: %s", view)
	}
}

func TestProjectPickerKeepsSelectionVisible(t *testing.T) {
	t.Setenv("TERM", "dumb")
	m := newInteractiveModel(&Session{}, t.TempDir(), true)
	m.width, m.height = 80, 18
	m.projectPicker = true
	for i := 0; i < 25; i++ {
		m.sidebar.Projects = append(m.sidebar.Projects, sidebarProject{Path: fmt.Sprintf("/project-%02d", i)})
	}
	m.projectIndex = 24
	view := m.View()
	if !strings.Contains(view, "> /project-24") {
		t.Fatalf("selected project not visible: %s", view)
	}
}

func TestSecondCtrlCConfirmsWhenGoalIsActive(t *testing.T) {
	m := newInteractiveModel(&Session{}, t.TempDir(), true)
	m.busy = true
	m.commandCancel = func() {}
	m.lastCtrlCValid = true
	m.lastCtrlC = time.Now()
	activeRuns.Store("ctrl-c-test", activeRun{project: m.project, cancel: func() {}})
	defer activeRuns.Delete("ctrl-c-test")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil || !updated.(interactiveModel).quitConfirmation {
		t.Fatal("active background goal must require confirmation")
	}
}

func TestInteractiveTrustAndKeys(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	pinTestLanguage(t, "pt-BR")
	session := &Session{ProjectPath: t.TempDir(), TrustStore: trust.NewStore(t.TempDir())}
	model := newInteractiveModel(session, session.ProjectPath, false)
	model.width, model.height = 80, 24
	view := model.View()
	for _, want := range []string{"Confirme a confiança nesta pasta", "Sim, confiar nesta pasta", "Não, manter sem confiança", "> Não, manter sem confiança"} {
		if !strings.Contains(view, want) {
			t.Errorf("trust view missing %q: %s", want, view)
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(interactiveModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})
	model = updated.(interactiveModel)
	if !model.trusted {
		t.Fatal("Up then Space did not grant trust")
	}
	trusted, err := session.TrustStore.IsTrusted(session.ProjectPath)
	if err != nil || !trusted {
		t.Fatalf("persisted trust=%v err=%v", trusted, err)
	}
}

func TestTrustDialogGolden80x24(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	pinTestLanguage(t, "pt-BR")
	model := newInteractiveModel(&Session{}, "/tmp/project", false)
	model.width, model.height = 80, 24
	got := normalizeInteractiveGolden(model.View())
	path := filepath.Join("testdata", "trust-80x24.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != normalizeInteractiveGolden(string(want)) {
		t.Fatalf("trust dialog differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestTrustDialogKeyMatrix(t *testing.T) {
	keys := []struct {
		name string
		key  tea.KeyMsg
	}{{"enter", tea.KeyMsg{Type: tea.KeyEnter}}, {"esc", tea.KeyMsg{Type: tea.KeyEsc}}, {"space", tea.KeyMsg{Type: tea.KeySpace}}, {"up", tea.KeyMsg{Type: tea.KeyUp}}, {"down", tea.KeyMsg{Type: tea.KeyDown}}, {"left", tea.KeyMsg{Type: tea.KeyLeft}}, {"right", tea.KeyMsg{Type: tea.KeyRight}}, {"letter", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")}}, {"y", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}}, {"n", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}}, {"tab", tea.KeyMsg{Type: tea.KeyTab}}}
	for _, test := range keys {
		t.Run(test.name, func(t *testing.T) {
			home, project := t.TempDir(), t.TempDir()
			model := newInteractiveModel(&Session{ProjectPath: project, TrustStore: trust.NewStore(home)}, project, false)
			updated, cmd := model.Update(test.key)
			got := updated.(interactiveModel)
			switch test.name {
			case "enter", "space":
				if !got.trustDenied || got.trusted {
					t.Fatal("default No selection did not show the untrusted panel")
				}
			case "esc":
				if cmd == nil || !got.closed || got.quitErr != nil {
					t.Fatal("Esc did not exit cleanly")
				}
			case "up":
				if got.trustChoice != 0 {
					t.Fatal("Up did not move selection to Yes")
				}
			case "down":
				if got.trustChoice != 0 {
					t.Fatal("Down did not wrap from the default No selection to Yes")
				}
			default:
				if got.trustChoice != 1 || got.trustDenied || got.trusted {
					t.Fatalf("%q changed trust choice: %+v", test.name, got)
				}
			}
		})
	}
}

func TestWizardTrustDecisionOpensUntrustedPanelWithoutPrompt(t *testing.T) {
	pinTestLanguage(t, "pt-BR")
	model := newInteractiveModel(&Session{TrustDecisionHandled: true}, t.TempDir(), false)
	if !model.trustDenied || strings.Contains(model.View(), i18n.T("tui.trust.yes")) {
		t.Fatal("an explicit wizard No decision reopened the trust prompt")
	}
	if !strings.Contains(model.View(), "gasa setup") {
		t.Fatal("the untrusted panel does not explain how to change trust")
	}
}

func TestInteractiveChatGoldenAndCommandPalette(t *testing.T) {
	i18n.SetLanguage("pt-BR")
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
	if !strings.Contains(strings.Join(model.lines, "\n"), "coding CLI") {
		t.Fatal("goal command did not report the missing coding CLI")
	}
}

func TestRegistryDrivesPaletteAndHelp(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.chat.SetValue("/")
	matches := model.matches()
	if len(matches) != len(RegisteredCommands()) {
		t.Fatalf("palette has %d commands, registry has %d", len(matches), len(RegisteredCommands()))
	}
	if !strings.Contains(helpText(), "/permissions") || strings.Contains(helpText(), "/permissions     Configure execution permissions (planned") {
		t.Fatalf("help omitted active permissions command: %s", helpText())
	}
	for _, language := range []string{"pt-BR", "en"} {
		keys, err := i18n.CatalogKeys(language)
		if err != nil {
			t.Fatal(err)
		}
		available := make(map[string]bool, len(keys))
		for _, key := range keys {
			available[key] = true
		}
		for _, command := range RegisteredCommands() {
			if !available[command.Summary] {
				t.Errorf("%s catalog missing %s", language, command.Summary)
			}
			if strings.TrimSpace(commandSummary(command)) == "" || commandSummary(command) == command.Summary {
				t.Errorf("%s registry command %s has no translated summary", language, command.Name)
			}
		}
		if !strings.Contains(helpText(), "GasaCLI commands:") {
			t.Errorf("real registry did not render for %s", language)
		}
	}
}

func TestUnknownCommandSuggestsRegisteredCommand(t *testing.T) {
	i18n.SetLanguage("en")
	if got := unknownCommandMessage("/hlep"); !strings.Contains(got, "/help") {
		t.Fatalf("suggestion=%q", got)
	}
	i18n.SetLanguage("pt-BR")
}

func TestRegisterRejectsDuplicateNameAndAlias(t *testing.T) {
	for name, command := range map[string]Command{
		"name":  {Name: "/help"},
		"alias": {Name: "/new-test-command", Aliases: []string{"/help"}},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("Register accepted duplicate name or alias")
				}
			}()
			Register(command)
		})
	}
}

func TestHelpCatalogGoldens(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage("pt-BR") })
	for _, language := range []string{"pt-BR", "en"} {
		t.Run(language, func(t *testing.T) {
			i18n.SetLanguage(language)
			got := helpTextFor(helpGoldenFixture()) + "\n"
			path := filepath.Join("testdata", "help-"+language+"-80x24.golden")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if normalizeInteractiveGolden(got) != normalizeInteractiveGolden(string(want)) {
				t.Fatalf("/help differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
			}
		})
	}
}

func helpGoldenFixture() []Command {
	return []Command{
		{Name: "/agents", Summary: "command.agents"},
		{Name: "/backup", Summary: "command.backup"},
		{Name: "/backups", Summary: "command.backups"},
		{Name: "/brain", Summary: "command.brain"},
		{Name: "/config", Summary: "command.config"},
		{Name: "/doctor", Summary: "command.doctor"},
		{Name: "/exit", Summary: "command.exit"},
		{Name: "/github", Summary: "command.github"},
		{Name: "/goal", Summary: "command.goal"},
		{Name: "/help", Summary: "command.help"},
		{Name: "/permissions", Summary: "command.permissions", Planned: true, IssueRef: "#31"},
		{Name: "/prompts", Summary: "command.prompts"},
		{Name: "/quit", Summary: "command.exit"},
		{Name: "/restore", Summary: "command.restore"},
		{Name: "/send", Summary: "command.send"},
		{Name: "/sessions", Summary: "command.sessions"},
		{Name: "/resume", Summary: "command.resume", Planned: true, IssueRef: "#11"},
		{Name: "/setup", Summary: "command.setup"},
		{Name: "/skills", Summary: "command.skills"},
		{Name: "/status", Summary: "command.status"},
		{Name: "/web", Summary: "command.web"},
	}
}

func TestSetupCommandStillSuspendsChatBeforeSequentialWizard(t *testing.T) {
	model := newInteractiveModel(&Session{Interactive: true}, t.TempDir(), true)
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

func TestIncomingOutputPreservesScrollPosition(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.view.Height = 3
	model.append("one\ntwo\nthree\nfour\nfive")
	if !model.view.AtBottom() {
		t.Fatal("new output should initially follow the bottom")
	}
	model.view.LineUp(2)
	before := model.view.YOffset
	model.append("six")
	if model.view.YOffset != before {
		t.Fatalf("incoming output moved scroll from %d to %d", before, model.view.YOffset)
	}
	model.view.GotoBottom()
	model.append("seven")
	if !model.view.AtBottom() {
		t.Fatal("output should follow again after returning to bottom")
	}
}

func TestBusyCommandPreservesDraftAndCancel(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.busy = true
	canceled := false
	model.commandCancel = func() { canceled = true }
	model.chat.SetValue("/status")
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	if command != nil || model.chat.Value() != "/status" || len(model.history) != 0 {
		t.Fatal("busy submission must preserve the draft without starting another command")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(interactiveModel)
	if !canceled || model.chat.Value() != "/status" {
		t.Fatal("Ctrl+C must cancel the original command and preserve the draft")
	}
}

func TestEscClosesCommandSuggestionsWithoutClearingInput(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.chat.SetValue("/status")
	model.palette = true
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(interactiveModel)
	if model.palette || model.chat.Value() != "/status" {
		t.Fatalf("Esc changed the command input or left suggestions open: palette=%v value=%q", model.palette, model.chat.Value())
	}
}

func TestPaletteEnterSelectsHighlightedCommand(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.chat.SetValue("/")
	model.palette = true
	matches := model.matches()
	if len(matches) < 2 {
		t.Fatal("expected multiple registered commands")
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(interactiveModel)
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	if cmd != nil || model.palette || model.chat.Value() != matches[1].name+" " {
		t.Fatalf("selection: value=%q palette=%v command=%v", model.chat.Value(), model.palette, cmd)
	}
}

func TestPalettePrefersCommandNameOverDescription(t *testing.T) {
	for _, language := range []string{"en", "pt-BR"} {
		t.Run(language, func(t *testing.T) {
			i18n.SetLanguage(language)
			model := newInteractiveModel(&Session{}, t.TempDir(), true)
			updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/he")})
			model = updated.(interactiveModel)
			updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
			model = updated.(interactiveModel)
			if model.chat.Value() != "/help " {
				t.Fatalf("typing /he then Enter selected %q", model.chat.Value())
			}
		})
	}
}

func TestPaletteEnterExecutesExactCommand(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.chat.SetValue("/help")
	model.palette = true
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	if model.chat.Value() != "" || len(model.history) != 1 || model.history[0] != "/help" {
		t.Fatalf("exact command was not submitted: value=%q history=%v", model.chat.Value(), model.history)
	}
}

func TestCtrlCCancelsCurrentCommandAndSecondPressExits(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.busy = true
	canceled := false
	model.commandCancel = func() { canceled = true }
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model = updated.(interactiveModel)
	if !canceled || command != nil || model.status != "cancelado" {
		t.Fatalf("first Ctrl+C: canceled=%v command=%v status=%q", canceled, command, model.status)
	}
	updated, command = model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if _, ok := updated.(interactiveModel); !ok || command == nil {
		t.Fatal("second Ctrl+C within one second did not request exit")
	}
}

func TestInteractiveWelcomeAndEnterSends(t *testing.T) {
	i18n.SetLanguage("pt-BR")
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
	if command == nil || model.chat.Value() != "" || !model.busy {
		t.Fatalf("Enter did not send the message: model=%+v", model)
	}
}

func TestRecentSessionResultDoesNotReplaceManualSelection(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.selectedSession, model.selectedAgent = "manual", "codex"
	updated, _ := model.Update(recentSessionLoaded{project: model.project, id: "older", agent: "claude", history: "history"})
	model = updated.(interactiveModel)
	if model.selectedSession != "manual" || strings.Contains(strings.Join(model.lines, "\n"), "history") {
		t.Fatal("background load replaced the manual session")
	}
}

func TestCancelledRecentSessionNeverStartsProvider(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.selectedSession = "already-loaded"
	model.busy = true
	updated, cmd := model.Update(recentSessionLoaded{project: model.project, pending: "hello", err: context.Canceled})
	model = updated.(interactiveModel)
	if cmd != nil || model.busy {
		t.Fatal("cancelled selection started a provider")
	}
}

func TestRecentSessionCancellationAfterResultWasQueued(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	ctx, cancel := context.WithCancel(context.Background())
	model.recentContext, model.commandCancel, model.busy = ctx, cancel, true
	cancel()
	updated, cmd := model.Update(recentSessionLoaded{project: model.project, pending: "hello", id: "session", agent: "codex"})
	model = updated.(interactiveModel)
	if cmd != nil || model.busy || model.selectedSession != "" {
		t.Fatal("queued result started a cancelled request")
	}
}

func TestRecentSessionReportsHistoryError(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	updated, _ := model.Update(recentSessionLoaded{project: model.project, id: "session", agent: "codex", historyErr: errors.New("history unavailable")})
	model = updated.(interactiveModel)
	if !strings.Contains(strings.Join(model.lines, "\n"), "history unavailable") {
		t.Fatal("history failure was hidden")
	}
}

func TestInteractiveSelectionStateAndFailedSelection(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), true)
	model.submit("/use")
	if !strings.Contains(strings.Join(model.lines, "\n"), "Nenhuma sessão selecionada") {
		t.Fatal("missing empty selection guidance")
	}
	updated, _ := model.Update(sessionSelected{id: "codex-1234567890", agent: "Codex"})
	model = updated.(interactiveModel)
	if model.selectedSession != "codex-1234567890" || !strings.Contains(model.View(), "Codex") {
		t.Fatal("selection was not retained and displayed")
	}
	updated, _ = model.Update(sessionSelected{err: fmt.Errorf("sessão não encontrada")})
	model = updated.(interactiveModel)
	if model.selectedSession != "codex-1234567890" {
		t.Fatal("failed selection replaced the previous session")
	}
	model.submit("/use")
	if !strings.Contains(strings.Join(model.lines, "\n"), "Sessão selecionada: codex-12345678") {
		t.Fatal("/use did not show current selection")
	}
}

func TestCancelSessionSelectionDoesNotReplaceSelection(t *testing.T) {
	model := newInteractiveModel(&Session{}, t.TempDir(), false)
	model.selectedSession = "codex-original"
	command := model.startSessionSelection("missing")
	model.commandCancel()
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("selection must run asynchronously with a spinner")
	}
	result, ok := batch[1]().(sessionSelected)
	if !ok || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("selection cancellation = %#v", result)
	}
	updated, _ := model.Update(result)
	model = updated.(interactiveModel)
	if model.busy || model.selectedSession != "codex-original" {
		t.Fatal("canceled selection changed the active session or stayed busy")
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
			if got != normalizeInteractiveGolden(string(want)) {
				t.Fatalf("welcome view differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
			}
		})
	}
}

func normalizeInteractiveGolden(view string) string {
	view = strings.ReplaceAll(view, "\r\n", "\n")
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

func TestInteractiveAgentsStayCheckingUntilDiscoveryResult(t *testing.T) {
	model := newInteractiveModel(&Session{ProjectPath: "C:/workspace", DiscoverReady: func() int { return 3 }}, "C:/workspace", true)
	if !strings.Contains(model.welcomeCard(), i18n.T("tui.agents.checking")) {
		t.Fatal("discovery did not start in checking state")
	}
	updated, _ := model.Update(readyAgentsResult{count: 2})
	model = updated.(interactiveModel)
	if model.agentsChecking || !strings.Contains(model.welcomeCard(), "Agentes prontos: 2") {
		t.Fatal("completed discovery result was not shown")
	}
}

func TestAsyncDiscoveryShowsSpinnerAndReturnsResult(t *testing.T) {
	results := make(chan any, 2)
	session := &Session{Agents: func(w io.Writer) { _, _ = io.WriteString(w, "Codex available") }, sendMessage: func(message any) { results <- message }}
	model := newInteractiveModel(session, t.TempDir(), true)
	model.chat.SetValue("/agents")
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyCtrlS})
	model = updated.(interactiveModel)
	if command == nil || !model.busy {
		t.Fatal("diagnostic did not start asynchronously")
	}
	view := model.View()
	if !strings.Contains(view, i18n.T("tui.discovering")) {
		t.Fatal("busy status was not rendered")
	}
	first := <-results
	if _, ok := first.(handlerDone); !ok {
		t.Fatalf("first async event = %T, want completion", first)
	}
	updated, _ = model.Update(first.(tea.Msg))
	model = updated.(interactiveModel)
	if model.busy || !strings.Contains(strings.Join(model.lines, "\n"), "Codex available") {
		t.Fatal("diagnostic result did not update the conversation")
	}
}

func TestResponsiveLayoutAndNavigation(t *testing.T) {
	t.Setenv("TERM", "dumb")
	t.Setenv("NO_COLOR", "1")
	model := newInteractiveModel(&Session{}, "project", true)
	for _, size := range []struct{ w, h int }{{120, 36}, {80, 24}, {40, 15}} {
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})
		model = updated.(interactiveModel)
		view := model.View()
		if model.showSidebar() != (size.w >= 96 && size.h >= 18) {
			t.Fatalf("sidebar at %dx%d", size.w, size.h)
		}
		if n := len(strings.Split(view, "\n")); n > size.h {
			t.Errorf("%dx%d: %d rows", size.w, size.h, n)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > size.w {
				t.Errorf("%dx%d: line width %d", size.w, size.h, lipgloss.Width(line))
				break
			}
		}
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	model = updated.(interactiveModel)
	if !model.palette || model.chat.Value() != "/" {
		t.Fatal("Ctrl+K did not open the command palette")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(interactiveModel)
	if model.palette {
		t.Fatal("Esc did not close the palette")
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	model = updated.(interactiveModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	model = updated.(interactiveModel)
	if model.showSidebar() {
		t.Fatal("Ctrl+B did not hide sidebar")
	}
}

func TestProjectPickerChecksTrustAndClearsSession(t *testing.T) {
	store := trust.NewStore(t.TempDir())
	first, second := t.TempDir(), t.TempDir()
	if err := store.Grant(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(second); err != nil {
		t.Fatal(err)
	}
	session := &Session{TrustStore: store, ProjectPath: first}
	model := newInteractiveModel(session, first, true)
	model.sidebar.Projects = []sidebarProject{{Path: second}}
	model.selectedSession, model.selectedAgent = "old-session", "Codex"
	model.lines = []string{"old project content"}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(interactiveModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(interactiveModel)
	canonical, err := trust.Canonical(second)
	if err != nil {
		t.Fatal(err)
	}
	if model.project != canonical || session.ProjectPath != canonical || model.selectedSession != "" || len(model.lines) != 0 {
		t.Fatal("project switch retained previous project state")
	}
	model.busy = true
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(interactiveModel)
	if model.projectPicker {
		t.Fatal("picker opened while execution is active")
	}
}

func TestStreamingChunksAndThemeSurviveResize(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	model := newInteractiveModel(&Session{}, "project", true)
	updated, _ := model.Update(handlerChunk{text: "```go\n", stream: true})
	model = updated.(interactiveModel)
	updated, _ = model.Update(handlerChunk{text: "func main() {}\n```", stream: true})
	model = updated.(interactiveModel)
	if len(model.transcript) != 1 || !strings.Contains(model.transcript[0], "func main()") {
		t.Fatal("stream chunks not grouped")
	}
	updated, _ = model.Update(handlerChunk{text: "first event"})
	model = updated.(interactiveModel)
	updated, _ = model.Update(handlerChunk{text: "second event"})
	model = updated.(interactiveModel)
	if len(model.transcript) != 3 {
		t.Fatal("independent events were combined")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyCtrlT})
	model = updated.(interactiveModel)
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(interactiveModel)
	if !model.lightTheme || model.renderWidth != model.view.Width {
		t.Fatal("theme or render cache lost on resize")
	}
}

func TestProjectPickerPreservesDraft(t *testing.T) {
	model := newInteractiveModel(&Session{}, "project", true)
	model.chat.SetValue("draft")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	model = updated.(interactiveModel)
	if model.projectPicker || model.chat.Value() != "draft" {
		t.Fatal("project picker must preserve draft")
	}
}

func TestSmallApprovalDialogFitsAndScrolls(t *testing.T) {
	t.Setenv("TERM", "dumb")
	model := newInteractiveModel(&Session{}, "project", true)
	model.width, model.height = 30, 12
	model.approval = &approvalPrompt{request: PermissionRequest{Agent: "Codex", Target: strings.Repeat("long target ", 12), ExactCommand: "important-command"}}
	view := model.View()
	if len(strings.Split(view, "\n")) > 12 {
		t.Fatal("dialog exceeds terminal height")
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatal("dialog exceeds terminal width")
		}
	}
	for i := 0; i < 40; i++ {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyPgDown})
		model = updated.(interactiveModel)
	}
	if !strings.Contains(model.View(), "important-command") {
		t.Fatal("approval details cannot be reached by scrolling")
	}
}

func TestSmallTrustDeniedDialogFits(t *testing.T) {
	t.Setenv("TERM", "dumb")
	model := newInteractiveModel(&Session{}, "project", false)
	model.width, model.height, model.trustDenied = 30, 12, true
	view := model.View()
	if len(strings.Split(view, "\n")) > 12 {
		t.Fatal("trust dialog exceeds terminal height")
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > 30 {
			t.Fatal("trust dialog exceeds terminal width")
		}
	}
}

func TestFooterShowsActiveBackgroundRun(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	model := newInteractiveModel(&Session{}, "project-background-test", true)
	activeRuns.Store("footer-test", activeRun{project: model.project, cancel: func() {}})
	defer activeRuns.Delete("footer-test")
	if !strings.Contains(model.View(), "Execu\u00e7\u00e3o ativa") {
		t.Fatal("active background run is not shown")
	}
}

func TestConfigurableNavigationKeys(t *testing.T) {
	keys, err := (config.TUIKeys{Projects: "f1", Palette: "f2"}).Resolved()
	if err != nil {
		t.Fatal(err)
	}
	model := newInteractiveModel(&Session{}, "project", true)
	model.keys = keys
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyF1})
	model = updated.(interactiveModel)
	if !model.projectPicker {
		t.Fatal("configured project key did not open picker")
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = updated.(interactiveModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyF2})
	model = updated.(interactiveModel)
	if !model.palette {
		t.Fatal("configured palette key did not open palette")
	}
	model.trusted = false
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyF1})
	model = updated.(interactiveModel)
	if model.projectPicker {
		t.Fatal("navigation key bypassed trust prompt")
	}
}

func TestPalettePreservesDraft(t *testing.T) {
	model := newInteractiveModel(&Session{}, "project", true)
	model.chat.SetValue("unsent message")
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyCtrlK})
	model = updated.(interactiveModel)
	if model.chat.Value() != "unsent message" || model.palette {
		t.Fatal("palette must preserve the draft")
	}
}
