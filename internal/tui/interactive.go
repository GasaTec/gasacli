package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/trust"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type commandInfo struct{ name, description string }

type interactiveModel struct {
	session        *Session
	project        string
	trusted        bool
	trustYes       bool
	chat           textarea.Model
	spinner        spinner.Model
	view           viewport.Model
	lines          []string
	history        []string
	width          int
	height         int
	ready          bool
	closed         bool
	quitErr        error
	palette        bool
	selected       int
	status         string
	busy           bool
	setupRequested bool
	trustDenied    bool
	agentsChecking bool
}

type commandResult struct{ text string }
type readyAgentsResult struct{ count int }

func newInteractiveModel(s *Session, project string, trusted bool) interactiveModel {
	input := textarea.New()
	input.Placeholder = i18n.T("tui.placeholder")
	input.Prompt = "  > "
	input.ShowLineNumbers = false
	input.SetHeight(2)
	input.CharLimit = 0
	input.KeyMap.InsertNewline.SetKeys("alt+enter", "ctrl+j", "shift+enter")
	input.KeyMap.InsertNewline.SetEnabled(true)
	input.Focus()
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#16866d", Dark: "#1ECFA5"})
	return interactiveModel{
		session: s, project: project, trusted: trusted, trustYes: false,
		chat: input, view: viewport.New(80, 12), spinner: spin,
		status:         "Enter envia | Alt+Enter/Ctrl+J quebra linha | Ctrl+D sai",
		agentsChecking: s.DiscoverReady != nil,
	}
}

func (s *Session) runInteractive() error {
	project, err := trust.Canonical(s.ProjectPath)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	trusted, err := s.TrustStore.IsTrusted(project)
	if err != nil {
		return fmt.Errorf("check project trust: %w", err)
	}
	m := newInteractiveModel(s, project, trusted)
	if !trusted {
		m.append("This folder is not trusted yet. GasaCLI will not access project files until you approve it.")
	} else {
		if s.OnTrusted != nil {
			if err := s.OnTrusted(); err != nil {
				return err
			}
		}
	}
	programContext := s.Context
	if programContext == nil {
		programContext = context.Background()
	}
	for {
		// /setup currently suspends this chat and runs the onboarding program in sequence.
		// Keep chat state intact until the wizard can be embedded as a mode in this model.
		program := tea.NewProgram(m, tea.WithInput(s.In), tea.WithOutput(s.Out), tea.WithAltScreen(), tea.WithContext(programContext))
		final, err := program.Run()
		if err != nil {
			return fmt.Errorf("run terminal UI: %w", err)
		}
		model, ok := final.(interactiveModel)
		if !ok {
			return nil
		}
		if !model.setupRequested {
			return model.quitErr
		}
		model.setupRequested = false
		if s.Setup == nil {
			model.append("Setup is not configured.")
		} else if err := s.Setup(s.Out); err != nil {
			model.append(fmt.Sprintf("/setup: %v", err))
		} else {
			model.append("Setup finished. Your chat session has resumed.")
		}
		m = model
	}
}

func (m *interactiveModel) append(text string) {
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		m.lines = append(m.lines, line)
	}
	m.view.SetContent(strings.Join(m.lines, "\n"))
	m.view.GotoBottom()
}

func (m interactiveModel) Init() tea.Cmd {
	commands := []tea.Cmd{textarea.Blink}
	if m.session.DiscoverReady != nil {
		commands = append(commands, func() tea.Msg { return readyAgentsResult{count: m.session.DiscoverReady()} })
	}
	return tea.Batch(commands...)
}

