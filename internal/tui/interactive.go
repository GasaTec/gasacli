package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/trust"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type commandInfo struct{ name, description string }

var commands = []commandInfo{
	{"/help", "Show commands and keyboard shortcuts"},
	{"/agents", "Discover installed coding agents"},
	{"/doctor", "Run local diagnostics"},
	{"/setup", "Review setup and install guidance"},
	{"/status", "Show the current session"},
	{"/goal", "Start an orchestrated task (planned, issue #11)"},
	{"/config", "Edit effective configuration (planned, issue #11)"},
	{"/skills", "Manage local skills (planned, issue #11)"},
	{"/prompts", "Manage prompts (planned, issue #11)"},
	{"/brain", "Search the local knowledge base (planned, issue #11)"},
	{"/resume", "Resume a saved run (planned, issue #11)"},
	{"/github", "Connect GitHub tools (planned, issue #11)"},
	{"/web", "Open or inspect the local dashboard"},
	{"/permissions", "Configure execution permissions (planned, issue #31)"},
	{"/backup", "Create a local backup (planned, issue #11)"},
	{"/restore", "Restore local data (planned, issue #11)"},
	{"/exit", "Close GasaCLI"},
	{"/quit", "Close GasaCLI"},
}

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
}

type commandResult struct{ text string }

func newInteractiveModel(s *Session, project string, trusted bool) interactiveModel {
	input := textarea.New()
	input.Placeholder = "Ask GasaCLI or type / for commands"
	input.Prompt = "  > "
	input.ShowLineNumbers = false
	input.SetHeight(2)
	input.CharLimit = 0
	input.KeyMap.InsertNewline.SetKeys("enter")
	input.KeyMap.InsertNewline.SetEnabled(true)
	input.Focus()
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#16866d", Dark: "#1ECFA5"})
	return interactiveModel{
		session: s, project: project, trusted: trusted, trustYes: false,
		chat: input, view: viewport.New(80, 12), spinner: spin,
		status: "Enter adds a line | Ctrl+S sends | Ctrl+D exits",
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
		m.append("Session ready. Type / to browse commands.")
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

func (m interactiveModel) Init() tea.Cmd { return textarea.Blink }

func (m interactiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case commandResult:
		m.busy = false
		if msg.text != "" {
			m.append("GasaCLI\n" + msg.text)
		}
		m.status = "Enter adds a line | Ctrl+S sends | Ctrl+D exits"
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
					return m, tea.Quit
				}
				if err := m.session.TrustStore.Grant(m.project); err != nil {
					m.quitErr = fmt.Errorf("grant project trust: %w", err)
					return m, tea.Quit
				}
				m.trusted = true
				m.append("Folder trusted. You can change this later with `gasa reset`.")
				if m.session.OnTrusted != nil {
					if err := m.session.OnTrusted(); err != nil {
						m.append("Dashboard unavailable: " + err.Error())
					}
				}
			case "ctrl+c":
				m.status = "Trust was not granted. Press n then Enter to leave, or y then Enter to trust."
			case "ctrl+d", "esc":
				m.quitErr = fmt.Errorf("project not trusted")
				return m, tea.Quit
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
			if strings.TrimSpace(m.chat.Value()) == "" {
				return m, tea.Quit
			}
			m.status = "Draft preserved. Clear it before Ctrl+D to exit."
			return m, nil
		case "esc":
			if strings.HasPrefix(strings.TrimSpace(m.chat.Value()), "/") {
				m.chat.SetValue("")
				m.palette = false
				return m, nil
			}
			m.status = "Session state is preserved."
			return m, nil
		case "ctrl+s":
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
			if input != "" {
				m.submit(input)
			}
			m.chat.SetValue("")
			m.palette = false
			if m.closed || m.setupRequested {
				return m, tea.Quit
			}
			return m, nil
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
			output.WriteString(help)
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
			fmt.Fprintf(&output, "Project: %s\nTrust: granted\nDashboard: %s (%s)\nReady agents: %d\nNo active run; orchestration is planned (issue #11).", m.project, valueOr(m.session.Dashboard, "not started"), valueOr(m.session.DashboardState, "local"), m.session.ReadyAgents)
		case "/web":
			if m.session.Web != nil {
				if err := m.session.Web(&output, fields[1:]); err != nil {
					fmt.Fprintf(&output, "/web: %v", err)
				}
			} else {
				output.WriteString("Dashboard is not configured.")
			}
		case "/goal":
			output.WriteString("/goal is planned (issue #11). No task was started.")
		case "/permissions":
			output.WriteString("Permission profiles are planned; see issue #31.")
		case "/config", "/skills", "/prompts", "/brain", "/resume", "/github", "/backup", "/restore":
			fmt.Fprintf(&output, "%s is planned (issue #11).", fields[0])
		default:
			output.WriteString("Unknown command. Type /help or / for suggestions.")
		}
		if output.Len() > 0 {
			m.append("GasaCLI\n" + output.String())
		}
		return
	}
	m.append("Model chat is not connected yet. Type /help to see available local actions.")
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (m interactiveModel) matches() []commandInfo {
	value := strings.TrimSpace(m.chat.Value())
	if value == "/" {
		return commands
	}
	needle := strings.ToLower(value)
	var found []commandInfo
	for _, command := range commands {
		if strings.Contains(strings.ToLower(command.name), needle) || strings.Contains(strings.ToLower(command.description), strings.TrimPrefix(needle, "/")) {
			found = append(found, command)
		}
	}
	return found
}

func (m interactiveModel) View() string {
	if !m.trusted {
		return m.trustView()
	}
	border := lipgloss.RoundedBorder()
	if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
		border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#16866d", Dark: "#1ECFA5"}).Render("G> GasaCLI")
	meta := fmt.Sprintf("  %s  |  Trust: granted  |  Dashboard: %s (%s)  |  Agents ready: %d", filepath.Base(m.project), valueOr(m.session.Dashboard, "not started"), valueOr(m.session.DashboardState, "idle"), m.session.ReadyAgents)
	box := lipgloss.NewStyle().Border(border).Padding(0, 1).Width(max(20, m.width-2)).Render(header + meta)
	conversation := lipgloss.NewStyle().Border(border).Padding(0, 1).Width(max(20, m.width-2)).Height(max(4, m.height-13)).Render(m.view.View())
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
			rows = append(rows, marker+command.name+"  "+command.description)
		}
		input += "\n" + lipgloss.NewStyle().Border(border).Padding(0, 1).Render(strings.Join(rows, "\n"))
		footer = "Tab completes | Up/Down selects | Esc closes suggestions"
	}
	return lipgloss.JoinVertical(lipgloss.Left, box, conversation, input, footer)
}

func (m interactiveModel) trustView() string {
	yes, no := "  Trust this folder", "  No, leave it untrusted"
	if m.trustYes {
		yes = "> Trust this folder"
	} else {
		no = "> No, leave it untrusted"
	}
	content := "GasaCLI needs your approval\n\nTrusting allows GasaCLI to discover tools and, when you ask, read or change files in this project. Trust does not approve commands, installs, or network access.\n\nExact folder:\n" + m.project + "\n\n" + yes + "\n" + no + "\n\nUse left/right or y/n, then Enter. Default: No."
	border := lipgloss.RoundedBorder()
	if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
		border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	style := lipgloss.NewStyle().Border(border).Padding(1, 2).Width(max(40, min(76, m.width-4))).BorderForeground(lipgloss.AdaptiveColor{Light: "#16866d", Dark: "#1ECFA5"})
	return lipgloss.Place(max(40, m.width), max(12, m.height), lipgloss.Center, lipgloss.Center, style.Render(content))
}
