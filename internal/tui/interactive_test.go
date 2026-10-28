package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/i18n"
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
		{Name: "/goal", Summary: "command.goal", Planned: true, IssueRef: "#11"},
		{Name: "/help", Summary: "command.help"},
		{Name: "/permissions", Summary: "command.permissions", Planned: true, IssueRef: "#31"},
		{Name: "/prompts", Summary: "command.prompts"},
		{Name: "/quit", Summary: "command.exit"},
		{Name: "/restore", Summary: "command.restore"},
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
	if command != nil || model.chat.Value() != "" || !strings.Contains(strings.Join(model.lines, "\n"), "chat com modelo ainda não está conectado") {
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