func (m interactiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case commandResult:
		m.busy = false
		if msg.text != "" {
			m.append("GasaCLI\n" + msg.text)
		}
		m.status = "Enter envia | Alt+Enter/Ctrl+J quebra linha | Ctrl+D sai"
		return m, nil
	case readyAgentsResult:
		m.agentsChecking = false
		m.session.ReadyAgents = msg.count
		return m, nil
	case spinner.TickMsg:
		if m.busy {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(message)
			return m, cmd
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.view.Width = max(20, msg.Width-4)
		m.view.Height = max(4, msg.Height-13)
		m.chat.SetWidth(max(20, msg.Width-9))
		return m, nil
	case tea.KeyMsg:
		if !m.trusted {
			if m.trustDenied {
				return m, nil
			}
			switch msg.String() {
			case "y", "Y":
				m.trustYes = true
			case "left", "right", "h", "l":
				m.trustYes = !m.trustYes
			case "n", "N":
				m.trustYes = false
			case "enter":
				if !m.trustYes {
					m.quitErr = fmt.Errorf("project not trusted")
					m.trustDenied = true
					return m, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return tea.QuitMsg{} })
				}
				if err := m.session.TrustStore.Grant(m.project); err != nil {
					m.quitErr = fmt.Errorf("grant project trust: %w", err)
					return m, tea.Quit
				}
				m.trusted = true
				m.status = "Pasta confiável. Use `gasa setup` para rever a configuração."
				if m.session.OnTrusted != nil {
					if err := m.session.OnTrusted(); err != nil {
						m.append("Dashboard unavailable: " + err.Error())
					}
				}
			case "ctrl+c":
				m.status = "Trust was not granted. Press n then Enter to leave, or y then Enter to trust."
			case "ctrl+d", "esc":
				m.quitErr = fmt.Errorf("project not trusted")
				m.trustDenied = true
				return m, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return tea.QuitMsg{} })
			}
			return m, nil
		}
		if m.palette {
			switch msg.String() {
			case "up":
				if m.selected > 0 {
					m.selected--
				}
				return m, nil
			case "down":
				if m.selected < len(m.matches())-1 {
					m.selected++
				}
				return m, nil
			case "tab":
				if matches := m.matches(); len(matches) > 0 {
					m.chat.SetValue(matches[m.selected].name + " ")
					m.palette = false
				}
				return m, nil
			case "esc":
				m.palette = false
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+c":
			m.status = "Draft preserved. Use Ctrl+D to exit."
			return m, nil
		case "ctrl+d":
			return m, tea.Quit
		case "esc":
			if strings.HasPrefix(strings.TrimSpace(m.chat.Value()), "/") {
				m.chat.SetValue("")
				m.palette = false
				return m, nil
			}
			m.status = "Session state is preserved."
			return m, nil
		case "enter", "ctrl+s":
			input := strings.TrimSpace(m.chat.Value())
			if input == "/setup" {
				m.history = append(m.history, input)
				m.append("You  > " + input)
				m.chat.SetValue("")
				m.palette = false
				m.setupRequested = true
				return m, tea.Quit
			}
			if input == "/doctor" || input == "/agents" {
				m.history = append(m.history, input)
				m.append("You  > " + input)
				m.chat.SetValue("")
				m.palette = false
				m.busy = true
				m.status = "Discovering installed tools..."
				return m, tea.Batch(m.runDiagnostic(input), m.spinner.Tick)
			}
			return m.send(input)
		case "ctrl+up":
			if len(m.history) > 0 {
				m.chat.SetValue(m.history[len(m.history)-1])
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.chat, cmd = m.chat.Update(message)
	if strings.HasPrefix(strings.TrimSpace(m.chat.Value()), "/") {
		m.palette = true
		m.selected = 0
	} else {
		m.palette = false
	}
	m.view, _ = m.view.Update(message)
	return m, cmd
}

func (m interactiveModel) send(input string) (tea.Model, tea.Cmd) {
	if input != "" {
		m.submit(input)
	}
	m.chat.SetValue("")
	m.palette = false
	if m.closed || m.setupRequested {
		return m, tea.Quit
	}
	return m, nil
}

func (m interactiveModel) runDiagnostic(command string) tea.Cmd {
	return func() tea.Msg {
		var output strings.Builder
		if command == "/doctor" && m.session.Doctor != nil {
			m.session.Doctor(&output)
		} else if m.session.Agents != nil {
			m.session.Agents(&output)
		} else {
			output.WriteString("Agent discovery is not configured.")
		}
		return commandResult{text: output.String()}
	}
}

func (m *interactiveModel) submit(input string) {
	m.history = append(m.history, input)
	m.append("You  › " + strings.ReplaceAll(input, "\n", "\n     "))
	if strings.HasPrefix(input, "/") {
		fields := strings.Fields(input)
		if len(fields) > 0 && (fields[0] == "/exit" || fields[0] == "/quit") {
			m.closed = true
			return
		}
		var output strings.Builder
		switch fields[0] {
		case "/help":
			output.WriteString(helpText())
		case "/agents":
			if m.session.Agents != nil {
				m.session.Agents(&output)
			} else {
				output.WriteString("Agent discovery is not configured.")
			}
		case "/doctor":
			if m.session.Doctor != nil {
				m.session.Doctor(&output)
			} else {
				output.WriteString("Diagnostics are not configured.")
			}
		case "/setup":
			m.setupRequested = true
		case "/status":
			fmt.Fprintf(&output, "Project: %s\nTrust: granted\nDashboard: %s (%s)\nAgents ready: %s\nNo active run; orchestration is planned (issue #11).", m.project, valueOr(m.session.Dashboard, "not started"), valueOr(m.session.DashboardState, "local"), m.agentsReadyLabel())
		case "/web":
			if m.session.Web != nil {
				if err := m.session.Web(&output, fields[1:]); err != nil {
					fmt.Fprintf(&output, "/web: %v", err)
				}
			} else {
				output.WriteString("Dashboard is not configured.")
			}
		default:
			if command, ok := lookupCommand(fields[0]); ok && command.Planned {
				if command.Handler != nil {
					result, err := command.Handler(context.Background(), nil, fields[1:])
					if err != nil {
						output.WriteString(err.Error())
					} else {
						output.WriteString(result)
					}
				}
			} else {
				output.WriteString(unknownCommandMessage(fields[0]))
			}
		}
		if output.Len() > 0 {
			m.append("GasaCLI\n" + output.String())
		}
		return
	}
	m.append(i18n.T("command.model_unavailable"))
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (m interactiveModel) matches() []commandInfo {
	value := strings.TrimSpace(m.chat.Value())
	needle := strings.ToLower(value)
	var found []commandInfo
	for _, command := range RegisteredCommands() {
		info := commandInfo{name: command.Name, description: commandSummary(command)}
		if strings.Contains(strings.ToLower(info.name), needle) || strings.Contains(strings.ToLower(info.description), strings.TrimPrefix(needle, "/")) {
			found = append(found, info)
		}
	}
	return found
}

func (m interactiveModel) View() string {
	if !m.trusted {
		return m.trustView()
	}
	border := lipgloss.Border{Top: "─", Bottom: "─", Left: "│", Right: "│", TopLeft: "┌", TopRight: "┐", BottomLeft: "└", BottomRight: "┘"}
	if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
		border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#087E68", Dark: "#1ECFA5"}).Render(m.brandMark() + " gasacli")
	meta := fmt.Sprintf("  Projeto: %s  |  Confiança: concedida  |  Dashboard: %s (%s)  |  Agentes prontos: %s", filepath.Base(m.project), valueOr(m.session.Dashboard, "não iniciado"), valueOr(m.session.DashboardState, "ocioso"), m.agentsReadyLabel())
	box := lipgloss.NewStyle().Border(border).Foreground(lipgloss.AdaptiveColor{Light: "#172A46", Dark: "#F4F7FB"}).Padding(0, 1).Width(max(20, m.width-2)).Render(header + meta)
	conversationContent := m.view.View()
	if len(m.lines) == 0 {
		conversationContent = m.welcomeCard()
	}
	conversation := lipgloss.NewStyle().Border(border).Padding(0, 1).Width(max(20, m.width-2)).Height(max(4, m.height-13)).Render(conversationContent)
	input := lipgloss.NewStyle().Border(border).Padding(0, 1).Width(max(20, m.width-2)).Render(m.chat.View())
	status := m.status
	if m.busy {
		status = m.spinner.View() + " " + status
	}
	footer := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#475569", Dark: "#94a3b8"}).Render(status)
	if m.palette {
		matches := m.matches()
		var rows []string
		for index, command := range matches {
			marker := "  "
			if index == m.selected {
				marker = "> "
			}
			rows = append(rows, truncateLine(marker+command.name+"  "+command.description, max(16, m.width-10)))
		}
		input += "\n" + lipgloss.NewStyle().Border(border).Padding(0, 1).Render(strings.Join(rows, "\n"))
		footer = "Tab completa | ↑/↓ seleciona | Enter envia | Esc fecha sugestões"
	}
	return lipgloss.JoinVertical(lipgloss.Left, box, conversation, input, footer)
}

func (m interactiveModel) trustView() string {
	if m.trustDenied {
		content := "Configuração encerrada\n\nEsta pasta continua sem confiança. O GasaCLI não acessou arquivos do projeto.\n\nPara revisar a confiança depois, execute `gasa setup`.\n\nVocê pode fechar este terminal."
		return lipgloss.Place(max(40, m.width), max(12, m.height), lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1, 2).Render(content))
	}
	yes, no := "  Confiar nesta pasta", "  Manter sem confiança"
	if m.trustYes {
		yes = "> Confiar nesta pasta"
	} else {
		no = "> Manter sem confiança"
	}
	content := "Confirme a confiança do workspace\n\nA confiança permite descobrir ferramentas e acessar arquivos do projeto quando você solicitar. Ela não aprova comandos, instalações ou acesso à rede.\n\nPasta:\n" + m.project + "\n\n" + yes + "\n" + no + "\n\nUse ←/→ ou y/n e pressione Enter. Padrão: manter sem confiança."
	border := lipgloss.RoundedBorder()
	if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
		border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	style := lipgloss.NewStyle().Border(border).Padding(1, 2).Width(max(40, min(76, m.width-4))).BorderForeground(lipgloss.AdaptiveColor{Light: "#16866d", Dark: "#1ECFA5"})
	return lipgloss.Place(max(40, m.width), max(12, m.height), lipgloss.Center, lipgloss.Center, style.Render(content))
}

func (m interactiveModel) brandMark() string {
	if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
		return "+G>+"
	}
	return "╭G›╮"
}

func (m interactiveModel) agentsReadyLabel() string {
	if m.agentsChecking {
		return "checking..."
	}
	return fmt.Sprint(m.session.ReadyAgents)
}

func (m interactiveModel) welcomeCard() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s gasacli\n", m.brandMark())
	fmt.Fprintf(&b, "Projeto: %s  |  Confiança: concedida\n", filepath.Base(m.project))
	fmt.Fprintf(&b, "Agentes prontos: %s\n\n", m.agentsReadyLabel())
	b.WriteString("Comece por aqui\n")
	b.WriteString("/help       ver comandos e atalhos\n")
	b.WriteString("/agents     conferir ferramentas instaladas\n")
	b.WriteString("/doctor     executar diagnóstico local\n")
	b.WriteString("Enter       enviar mensagem ou comando\n")
	b.WriteString("Alt+Enter   inserir uma nova linha\n")
	b.WriteString("/goal       planejado; nenhuma tarefa será iniciada\n")
	return b.String()
}

func truncateLine(value string, width int) string {
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
