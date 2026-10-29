package onboarding

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestWizardScreenViews(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	model := wizardModel{
		options:  WizardOptions{ctx: context.Background(), Home: t.TempDir(), Project: filepath.Join(t.TempDir(), "project")},
		missing:  []Item{{ID: "cursor", Name: "Cursor CLI", URL: "https://example.com", Methods: map[string][]Method{"windows": {manual("manual", "install manually", "https://example.com")}}}},
		selected: map[string]bool{"cursor": true},
		language: 1,
		cursor:   1,
		results:  []discovery.Result{{Name: "Codex", Version: "1.2.3", Status: discovery.Ready, Auth: discovery.AuthReady, Path: "C:/tools/codex.exe"}},
		width:    100,
	}
	cases := []struct {
		step  int
		wants []string
	}{
		{0, []string{"Passo 1 de 8", "Idioma do terminal", "Português (Brasil)"}},
		{1, []string{"Passo 2 de 8", "Agentes e ferramentas locais", "Codex", "1.2.3", "READY"}},
		{2, []string{"Passo 3 de 8", "Instalar ferramentas ausentes", "Cursor CLI", "Comando:"}},
		{3, []string{"Passo 4 de 8", "Sim, confiar nesta pasta", "Não, manter sem confiança"}},
		{4, []string{"Passo 5 de 8", "Perfil de permissões", "restricted"}},
		{5, []string{"Passo 6 de 8", "Modo de inicialização do dashboard"}},
		{6, []string{"Passo 7 de 8", "Community", "ativação depois"}},
		{7, []string{"Passo 8 de 8", "Resumo da configuração", "Revise suas escolhas", "Instalações confirmadas: Nenhuma"}},
	}
	for _, tc := range cases {
		t.Run(tc.wants[0], func(t *testing.T) {
			model.step = tc.step
			model.cursor = model.selectedCursor()
			for _, width := range []int{80, 120} {
				model.width = width
				view := model.View()
				for _, want := range tc.wants {
					if !strings.Contains(view, want) {
						t.Errorf("step %d width %d missing %q: %s", tc.step, width, want, view)
					}
				}
			}
		})
	}
}

func TestWizardGoldenTerminalSizes(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "dumb")
	for _, size := range []struct{ width, height int }{{80, 24}, {120, 36}} {
		for _, screen := range []struct {
			name string
			step int
		}{{"welcome", -1}, {"language", 0}, {"agents", 1}, {"installs", 2}, {"trust", 3}, {"permissions", 4}, {"dashboard", 5}, {"edition", 6}, {"summary", 7}} {
			name := fmt.Sprintf("%s-%dx%d.golden", screen.name, size.width, size.height)
			t.Run(name, func(t *testing.T) {
				model := wizardModel{
					options:  WizardOptions{ctx: context.Background(), Project: "C:/workspace", Home: t.TempDir()},
					step:     screen.step,
					width:    size.width,
					height:   size.height,
					language: 1,
					cursor:   1,
					missing:  []Item{{ID: "cursor", Name: "Cursor CLI", URL: "https://example.com", Methods: map[string][]Method{"windows": {manual("manual", "install manually", "https://example.com")}}}},
					results:  []discovery.Result{{Name: "Codex", Version: "1.2.3", Status: discovery.Ready, Auth: discovery.AuthReady, Path: "C:/tools/codex.exe"}},
				}
				got := normalizeWizardGolden(model.View())
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
				if got != normalizeWizardGolden(string(want)) {
					t.Fatalf("wizard %s view differs from %s\n--- got ---\n%s\n--- want ---\n%s", screen.name, path, got, want)
				}
			})
		}
	}
}

func normalizeWizardGolden(view string) string {
	view = strings.ReplaceAll(view, "\r\n", "\n")
	lines := strings.Split(view, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}

func TestWizardWelcomeAndBackNavigation(t *testing.T) {
	model := wizardModel{options: WizardOptions{Project: "C:/workspace"}, step: -1, width: 80, language: 1}
	if view := model.View(); !strings.Contains(view, "Bem-vindo ao GasaCLI") || !strings.Contains(view, "Enter inicia a configuração") {
		t.Fatalf("welcome view is incomplete: %s", view)
	}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(wizardModel)
	if model.step != 0 {
		t.Fatalf("welcome Enter advanced to step %d, want 0", model.step)
	}
	model.step = 3
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})
	if got := updated.(wizardModel).step; got != 2 {
		t.Fatalf("b should go back in the wizard: step=%d", got)
	}
}

