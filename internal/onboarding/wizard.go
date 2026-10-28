package onboarding

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/trust"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type WizardOptions struct {
	Home, Project string
	Out           io.Writer
	Resume        bool
	ctx           context.Context
	Discover      func(context.Context) []discovery.Result
	GrantTrust    func(string) error
}

type wizardModel struct {
	options                                                       WizardOptions
	state                                                         config.OnboardingState
	results                                                       []discovery.Result
	missing                                                       []Item
	selected                                                      map[string]bool
	step, cursor, language, permission, dashboard, edition, width int
	trustFolder, confirmInstall, installConfirmed, completed      bool
	err                                                           error
}

var permissionProfiles = []string{"restricted", "balanced", "autonomous"}
var dashboardModes = []string{"auto", "manual", "always"}
var editions = []string{"Community", "Pro (activate later)"}
var languages = []string{"English", "Portugues"}

func RunWizard(ctx context.Context, options WizardOptions) error {
	options.ctx = ctx
	if options.Out == nil {
		options.Out = os.Stdout
	}
	if options.Home == "" {
		home, err := config.Home()
		if err != nil {
			return err
		}
		options.Home = home
	}
	if options.Project == "" {
		project, err := os.Getwd()
		if err != nil {
			return err
		}
		options.Project = project
	}
	project, err := trust.Canonical(options.Project)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	options.Project = project
	if options.Discover == nil {
		options.Discover = func(ctx context.Context) []discovery.Result {
			return discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
		}
	}
	saved, err := config.LoadState(options.Home)
	if err != nil {
		return fmt.Errorf("load onboarding state: %w", err)
	}
	results := options.Discover(ctx)
	missing := missingItems(results)
	selected := map[string]bool{}
	for _, item := range missing {
		selected[item.ID] = item.ID == "claude" || item.ID == "codex"
	}
	model := wizardModel{options: options, state: saved.Onboarding, results: results, missing: missing, selected: selected}
	if options.Resume && (!saved.Onboarding.Completed || saved.Onboarding.Version < 2) {
		model.step = max(0, min(5, saved.Onboarding.CurrentStep))
		model.state.Completed = false
	}
	for i, value := range permissionProfiles {
		if value == saved.Onboarding.Permissions {
			model.permission = i
		}
	}
	for i, value := range languages {
		if value == saved.Onboarding.Language {
			model.language = i
		}
	}
	for i, value := range dashboardModes {
		if value == saved.Onboarding.Dashboard {
			model.dashboard = i
		}
	}
	for i, value := range editions {
		if value == saved.Onboarding.Edition {
			model.edition = i
		}
	}
	program := tea.NewProgram(model, tea.WithInput(os.Stdin), tea.WithOutput(options.Out), tea.WithAltScreen(), tea.WithContext(ctx))
	final, err := program.Run()
	if err != nil {
		return fmt.Errorf("run onboarding wizard: %w", err)
	}
	result, ok := final.(wizardModel)
	if !ok || !result.completed {
		return nil
	}
	if result.trustFolder && options.GrantTrust != nil {
		if err := options.GrantTrust(options.Project); err != nil {
			return fmt.Errorf("grant project trust: %w", err)
		}
	}
	if err := config.SaveWebMode(options.Home, dashboardModes[result.dashboard]); err != nil {
		return fmt.Errorf("save dashboard preference: %w", err)
	}
	if result.installConfirmed {
		var numbers []string
		for index, item := range result.missing {
			if result.selected[item.ID] {
				numbers = append(numbers, fmt.Sprint(index+1))
			}
		}
		input := strings.NewReader(strings.Join(numbers, " ") + "\ny\n")
		if err := Run(ctx, Options{In: input, Out: options.Out, Interactive: true, Numbered: true}); err != nil {
			return fmt.Errorf("run confirmed installation plan: %w", err)
		}
	}
	return nil
}

func (m wizardModel) Init() tea.Cmd { return nil }

func (m wizardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.KeyMsg:
		if m.step == 2 && m.confirmInstall {
			switch msg.String() {
			case "y", "Y":
				m.installConfirmed, m.confirmInstall = true, false
				m.advance()
			case "n", "N", "esc":
				m.confirmInstall = false
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			m.persist()
			return m, tea.Quit
		case "left", "h", "b", "esc":
			if m.step == 0 && msg.String() == "left" {
				m.language = (m.language + len(languages) - 1) % len(languages)
			} else if m.step == 4 && msg.String() == "left" {
				m.permission = (m.permission + len(permissionProfiles) - 1) % len(permissionProfiles)
			} else if m.step > 0 {
				m.step--
				m.persist()
			}
		case "right", "enter":
			if m.step == 0 && msg.String() == "right" {
				m.language = (m.language + 1) % len(languages)
			} else if m.step == 4 && msg.String() == "right" {
				m.permission = (m.permission + 1) % len(permissionProfiles)
			} else if m.step == 2 && anySelected(m.selected) {
				m.confirmInstall = true
			} else if m.step < 6 {
				m.advance()
			} else {
				m.complete()
			}
		case "s":
			if m.step < 6 {
				m.advance()
			} else {
				m.complete()
			}
		case "up", "k":
			if m.step == 2 && m.cursor > 0 {
				m.cursor--
			}
			if m.step == 5 && m.dashboard > 0 {
				m.dashboard--
			}
		case "down", "j":
			if m.step == 2 && m.cursor < len(m.missing)-1 {
				m.cursor++
			}
			if m.step == 5 && m.dashboard < len(dashboardModes)-1 {
				m.dashboard++
			}
		case " ":
			if m.step == 2 && len(m.missing) > 0 {
				item := m.missing[m.cursor]
				m.selected[item.ID] = !m.selected[item.ID]
			}
		case "y", "Y":
			if m.step == 3 {
				m.trustFolder = true
			}
		case "n", "N":
			if m.step == 3 {
				m.trustFolder = false
			}
		case "tab":
			if m.step == 5 {
				m.edition = (m.edition + 1) % len(editions)
			}
		}
	}
	return m, nil
}

