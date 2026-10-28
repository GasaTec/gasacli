package tui

import (
	"context"
	"errors"
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
	session          *Session
	project          string
	trusted          bool
	trustYes         bool
	chat             textarea.Model
	spinner          spinner.Model
	view             viewport.Model
	lines            []string
	history          []string
	width            int
	height           int
	ready            bool
	closed           bool
	quitErr          error
	palette          bool
	selected         int
	status           string
	busy             bool
	setupRequested   bool
	trustDenied      bool
	agentsChecking   bool
	commandCancel    context.CancelFunc
	lastCtrlC        time.Time
	lastCtrlCValid   bool
	approval         *approvalPrompt
	pendingApprovals []approvalPrompt
	quitConfirmation bool
	quitYes          bool
}

type commandResult struct{ text string }
type readyAgentsResult struct{ count int }
type handlerChunk struct{ text string }
type handlerDone struct {
	text string
	err  error
}
type approvalAskMsg struct {
	request PermissionRequest
	answer  chan ApprovalChoice
}
type approvalPrompt struct {
	request PermissionRequest
	answer  chan ApprovalChoice
	choice  ApprovalChoice
}

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
		status:         i18n.T("tui.status.ready"),
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
		s.sendMessage = func(message any) { program.Send(message) }
		final, err := program.Run()
		s.sendMessage = nil
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
		m.status = i18n.T("tui.status.ready")
		return m, nil
	case handlerChunk:
		if msg.text != "" {
			m.append(msg.text)
		}
		return m, nil
	case handlerDone:
		m.busy = false
		if m.commandCancel != nil {
			m.commandCancel()
			m.commandCancel = nil
		}
		if msg.text != "" {
			m.append("GasaCLI\n" + msg.text)
		}
		if errors.Is(msg.err, context.Canceled) {
			m.append("GasaCLI\ncancelado")
		}
		if msg.err != nil && !errors.Is(msg.err, context.Canceled) && !errors.Is(msg.err, errExitCommand) && !errors.Is(msg.err, errSetupCommand) {
			m.append("GasaCLI\n" + msg.err.Error())
		}
		if errors.Is(msg.err, errExitCommand) {
			if hasActiveProjectRun(m.project) {
				m.quitConfirmation = true
				m.quitYes = false
				m.status = i18n.T("tui.quit.active")
				return m, nil
			}
			m.closed = true
			return m, tea.Quit
		}
		if errors.Is(msg.err, errSetupCommand) {
			m.setupRequested = true
			return m, tea.Quit
		}
		m.status = i18n.T("tui.status.ready")
		return m, nil
	case approvalAskMsg:
		prompt := approvalPrompt{request: msg.request, answer: msg.answer, choice: ApprovalDeny}
		if m.approval == nil {
			m.approval = &prompt
		} else {
			m.pendingApprovals = append(m.pendingApprovals, prompt)
		}
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
		if m.quitConfirmation {
			switch msg.String() {
			case "y", "Y", "right", "l":
				m.quitYes = true
			case "n", "N", "left", "h":
				m.quitYes = false
			case "enter":
				m.quitConfirmation = false
				if m.quitYes {
					cancelProjectRuns(m.project)
					m.closed = true
					return m, tea.Quit
				}
				m.status = i18n.T("tui.status.ready")
			case "esc":
				m.quitConfirmation = false
				m.status = i18n.T("tui.status.ready")
			}
			return m, nil
		}
		if m.approval != nil {
			switch msg.String() {
			case "left", "h":
				if m.approval.choice > ApprovalOnce {
					m.approval.choice--
				}
			case "right", "l":
				if m.approval.choice < ApprovalDeny {
					m.approval.choice++
				}
			case "y", "Y":
				m.approval.choice = ApprovalRun
			case "n", "N":
				m.approval.choice = ApprovalDeny
			case "enter":
				m.finishApproval(m.approval.choice)
			case "esc", "ctrl+c":
				m.finishApproval(ApprovalDeny)
			}
			return m, nil
		}
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
			if m.busy && m.commandCancel != nil {
				if m.lastCtrlCValid && time.Since(m.lastCtrlC) <= time.Second {
					return m, tea.Quit
				}
				m.lastCtrlC, m.lastCtrlCValid = time.Now(), true
				m.commandCancel()
				m.status = "cancelado"
				return m, nil
			}
			m.status = i18n.T("tui.status.draft")
			return m, nil
		case "ctrl+d":
			if hasActiveProjectRun(m.project) {
				m.quitConfirmation = true
				m.quitYes = false
				m.status = i18n.T("tui.quit.active")
				return m, nil
			}
			return m, tea.Quit
		case "esc":
			if strings.HasPrefix(strings.TrimSpace(m.chat.Value()), "/") {
				m.palette = false
				return m, nil
			}
			m.status = i18n.T("tui.status.preserved")
			return m, nil
		case "enter", "ctrl+s":
			input := strings.TrimSpace(m.chat.Value())
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

