package onboarding

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

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
	options                                                               WizardOptions
	state                                                                 config.OnboardingState
	results                                                               []discovery.Result
	missing                                                               []Item
	selected                                                              map[string]bool
	step, cursor, language, permission, dashboard, edition, width, height int
	trustFolder, confirmInstall, installConfirmed, completed              bool
	err                                                                   error
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
	if saved.Onboarding.Language == "" {
		model.language = 1
	}
	if !options.Resume {
		model.step = -1
	}
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
	if !result.trustFolder {
		fmt.Fprintln(options.Out, "\nConfiguração encerrada\nEsta pasta continua sem confiança; nenhum arquivo do projeto foi acessado.\nPara revisar depois, execute `gasa setup`.")
		return fmt.Errorf("project not trusted; review this choice with `gasa setup`")
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
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if m.step == -1 {
			switch msg.String() {
			case "enter":
				m.step = 0
				m.persist()
			case "esc", "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}
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
		// Wizard screens contain selections but no free-text fields; h/b are navigation aliases here.
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
	if m.completed {
		return m, tea.Quit
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
	m.state.CompletedAt = time.Now().UTC()
	if err := config.SaveOnboardingProgress(m.options.Home, m.state); err != nil {
		m.err = err
		return
	}
	m.completed = true
}

func (m wizardModel) View() string {
	accent := lipgloss.AdaptiveColor{Light: "#087E68", Dark: "#1ECFA5"}
	box := lipgloss.NewStyle().Border(lipgloss.Border{Top: "─", Bottom: "─", Left: "│", Right: "│", TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘"}).BorderForeground(accent).Foreground(lipgloss.AdaptiveColor{Light: "#172A46", Dark: "#F4F7FB"}).Padding(1, 2).Width(max(44, min(96, m.width-4)))
	var body strings.Builder
	if m.step == -1 {
		body.WriteString("Bem-vindo ao GasaCLI\n\n")
		fmt.Fprintf(&body, "Projeto: %s\nConfiança: pendente de confirmação\n\n", m.options.Project)
		body.WriteString("Configure as opções locais e revise as ferramentas antes de continuar.\n\nEnter inicia a configuração. Esc sai sem alterar o projeto.")
	} else if m.step == 3 {
		body.WriteString("Confirmação de workspace\n\n")
	} else if m.step < 6 {
		visibleStep := m.step + 1
		if m.step > 3 {
			visibleStep--
		}
		fmt.Fprintf(&body, "Passo %d de 6\n\n", visibleStep)
	} else {
		fmt.Fprintf(&body, "Passo 6 de 6\n\n")
		body.WriteString("Resumo da configuração\n\n")
	}
	switch m.step {
	case 0:
		fmt.Fprintf(&body, "Idioma do terminal\n\nPortuguês (Brasil) é recomendado. A saída padrão pode ser alterada depois.\n\nIdioma: %s (←/→ para alterar)\n", map[bool]string{true: "Português (Brasil)", false: "English (US)"}[m.language == 1])
	case 1:
		body.WriteString("Agentes e ferramentas locais\n\nNOME                 VERSÃO          STATUS\n")
		for _, result := range m.results {
			name := truncateColumns(result.Name, 20)
			version := result.Version
			if version == "" {
				version = "N/D"
			}
			version = truncateColumns(version, 15)
			fmt.Fprintf(&body, "%-20s %-15s [%s]\n", name, version, readinessLabel(result.Status))
			if result.Path != "" {
				pathWidth := max(12, min(68, m.width-12))
				fmt.Fprintf(&body, "  %s\n", lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#5B6B80", Dark: "#9AAAC0"}).Render(truncateColumns(result.Path, pathWidth)))
			}
		}
	case 2:
		body.WriteString("Instalar ferramentas ausentes\nUse Espaço para selecionar. Nada será instalado sem confirmação explícita.\n\n")
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
			body.WriteString("\nConfirmar este plano exato? Pressione y para aceitar ou n para pular.\n")
		}
	case 3:
		body.WriteString("Confirmação de pasta confiável\n\nA confiança permite descobrir ferramentas e acessar arquivos do projeto quando solicitado. Ela não aprova comandos ou instalações.\n\n")
		if m.trustFolder {
			body.WriteString("> Sim, confiar em " + m.options.Project)
		} else {
			body.WriteString("  Sim, confiar em " + m.options.Project)
		}
		body.WriteString("\n")
		if !m.trustFolder {
			body.WriteString("> ")
		} else {
			body.WriteString("  ")
		}
		body.WriteString("Manter sem confiança (padrão). Pressione y ou n para escolher.")
	case 4:
		fmt.Fprintf(&body, "Perfil de permissões (←/→ para alterar: %s)\n\n", permissionProfiles[m.permission])
		for i, name := range permissionProfiles {
			mark := "[ ]"
			if i == m.permission {
				mark = "[x]"
			}
			description := []string{"Pergunta antes de gravar, executar comandos, instalar ou acessar a rede.", "Permite ações comuns; pergunta antes de ações arriscadas ou externas.", "Permite ações comuns; operações sensíveis ainda exigem aprovação."}[i]
			fmt.Fprintf(&body, "%s %s - %s\n", mark, name, description)
		}
	case 5:
		fmt.Fprintf(&body, "Modo do dashboard: %s\nEdição: %s\n\n↑/↓ altera o modo do dashboard; Tab altera a edição.\n", dashboardModes[m.dashboard], editionLabel(m.edition))
	case 6:
		installPlan := "Nenhuma (não confirmada)"
		if m.installConfirmed {
			installPlan = selectedNames(m.missing, m.selected)
			if installPlan == "none" {
				installPlan = "Nenhuma"
			}
		}
		trustStatus := "Não confiável"
		if m.trustFolder {
			trustStatus = "Confiável"
		}
		fmt.Fprintf(&body, "Revise suas escolhas\n\nPerfil de permissões: %s\nDashboard: %s\nEdição: %s\nConfiança do workspace: %s\nInstalações confirmadas: %s\n\nEnter inicia o GasaCLI.", permissionProfiles[m.permission], dashboardModes[m.dashboard], editionLabel(m.edition), trustStatus, installPlan)
	}
	if m.err != nil {
		body.WriteString("\n\nNão foi possível salvar o progresso da configuração: " + m.err.Error())
	}
	footer := "Enter: Iniciar configuração | Esc: Sair"
	if m.step >= 0 {
		footer = "Enter: Avançar | S: Pular"
	}
	if m.step > 0 && m.step < 6 {
		footer = "←: Voltar | Enter: Avançar | S: Pular"
	}
	if m.step == 6 {
		footer = "Enter: Concluir | Esc: Voltar"
	}
	mark := "╭G›╮"
	if os.Getenv("TERM") == "dumb" {
		mark = "+G>+"
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(mark + " gasacli")
	return box.Render(title + "\n\n" + body.String() + "\n\n" + lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94a3b8"}).Render(footer))
}

func readinessLabel(status discovery.Status) string {
	switch status {
	case discovery.Available:
		return "READY"
	case discovery.VersionUnknown:
		return "INSTALADO"
	case discovery.NotFound:
		return "AUSENTE"
	default:
		return "DESCONHECIDO"
	}
}

func editionLabel(index int) string {
	if index == 1 {
		return "Pro (ativação depois)"
	}
	return "Community"
}

func truncateColumns(value string, width int) string {
	if width < 1 {
		return ""
	}
	if lipgloss.Width(value) <= width {
		return value
	}
	var out strings.Builder
	for _, r := range value {
		candidate := out.String() + string(r) + "…"
		if lipgloss.Width(candidate) > width {
			break
		}
		out.WriteRune(r)
	}
	return out.String() + "…"
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