func TestWizardShowsCheckingRowsBeforeDiscoveryCompletes(t *testing.T) {
	called := false
	model := wizardModel{
		options: WizardOptions{Discover: func(context.Context) []discovery.Result {
			called = true
			return []discovery.Result{{ID: "claude", Name: "Claude Code", Status: discovery.Ready}}
		}},
		step: 1, language: 1, width: 80, discovering: true,
	}
	if got := model.View(); !strings.Contains(got, "Claude Code") || !strings.Contains(got, "VERIFICANDO") {
		t.Fatalf("initial agent table should show checking rows: %s", got)
	}
	if called {
		t.Fatal("discovery ran before the wizard could render")
	}
	updated, _ := model.Update(model.Init()())
	result := updated.(wizardModel)
	if result.discovering || len(result.results) != 1 || result.results[0].Status != discovery.Ready {
		t.Fatalf("discovery result was not applied: %+v", result)
	}
}

func TestWizardAgentRowsUseBoundedStatusesAndPaths(t *testing.T) {
	model := wizardModel{step: 1, width: 80, language: 1, results: []discovery.Result{
		{Name: "A very long agent name which must be shortened", Status: discovery.Ready, Auth: discovery.AuthReady, Version: "version output that is much too long", Path: "C:/tools/a/very/long/path/agent.exe"},
		{Name: "missing", Status: discovery.NotFound},
		{Name: "installed", Status: discovery.VersionUnknown, Path: "C:/tools/installed.exe"},
		{Name: "unknown", Status: discovery.Error},
	}}
	view := model.View()
	for _, want := range []string{"READY", "AUSENTE", "INSTALADO", "DESCONHECIDO", "…"} {
		if !strings.Contains(view, want) {
			t.Errorf("agent table missing %q: %s", want, view)
		}
	}
}

func TestWizardAgentRowsUseAuthReadinessVerdicts(t *testing.T) {
	model := wizardModel{step: 1, width: 80, language: 1, results: []discovery.Result{
		{Name: "ready", Status: discovery.Ready, Auth: discovery.AuthReady},
		{Name: "login", Status: discovery.NeedsLogin, Auth: discovery.AuthNeedsLogin},
		{Name: "installed", Status: discovery.Installed, Auth: discovery.AuthUnknown},
		{Name: "missing", Status: discovery.Missing},
	}}
	view := model.View()
	for _, want := range []string{"[READY]", "[LOGIN NECESSÁRIO]", "[INSTALADO]", "[AUSENTE]"} {
		if !strings.Contains(view, want) {
			t.Errorf("agent table missing %q: %s", want, view)
		}
	}
}