func (m *interactiveModel) finishApproval(choice ApprovalChoice) {
	if m.approval == nil {
		return
	}
	m.approval.answer <- choice
	m.approval = nil
	if len(m.pendingApprovals) > 0 {
		next := m.pendingApprovals[0]
		m.pendingApprovals = m.pendingApprovals[1:]
		m.approval = &next
	}
}

func hasActiveProjectRun(project string) bool {
	active := false
	activeRuns.Range(func(_, value any) bool {
		if value.(activeRun).project == project {
			active = true
			return false
		}
		return true
	})
	return active
}

func cancelProjectRuns(project string) {
	activeRuns.Range(func(_, value any) bool {
		run := value.(activeRun)
		if run.project == project {
			run.cancel()
		}
		return true
	})
}

func (m interactiveModel) send(input string) (tea.Model, tea.Cmd) {
	var command tea.Cmd
	if input != "" {
		command = m.submit(input)
	}
	m.chat.SetValue("")
	m.palette = false
	if m.closed || m.setupRequested {
		return m, tea.Quit
	}
	if command != nil {
		return m, tea.Batch(command, m.spinner.Tick)
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

func (m *interactiveModel) submit(input string) tea.Cmd {
	m.history = append(m.history, input)
	m.append("You  > " + strings.ReplaceAll(input, "\n", "\n     "))
	if !strings.HasPrefix(input, "/") {
		m.append(i18n.T("command.model_unavailable"))
		return nil
	}
	_, _, err := splitCommand(input)
	if err != nil {
		m.append("GasaCLI\n" + err.Error())
		return nil
	}
	return m.startCommand(input)
}

func (m *interactiveModel) startCommand(input string) tea.Cmd {
	parent := m.session.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m.commandCancel = cancel
	m.lastCtrlCValid = false
	m.busy = true
	m.status = i18n.T("tui.running")
	if name, _, err := splitCommand(input); err == nil && (normalizeCommandName(name) == "/agents" || normalizeCommandName(name) == "/doctor") {
		m.status = i18n.T("tui.discovering")
	}
	sendMessage := m.session.sendMessage
	if sendMessage != nil {
		go func() {
			result, err := executeCommand(ctx, m.session, m.project, m.trusted, input, func(chunk string) {
				sendMessage(handlerChunk{text: chunk})
			}, func(askContext context.Context, request PermissionRequest) (ApprovalChoice, error) {
				answer := make(chan ApprovalChoice, 1)
				sendMessage(approvalAskMsg{request: request, answer: answer})
				select {
				case choice := <-answer:
					return choice, nil
				case <-askContext.Done():
					return ApprovalDeny, askContext.Err()
				}
			})
			sendMessage(handlerDone{text: result, err: err})
		}()
	} else {
		result, err := executeCommand(ctx, m.session, m.project, m.trusted, input, nil, nil)
		m.busy = false
		cancel()
		if errors.Is(err, errSetupCommand) {
			m.setupRequested = true
			return nil
		}
		if errors.Is(err, errExitCommand) {
			m.closed = true
			return nil
		}
		if result != "" {
			m.append("GasaCLI\n" + result)
		}
		if err != nil && !errors.Is(err, errExitCommand) {
			m.append("GasaCLI\n" + err.Error())
		}
	}
	return m.spinner.Tick
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
	meta := fmt.Sprintf("  %s  |  %s: %s  |  Dashboard: %s (%s)  |  %s", i18n.T("tui.project", filepath.Base(m.project)), i18n.T("tui.trust.label"), i18n.T("tui.trust.granted"), valueOr(m.session.Dashboard, "não iniciado"), valueOr(m.session.DashboardState, "ocioso"), i18n.T("tui.agents.ready", m.agentsReadyLabel()))
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
		footer = i18n.T("tui.status.palette")
	}
	view := lipgloss.JoinVertical(lipgloss.Left, box, conversation, input, footer)
	if m.quitConfirmation {
		choices := []string{i18n.T("tui.quit.stay"), i18n.T("tui.quit.cancel")}
		if !m.quitYes {
			choices[0] = "> " + choices[0]
		} else {
			choices[1] = "> " + choices[1]
		}
		content := i18n.T("tui.quit.active") + "\n\n" + choices[0] + "    " + choices[1] + "\n\n" + i18n.T("goal.approval.help")
		style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#F59E0B")).Padding(1, 2).Width(max(40, min(76, m.width-4)))
		return lipgloss.Place(max(40, m.width), max(12, m.height), lipgloss.Center, lipgloss.Center, style.Render(content))
	}
	if m.approval != nil {
		request := m.approval.request
		choices := []string{i18n.T("goal.approval.once"), i18n.T("goal.approval.run"), i18n.T("goal.approval.deny")}
		for index := range choices {
			prefix := "  "
			if int(m.approval.choice) == index {
				prefix = "> "
			}
			choices[index] = prefix + choices[index]
		}
		content := fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s: %s\n%s: %s\n\n%s    %s    %s\n\n%s", i18n.T("tui.approval.title"), i18n.T("tui.approval.agent"), valueOr(request.Agent, "GasaCLI"), i18n.T("tui.approval.action"), i18n.T("permission."+request.Action), i18n.T("tui.approval.target"), request.Target, i18n.T("tui.approval.command"), request.ExactCommand, choices[0], choices[1], choices[2], i18n.T("goal.approval.help"))
		border := lipgloss.RoundedBorder()
		if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
			border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
		}
		dialog := lipgloss.NewStyle().Border(border).Padding(1, 2).Width(max(40, min(76, m.width-4))).Render(content)
		return lipgloss.Place(max(40, m.width), max(12, m.height), lipgloss.Center, lipgloss.Center, dialog)
	}
	return view
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
		return i18n.T("tui.agents.checking")
	}
	return fmt.Sprint(m.session.ReadyAgents)
}

func (m interactiveModel) welcomeCard() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s gasacli\n", m.brandMark())
	fmt.Fprintf(&b, "%s  |  %s: %s\n", i18n.T("tui.project", filepath.Base(m.project)), i18n.T("tui.trust.label"), i18n.T("tui.trust.granted"))
	fmt.Fprintf(&b, "%s\n\n", i18n.T("tui.agents.ready", m.agentsReadyLabel()))
	b.WriteString("Comece por aqui\n")
	b.WriteString("/help       ver comandos e atalhos\n")
	b.WriteString("/agents     conferir ferramentas instaladas\n")
	b.WriteString("/doctor     executar diagnóstico local\n")
	b.WriteString("Enter       enviar mensagem ou comando\n")
	b.WriteString("Alt+Enter   inserir uma nova linha\n")
	b.WriteString("/goal       " + i18n.T("tui.goal.tip") + "\n")
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