func (m *wizardModel) advance() {
	if m.step < 6 {
		m.step++
	}
	m.persist()
}

func (m *wizardModel) persist() {
	m.state.CurrentStep = m.step
	m.state.Permissions = permissionProfiles[m.permission]
	m.state.Dashboard = dashboardModes[m.dashboard]
	m.state.Edition = editions[m.edition]
	m.state.Language = languages[m.language]
	if err := config.SaveOnboardingProgress(m.options.Home, m.state); err != nil {
		m.err = err
	}
}

func (m *wizardModel) complete() {
	m.persist()
	if m.err != nil {
		return
	}
	m.state.Completed = true
	m.state.CurrentStep = 0
	if err := config.SaveOnboardingProgress(m.options.Home, m.state); err != nil {
		m.err = err
		return
	}
	m.completed = true
}

func (m wizardModel) View() string {
	accent := lipgloss.AdaptiveColor{Light: "#16866d", Dark: "#1ECFA5"}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accent).Padding(1, 2).Width(max(44, min(96, m.width-4)))
	var body strings.Builder
	if m.step < 6 {
		fmt.Fprintf(&body, "Step %d of 6\n\n", m.step+1)
	} else {
		body.WriteString("Setup summary\n\n")
	}
	switch m.step {
	case 0:
		fmt.Fprintf(&body, "Welcome to GasaCLI\n\nA local-first terminal for discovering coding agents and coordinating work. Community works without an account.\n\nLanguage: %s (Left/Right to change)\n", languages[m.language])
	case 1:
		body.WriteString("Detected agents\n\n")
		for _, result := range m.results {
			fmt.Fprintf(&body, "%-18s %-12s %-18s %s\n", result.Name, result.Version, result.Status, result.Path)
		}
	case 2:
		body.WriteString("Install missing tools\nSelect with Space. Auto and manual methods are shown. Nothing runs without explicit confirmation.\n\n")
		for index, item := range m.missing {
			mark, cursor := "[ ]", "  "
			if m.selected[item.ID] {
				mark = "[x]"
			}
			if index == m.cursor {
				cursor = "> "
			}
			method, _ := chooseMethod(m.options.ctx, item, normalizeOS(runtime.GOOS), execLookPath, runCommand)
			kind := "manual"
			if method.Auto {
				kind = "auto"
			}
			fmt.Fprintf(&body, "%s%s %s (%s)\n", cursor, mark, item.Name, kind)
			if index == m.cursor && m.selected[item.ID] {
				if item.Note != "" {
					fmt.Fprintf(&body, "    %s\n", item.Note)
				}
				fmt.Fprintf(&body, "    Command: %s\n    Official: %s\n", commandText(method), item.URL)
			}
		}
		if m.confirmInstall {
			body.WriteString("\nConfirm this exact plan? Press y to accept; n to skip installs.\n")
		}
	case 3:
		body.WriteString("Trust this folder?\n\nTrust allows discovery and project access when requested. It does not approve commands or installs.\n\n")
		if m.trustFolder {
			body.WriteString("> Yes, trust " + m.options.Project)
		} else {
			body.WriteString("  Yes, trust " + m.options.Project)
		}
		body.WriteString("\n")
		if !m.trustFolder {
			body.WriteString("> ")
		} else {
			body.WriteString("  ")
		}
		body.WriteString("Keep untrusted (default). Press y or n to choose.")
	case 4:
		fmt.Fprintf(&body, "Permission profile (Left/Right to change: %s)\n\n", permissionProfiles[m.permission])
		for i, name := range permissionProfiles {
			mark := "[ ]"
			if i == m.permission {
				mark = "[x]"
			}
			description := []string{"Ask before writes, commands, installs, and network access.", "Allow routine work; ask before risky commands and external access.", "Allow routine actions; sensitive operations still require approval."}[i]
			fmt.Fprintf(&body, "%s %s - %s\n", mark, name, description)
		}
	case 5:
		fmt.Fprintf(&body, "Dashboard mode: %s\nEdition: %s\n\nUp/Down changes dashboard mode; Tab changes edition.\n", dashboardModes[m.dashboard], editions[m.edition])
	case 6:
		installPlan := "none (not confirmed)"
		if m.installConfirmed {
			installPlan = selectedNames(m.missing, m.selected)
		}
		fmt.Fprintf(&body, "Review your choices\n\nPermission profile: %s\nDashboard: %s\nEdition: %s\nTrust folder: %t\nConfirmed installs: %s\n\nEnter starts GasaCLI.", permissionProfiles[m.permission], dashboardModes[m.dashboard], editions[m.edition], m.trustFolder, installPlan)
	}
	if m.err != nil {
		body.WriteString("\n\nCould not save setup progress: " + m.err.Error())
	}
	footer := "Left: Back | Right/Enter: Next | S: Skip"
	title := lipgloss.NewStyle().Bold(true).Foreground(accent).Render("G> GasaCLI setup")
	return box.Render(title + "\n\n" + body.String() + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94a3b8"}).Render(footer))
}

func execLookPath(name string) (string, error) { return exec.LookPath(name) }
func anySelected(values map[string]bool) bool {
	for _, value := range values {
		if value {
			return true
		}
	}
	return false
}
func selectedNames(items []Item, selected map[string]bool) string {
	var names []string
	for _, item := range items {
		if selected[item.ID] {
			names = append(names, item.Name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