func TestWizardInstallRequiresExplicitConfirmation(t *testing.T) {
	model := wizardModel{options: WizardOptions{Home: t.TempDir()}, step: 2, missing: []Item{{ID: "cursor"}}, selected: map[string]bool{"cursor": true}}
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(wizardModel)
	if !model.confirmInstall || model.installConfirmed || model.step != 2 {
		t.Fatalf("selection advanced without explicit confirmation: %+v", model)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	model = updated.(wizardModel)
	if !model.confirmInstall || model.installConfirmed || model.step != 2 {
		t.Fatalf("unexpected letter changed install confirmation: %+v", model)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(wizardModel)
	if !model.installConfirmed || model.step != 3 {
		t.Fatalf("confirmed plan did not advance: %+v", model)
	}
}

func TestWizardProgressPersistsAndResumes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	if err := config.Ensure(); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	model := wizardModel{options: WizardOptions{Home: home, Project: project}, state: config.OnboardingState{}, step: 0}
	model.advance()
	state, err := config.LoadState(home)
	if err != nil {
		t.Fatal(err)
	}
	if state.Onboarding.Version != 2 || state.Onboarding.CurrentStep != 1 || state.Onboarding.Permissions != "restricted" {
		t.Fatalf("progress=%+v", state.Onboarding)
	}
}

func TestSaveWebPreference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	if err := config.Ensure(); err != nil {
		t.Fatal(err)
	}
	if err := config.SaveWebMode(home, "manual"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil || cfg.Web.Mode != "manual" {
		t.Fatalf("web mode=%q err=%v", cfg.Web.Mode, err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestWizardSummaryEnterQuitsAndRecordsCompletion(t *testing.T) {
	home := t.TempDir()
	model := wizardModel{options: WizardOptions{Home: home}, state: config.OnboardingState{}, step: 7}
	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(wizardModel)
	if !model.completed {
		t.Fatal("wizard must be completed after Enter on the summary")
	}
	if cmd == nil {
		t.Fatal("completing the wizard must return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("completing the wizard must quit the program, got %T", cmd())
	}
	state, err := config.LoadState(home)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Onboarding.Completed || state.Onboarding.CompletedAt.IsZero() {
		t.Fatalf("completion not recorded: %+v", state.Onboarding)
	}
}

func TestWizardKeyMatrix(t *testing.T) {
	keys := []string{"enter", "esc", "s", "b", "backspace", "up", "down", "k", "j", " ", "a", "A", "left", "right", "z", "?", "ctrl+c"}
	for step := 0; step <= 7; step++ {
		for _, key := range keys {
			t.Run(fmt.Sprintf("step_%d/%q", step+1, key), func(t *testing.T) {
				model := wizardModel{
					options: WizardOptions{Home: t.TempDir(), Out: io.Discard, ctx: context.Background()},
					step:    step, cursor: 0, language: 1, permission: 1, dashboard: 0, edition: 0,
					missing: []Item{{ID: "one"}, {ID: "two"}}, selected: map[string]bool{"one": false, "two": false},
				}
				beforeStep := model.step
				beforeLanguage, beforePermission, beforeDashboard, beforeEdition := model.language, model.permission, model.dashboard, model.edition
				updated, cmd := model.Update(wizardKey(key))
				got := updated.(wizardModel)
				switch key {
				case "esc", "ctrl+c":
					if cmd == nil || !got.savedAndExit {
						t.Fatal("exit key did not save and quit")
					}
				case "enter":
					wantStep := min(7, beforeStep+1)
					if beforeStep == 7 {
						if !got.completed || cmd == nil {
							t.Fatal("Enter did not finish summary")
						}
					} else if beforeStep == 2 && got.confirmInstall {
						if got.step != 2 {
							t.Fatalf("install preview moved to step %d", got.step)
						}
					} else if got.step != wantStep {
						t.Fatalf("Enter moved to step %d, want %d", got.step, wantStep)
					}
				case "s":
					if beforeStep < 7 && got.step != beforeStep+1 {
						t.Fatalf("S moved to %d, want %d", got.step, beforeStep+1)
					}
				case "b", "backspace":
					if got.step != max(0, beforeStep-1) {
						t.Fatalf("back moved to %d", got.step)
					}
				case "?":
					if !got.showHelp {
						t.Fatal("? did not show help")
					}
				case "left", "right", "z":
					if got.step != beforeStep || got.language != beforeLanguage || got.permission != beforePermission || got.dashboard != beforeDashboard || got.edition != beforeEdition {
						t.Fatalf("%q unexpectedly changed a value: %+v", key, got)
					}
				case "a", "A":
					if beforeStep == 2 && (!got.selected["one"] || !got.selected["two"]) {
						t.Fatal("A did not select all install tools")
					}
				case "up", "down", "k", "j":
					if got.step != beforeStep {
						t.Fatalf("%q changed the step", key)
					}
					if (key == "down" || key == "j") && got.choiceCount() > 1 && got.cursor != 1 {
						t.Fatalf("%q did not move the highlight", key)
					}
				case " ":
					if got.step != beforeStep {
						t.Fatal("Space changed the step")
					}
				}
			})
		}
	}
}

func wizardKey(key string) tea.KeyMsg {
	switch key {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
}

func TestWizardTrueColorUsesBrandHighlightTokens(t *testing.T) {
	previousProfile, previousBackground := lipgloss.ColorProfile(), lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	t.Cleanup(func() {
		lipgloss.SetColorProfile(previousProfile)
		lipgloss.SetHasDarkBackground(previousBackground)
	})
	t.Setenv("NO_COLOR", "")
	model := wizardModel{step: 0, language: 0, cursor: 0, width: 80}
	view := model.View()
	for _, token := range []string{"48;2;30;207;165", "38;2;23;42;70"} {
		if !strings.Contains(view, token) {
			t.Fatalf("truecolor highlight does not include brand token %q: %q", token, view)
		}
	}
}
