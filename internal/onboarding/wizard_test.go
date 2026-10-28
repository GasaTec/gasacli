package onboarding

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	tea "github.com/charmbracelet/bubbletea"
)

func TestWizardScreenViews(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	model := wizardModel{
		options:  WizardOptions{ctx: context.Background(), Home: t.TempDir(), Project: filepath.Join(t.TempDir(), "project")},
		missing:  []Item{{ID: "cursor", Name: "Cursor CLI", URL: "https://example.com", Methods: map[string][]Method{"windows": {manual("manual", "install manually", "https://example.com")}}}},
		selected: map[string]bool{"cursor": true},
		results:  []discovery.Result{{Name: "Codex", Version: "1.2.3", Status: discovery.Available, Path: "C:/tools/codex.exe"}},
		width:    100,
	}
	cases := []struct {
		step  int
		wants []string
	}{
		{0, []string{"Step 1 of 6", "Welcome to GasaCLI", "Language:"}},
		{1, []string{"Step 2 of 6", "Detected agents", "Codex", "1.2.3"}},
		{2, []string{"Step 3 of 6", "Install missing tools", "Cursor CLI", "Command:"}},
		{3, []string{"Step 4 of 6", "Trust this folder?", "Keep untrusted"}},
		{4, []string{"Step 5 of 6", "Permission profile", "restricted"}},
		{5, []string{"Step 6 of 6", "Dashboard mode:", "Community"}},
		{6, []string{"Setup summary", "Review your choices", "Confirmed installs: none"}},
	}
	for _, tc := range cases {
		t.Run(tc.wants[0], func(t *testing.T) {
			model.step = tc.step
			view := model.View()
			for _, want := range tc.wants {
				if !strings.Contains(view, want) {
					t.Errorf("step %d missing %q: %s", tc.step, want, view)
				}
			}
		})
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
	if model.confirmInstall || model.installConfirmed || model.step != 2 {
		t.Fatalf("declined install plan was not retained: %+v", model)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(wizardModel)
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
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
