package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/engine"
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
	transcript       []string
	rendered         []string
	renderWidth      int
	lightTheme       bool
	streaming        bool
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
	hideSidebar      bool
	sidebar          sidebarSnapshot
	sidebarLoading   bool
	projectPicker    bool
	projectIndex     int
	dialogScroll     int
	heapBytes        uint64
	rssBytes         uint64
	rssKnown         bool
	attempts         map[attemptKey]attemptState
	lastAgentMetrics *engine.AgentMetrics
	keys             config.TUIKeys
	selectedSession  string
	selectedAgent    string
}

type heapSample struct {
	heap, rss uint64
	rssKnown  bool
}
type attemptKey struct {
	run       string
	execution uint64
}
type attemptState struct {
	attempt     int
	agent, role string
}
type attemptMsg struct {
	run, role, agent string
	event            adapters.AttemptEvent
}
type agentMetricsMsg struct {
	project string
	metrics engine.AgentMetrics
}
type sidebarLoaded struct{ snapshot sidebarSnapshot }
type commandResult struct{ text string }
type readyAgentsResult struct{ count int }
type handlerChunk struct {
	text   string
	stream bool
}
type handlerDone struct {
	text string
	err  error
}
type sessionSelected struct {
	id, agent string
	err       error
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
	keys, _ := (config.TUIKeys{}).Resolved()
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
		session: s, project: project, trusted: trusted, trustYes: false, width: 80, height: 24, keys: keys,
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
	if s.ConfigHome != "" {
		cfg, err := config.LoadFrom(s.ConfigHome)
		if err != nil {
			return fmt.Errorf("load TUI configuration: %w", err)
		}
		m.keys, err = cfg.TUI.Keys.Resolved()
		if err != nil {
			return err
		}
	}
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
	m.streaming = false
	follow := m.view.AtBottom()
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		m.lines = append(m.lines, line)
	}
	m.transcript = append(m.transcript, text)
	m.rendered = append(m.rendered, m.renderMessage(text))
	m.renderWidth = m.view.Width
	m.view.SetContent(strings.Join(m.rendered, "\n"))
	if follow {
		m.view.GotoBottom()
	}
}

func (m interactiveModel) Init() tea.Cmd {
	commands := []tea.Cmd{textarea.Blink, sampleHeap()}
	if m.trusted {
		commands = append(commands, m.loadSidebar())
	}
	if m.session.DiscoverReady != nil {
		commands = append(commands, func() tea.Msg { return readyAgentsResult{count: m.session.DiscoverReady()} })
	}
	return tea.Batch(commands...)
}

func (m interactiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case heapSample:
		m.heapBytes, m.rssBytes, m.rssKnown = msg.heap, msg.rss, msg.rssKnown
		for key := range m.attempts {
			if _, active := activeRuns.Load(key.run); !active {
				delete(m.attempts, key)
			}
		}
		return m, sampleHeap()
	case agentMetricsMsg:
		if msg.project == m.project {
			value := msg.metrics
			m.lastAgentMetrics = &value
		}
		return m, nil
	case attemptMsg:
		run, ok := activeRuns.Load(msg.run)
		if !ok || run.(activeRun).project != m.project {
			return m, nil
		}
		key := attemptKey{run: msg.run, execution: msg.event.ExecutionID}
		if m.attempts == nil {
			m.attempts = make(map[attemptKey]attemptState)
		}
		previous, exists := m.attempts[key]
		if msg.event.Status == "started" {
			if !exists || msg.event.Attempt > previous.attempt {
				m.attempts[key] = attemptState{attempt: msg.event.Attempt, agent: msg.agent, role: msg.role}
			}
		} else if exists && previous.attempt == msg.event.Attempt {
			delete(m.attempts, key)
		}
		return m, nil
	case sidebarLoaded:
		m.sidebar, m.sidebarLoading = msg.snapshot, false
		return m, nil
	case sessionSelected:
		m.busy = false
		if m.commandCancel != nil {
			m.commandCancel()
			m.commandCancel = nil
		}
		if msg.err != nil {
			m.append("GasaCLI\n" + msg.err.Error())
		} else {
			m.selectedSession, m.selectedAgent = msg.id, msg.agent
			m.append("GasaCLI\nSessão selecionada: " + shortSessionID(msg.id) + " (" + msg.agent + ")")
		}
		m.status = i18n.T("tui.status.ready")
		return m, nil
	case commandResult:
		m.busy = false
		if msg.text != "" {
			m.append("GasaCLI\n" + msg.text)
		}
		m.status = i18n.T("tui.status.ready")
		return m, nil
	case handlerChunk:
		if msg.text != "" {
			if msg.stream {
				m.appendChunk(msg.text)
			} else {
				m.append(msg.text)
			}
		}
		return m, nil
	case handlerDone:
		if m.streaming && len(m.transcript) > 0 {
			follow := m.view.AtBottom()
			index := len(m.transcript) - 1
			m.rendered[index] = m.renderMessage(m.transcript[index])
			m.view.SetContent(strings.Join(m.rendered, "\n"))
			if follow {
				m.view.GotoBottom()
			}
			m.streaming = false
		}
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
		m.resize()
		return m, nil
	case tea.KeyMsg:
		if m.quitConfirmation {
			if m.scrollDialog(msg) {
				return m, nil
			}
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
			if m.scrollDialog(msg) {
				return m, nil
			}
			switch msg.String() {
			case "left", "h":
				if m.approval.choice > ApprovalOnce {
					m.approval.choice--
				}
			case "right", "l":
				if m.approval.choice < ApprovalDeny {
					m.approval.choice++
				}
			case "tab":
				m.approval.choice = (m.approval.choice + 1) % (ApprovalDeny + 1)
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
			if m.scrollDialog(msg) {
				return m, nil
			}
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
				m.sidebarLoading = true
				return m, m.loadSidebar()
			case "ctrl+c":
				m.status = "Trust was not granted. Press n then Enter to leave, or y then Enter to trust."
			case "ctrl+d", "esc":
				m.quitErr = fmt.Errorf("project not trusted")
				m.trustDenied = true
				return m, tea.Tick(900*time.Millisecond, func(time.Time) tea.Msg { return tea.QuitMsg{} })
			}
			return m, nil
		}
		if m.projectPicker {
			switch msg.String() {
			case "esc":
				m.projectPicker = false
			case "up":
				if m.projectIndex > 0 {
					m.projectIndex--
				}
			case "down":
				if m.projectIndex+1 < len(m.sidebar.Projects) {
					m.projectIndex++
				}
			case "enter":
				if m.busy || hasActiveProjectRun(m.project) {
					m.status = "Aguarde ou cancele a execução antes de trocar de projeto"
					m.projectPicker = false
					return m, nil
				}
				if m.projectIndex >= len(m.sidebar.Projects) {
					m.projectPicker = false
					return m, nil
				}
				path, err := trust.Canonical(m.sidebar.Projects[m.projectIndex].Path)
				if err == nil {
					var trusted bool
					trusted, err = m.session.TrustStore.IsTrusted(path)
					if err == nil && !trusted {
						err = fmt.Errorf("project trust was revoked")
					}
				}
				if err != nil {
					m.status = "Projeto indisponível: " + err.Error()
					m.projectPicker = false
					return m, nil
				}
				m.project, m.session.ProjectPath = path, path
				m.selectedSession, m.selectedAgent = "", ""
				m.lines, m.history, m.transcript, m.rendered = nil, nil, nil, nil
				m.attempts = nil
				m.lastAgentMetrics = nil
				m.view.SetContent("")
				m.chat.SetValue("")
				m.projectPicker = false
				m.status = i18n.T("tui.status.ready")
				return m, m.loadSidebar()
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
		case m.keys.Theme:
			m.lightTheme = !m.lightTheme
			m.renderWidth = -1
			m.resize()
			return m, nil
		case m.keys.Projects:
			if strings.TrimSpace(m.chat.Value()) != "" {
				m.status = "Envie ou limpe o rascunho antes de trocar de projeto"
				return m, nil
			}
			if m.busy || hasActiveProjectRun(m.project) {
				m.status = "Aguarde ou cancele a execução antes de trocar de projeto"
				return m, nil
			}
			m.projectPicker, m.palette, m.projectIndex = true, false, 0
			return m, nil
		case m.keys.Palette:
			draft := strings.TrimSpace(m.chat.Value())
			if draft != "" && !strings.HasPrefix(draft, "/") {
				m.status = "Envie ou limpe o rascunho antes de abrir comandos"
				return m, nil
			}
			if draft == "" {
				m.chat.SetValue("/")
			}
			m.palette = true
			m.selected = 0
			return m, nil
		case m.keys.Sidebar:
			m.hideSidebar = !m.hideSidebar
			m.resize()
			return m, nil
		case "pgup", "pgdown":
			var cmd tea.Cmd
			m.view, cmd = m.view.Update(msg)
			return m, cmd
		case "ctrl+c":
			if m.busy && m.commandCancel != nil {
				if m.lastCtrlCValid && time.Since(m.lastCtrlC) <= time.Second {
					if hasActiveProjectRun(m.project) {
						m.quitConfirmation = true
						m.quitYes = false
						m.status = i18n.T("tui.quit.active")
						return m, nil
					}
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
			input := m.chat.Value()
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
	if m.busy {
		return m, nil
	}
	var command tea.Cmd
	if strings.TrimSpace(input) != "" {
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
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		if m.selectedSession == "" {
			m.append("GasaCLI\nSelecione uma sessão com /sessions e /use <id> antes de enviar uma mensagem.")
			return nil
		}
		return m.startLiveChat(input)
	}
	name, args, err := splitCommand(trimmed)
	if err != nil {
		m.append("GasaCLI\n" + err.Error())
		return nil
	}
	if normalizeCommandName(name) == "/use" {
		if len(args) == 0 {
			if m.selectedSession == "" {
				m.append("GasaCLI\nNenhuma sessão selecionada. Use /sessions para listar sessões.")
			} else {
				m.append("GasaCLI\nSessão selecionada: " + shortSessionID(m.selectedSession) + " (" + m.selectedAgent + ")")
			}
			return nil
		}
		if len(args) != 1 {
			m.append("GasaCLI\nUso: /use [sessao]")
			return nil
		}
		return m.startSessionSelection(args[0])
	}
	return m.startCommand(trimmed)
}

func (m *interactiveModel) startSessionSelection(query string) tea.Cmd {
	parent := m.session.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m.commandCancel = cancel
	m.lastCtrlCValid = false
	m.busy = true
	m.status = i18n.T("tui.running")
	session, project, trusted := m.session, m.project, m.trusted
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		item, err := selectLiveSession(ctx, session, project, trusted, query)
		if ctx.Err() != nil {
			return sessionSelected{err: ctx.Err()}
		}
		return sessionSelected{id: item.ID, agent: item.Agent, err: err}
	})
}

func (m *interactiveModel) startLiveChat(input string) tea.Cmd {
	parent := m.session.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m.commandCancel = cancel
	m.lastCtrlCValid = false
	m.busy = true
	m.status = i18n.T("tui.running")
	session, project, trusted, id := m.session, m.project, m.trusted, m.selectedSession
	sendMessage := session.sendMessage
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		var emit func(string)
		if sendMessage != nil {
			emit = func(chunk string) {
				sendMessage(handlerChunk{text: chunk, stream: true})
			}
		}
		ask := func(askContext context.Context, request PermissionRequest) (ApprovalChoice, error) {
			if sendMessage == nil {
				return ApprovalDeny, fmt.Errorf("approval is unavailable")
			}
			answer := make(chan ApprovalChoice, 1)
			sendMessage(approvalAskMsg{request: request, answer: answer})
			select {
			case choice := <-answer:
				return choice, nil
			case <-askContext.Done():
				return ApprovalDeny, askContext.Err()
			}
		}
		result, err := executeLiveChat(ctx, session, project, trusted, id, input, emit, ask)
		return handlerDone{text: result, err: err}
	})
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
	project := m.project
	if sendMessage != nil {
		go func() {
			result, err := executeCommandWithAttempts(ctx, m.session, m.project, m.trusted, input, func(chunk string) {
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
			}, func(run, role, agent string, event adapters.AttemptEvent) {
				sendMessage(attemptMsg{run: run, role: role, agent: agent, event: event})
			}, func(metrics engine.AgentMetrics) {
				sendMessage(agentMetricsMsg{project: project, metrics: metrics})
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

func (m interactiveModel) renderMessage(text string) string {
	if strings.HasPrefix(text, "You  > ") {
		label := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1ECFA5")).Render("Você")
		return label + "  " + renderTranscript(strings.TrimPrefix(text, "You  > "), m.view.Width, m.lightTheme)
	}
	if strings.HasPrefix(text, "GasaCLI\n") {
		labelColor := lipgloss.Color("#F4F7FB")
		if m.lightTheme {
			labelColor = lipgloss.Color("#172A46")
		}
		label := lipgloss.NewStyle().Bold(true).Foreground(labelColor).Render("GasaCLI")
		body := strings.TrimPrefix(text, "GasaCLI\n")
		upper := strings.ToUpper(body)
		for _, state := range []struct {
			name  string
			color lipgloss.Color
		}{{"BLOCKED", "#FF7272"}, {"DONE", "#43D19E"}, {"GATES", "#79B8FF"}, {"REVIEW", "#79B8FF"}, {"RUNNING", "#F2B84B"}} {
			if strings.Contains(upper, state.name) {
				badge := lipgloss.NewStyle().Bold(true).Foreground(state.color).Render("[" + state.name + "]")
				body = strings.Replace(body, state.name, badge, 1)
				break
			}
		}
		content := label + "\n" + renderTranscript(body, m.view.Width, m.lightTheme)
		if strings.Contains(upper, "DONE") || strings.Contains(upper, "BLOCKED") {
			return lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#7F8CA0", Dark: "#596982"}).Padding(0, 1).Render(content)
		}
		return content
	}
	return renderTranscript(text, m.view.Width, m.lightTheme)
}

func (m *interactiveModel) appendChunk(text string) {
	if !m.streaming || len(m.transcript) == 0 {
		m.append(text)
		m.streaming = true
		m.rendered[len(m.rendered)-1] = terminalText(text)
		m.view.SetContent(strings.Join(m.rendered, "\n"))
		return
	}
	follow := m.view.AtBottom()
	index := len(m.transcript) - 1
	m.transcript[index] += text
	m.lines = append(m.lines, text)
	m.rendered[index] = terminalText(m.transcript[index])
	m.view.SetContent(strings.Join(m.rendered, "\n"))
	if follow {
		m.view.GotoBottom()
	}
}

func sampleHeap() tea.Cmd {
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		rss, known := processRSSBytes()
		return heapSample{heap: goHeapBytes(), rss: rss, rssKnown: known}
	})
}

func (m interactiveModel) loadSidebar() tea.Cmd {
	return func() tea.Msg { return sidebarLoaded{snapshot: loadSidebarSnapshot(m.session, m.project)} }
}

func (m interactiveModel) sidebarText() string {
	var lines []string
	lines = append(lines, "PROJECTS")
	if len(m.sidebar.Projects) == 0 {
		lines = append(lines, "> "+filepath.Base(m.project))
	}
	for _, project := range m.sidebar.Projects {
		marker := "  "
		if project.Active {
			marker = "> "
		}
		lines = append(lines, marker+project.Name)
	}
	lines = append(lines, "", "AGENTS")
	if len(m.sidebar.Agents) == 0 {
		if m.sidebarLoading || m.agentsChecking {
			lines = append(lines, "  Discovering...")
		} else if m.sidebar.Err != nil {
			lines = append(lines, "  Discovery unavailable")
		} else {
			lines = append(lines, "  No agents discovered")
		}
	}
	for _, agent := range m.sidebar.Agents {
		status := string(agent.Status)
		for key, active := range m.attempts {
			if _, ok := activeRuns.Load(key.run); ok && strings.EqualFold(active.agent, agent.ID) {
				status = "running"
				break
			}
		}
		lines = append(lines, "  "+agent.Name+" ["+status+"]")
	}
	unknown := make(map[string]bool)
	for key, active := range m.attempts {
		if _, ok := activeRuns.Load(key.run); !ok {
			continue
		}
		found := false
		for _, agent := range m.sidebar.Agents {
			if strings.EqualFold(active.agent, agent.ID) {
				found = true
				break
			}
		}
		if !found {
			unknown[active.agent] = true
		}
	}
	names := make([]string, 0, len(unknown))
	for name := range unknown {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, "  "+name+" [running]")
	}
	if m.selectedSession != "" {
		lines = append(lines, "", "SESSION", shortSessionID(m.selectedSession), m.selectedAgent)
	}
	if m.sidebar.Err != nil && len(m.sidebar.Agents) > 0 {
		lines = append(lines, "", "Discovery unavailable")
	}
	for i := range lines {
		fullLine := lines[i]
		lines[i] = truncateLine(lines[i], 22)
		if lines[i] == "PROJECTS" || lines[i] == "AGENTS" || lines[i] == "SESSION" {
			color := lipgloss.Color("#7D95AB")
			if m.lightTheme {
				color = lipgloss.Color("#475569")
			}
			lines[i] = lipgloss.NewStyle().Bold(true).Foreground(color).Render(lines[i])
		} else if strings.HasPrefix(lines[i], "> ") {
			color := lipgloss.Color("#1ECFA5")
			if m.lightTheme {
				color = lipgloss.Color("#087E68")
			}
			lines[i] = lipgloss.NewStyle().Bold(true).Foreground(color).Render(lines[i])
		} else {
			var dark, light lipgloss.Color
			switch {
			case strings.Contains(fullLine, "[running]"):
				dark, light = "#F7C65C", "#805A00"
			case strings.Contains(fullLine, "[ready]"):
				dark, light = "#55D997", "#087647"
			case strings.Contains(fullLine, "[error]"), strings.Contains(fullLine, "Discovery unavailable"):
				dark, light = "#FF7A85", "#B42332"
			case strings.Contains(fullLine, "[missing]"):
				dark, light = "#91A0AF", "#52606D"
			}
			if dark != "" {
				if m.lightTheme {
					dark = light
				}
				lines[i] = lipgloss.NewStyle().Foreground(dark).Render(lines[i])
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (m interactiveModel) showSidebar() bool {
	return !m.hideSidebar && m.width >= 96 && m.height >= 18
}

func (m *interactiveModel) resize() {
	width := m.width
	if m.showSidebar() {
		width -= 26
	}
	m.view.Width = max(1, width-4)
	if m.renderWidth != m.view.Width && len(m.transcript) > 0 {
		follow := m.view.AtBottom()
		m.rendered = m.rendered[:0]
		for index, text := range m.transcript {
			if m.streaming && index == len(m.transcript)-1 {
				m.rendered = append(m.rendered, terminalText(text))
			} else {
				m.rendered = append(m.rendered, m.renderMessage(text))
			}
		}
		m.view.SetContent(strings.Join(m.rendered, "\n"))
		if follow {
			m.view.GotoBottom()
		}
		m.renderWidth = m.view.Width
	}
	m.view.Height = max(1, m.height-10)
	m.chat.SetWidth(max(1, m.width-6))
}

func keyLabel(key string) string {
	if strings.HasPrefix(key, "ctrl+") {
		return "Ctrl+" + strings.ToUpper(strings.TrimPrefix(key, "ctrl+"))
	}
	return strings.ToUpper(key)
}

func (m interactiveModel) View() string {
	if !m.trusted {
		return m.trustView()
	}
	// Restore dimensions on render as setup can resume without a resize event.
	m.resize()
	border := lipgloss.RoundedBorder()
	if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
		border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	width, height := max(1, m.width), max(1, m.height)
	accent := lipgloss.Color("#1ECFA5")
	muted := lipgloss.Color("#94A3B8")
	borderColor := lipgloss.Color("#31465D")
	headerBackground := lipgloss.Color("#11283C")
	if m.lightTheme {
		accent, muted = lipgloss.Color("#087E68"), lipgloss.Color("#475569")
		borderColor = lipgloss.Color("#94A3B8")
		headerBackground = lipgloss.Color("#E4F3F6")
	}
	header := m.brandMark() + " gasacli " + m.session.Version
	meta := fmt.Sprintf("  %s  |  %s: %s  |  %s", i18n.T("tui.project", filepath.Base(m.project)), i18n.T("tui.trust.label"), i18n.T("tui.trust.granted"), i18n.T("tui.agents.ready", m.agentsReadyLabel()))
	if m.selectedSession != "" {
		meta = "  " + m.selectedAgent + " " + shortSessionID(m.selectedSession) + meta
	}
	// Truncate plain text before styling so escape sequences cannot be cut in half.
	headerText := truncateLine(header+meta, max(1, width-4))
	box := lipgloss.NewStyle().Border(border).BorderForeground(accent).Background(headerBackground).Padding(0, 1).Width(max(1, width-2)).Render(lipgloss.NewStyle().Bold(true).Foreground(accent).Background(headerBackground).Render(headerText))
	conversationContent := m.view.View()
	if len(m.lines) == 0 {
		conversationContent = m.welcomeCard()
	}
	if m.projectPicker {
		rows := []string{"Projetos confiáveis (Enter seleciona, Esc fecha)"}
		visible := max(1, m.view.Height-1)
		start := max(0, m.projectIndex-visible+1)
		for index, project := range m.sidebar.Projects {
			if index < start {
				continue
			}
			if len(rows) > visible {
				break
			}
			marker := "  "
			if index == m.projectIndex {
				marker = "> "
			}
			rows = append(rows, truncateLine(marker+project.Path, m.view.Width))
		}
		if len(m.sidebar.Projects) == 0 {
			rows = append(rows, "Nenhum projeto confiável disponível")
		}
		conversationContent = strings.Join(rows, "\n")
	}
	if m.palette {
		var rows []string
		matches := m.matches()
		start := max(0, m.selected-m.view.Height+1)
		for index, command := range matches {
			if index < start {
				continue
			}
			if len(rows) >= m.view.Height {
				break
			}
			marker := "  "
			if index == m.selected {
				marker = "> "
			}
			row := truncateLine(marker+command.name+"  "+command.description, m.view.Width)
			if index == m.selected {
				fill, ink := lipgloss.Color("#1ECFA5"), lipgloss.Color("#172A46")
				if m.lightTheme {
					fill, ink = lipgloss.Color("#087E68"), lipgloss.Color("#FFFFFF")
				}
				row = lipgloss.NewStyle().Background(fill).Foreground(ink).Bold(true).Render(row)
			}
			rows = append(rows, row)
		}
		conversationContent = strings.Join(rows, "\n")
	}
	bodyHeight := max(1, height-8)
	chatWidth := width
	if m.showSidebar() {
		chatWidth -= 26
	}
	// Keep the same dimensions as a framed panel, but leave the conversation
	// open. This gives the transcript more breathing room without changing the
	// viewport's scroll and wrapping calculations.
	openBorder := lipgloss.Border{Top: " ", Bottom: " ", Left: " ", Right: " ", TopLeft: " ", TopRight: " ", BottomLeft: " ", BottomRight: " "}
	conversation := lipgloss.NewStyle().Border(openBorder).Padding(0, 1).Width(max(1, chatWidth-2)).Height(max(1, bodyHeight-2)).MaxHeight(bodyHeight).Render(conversationContent)
	if m.showSidebar() {
		side := m.sidebarText()
		divider := openBorder
		divider.Right = "│"
		if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
			divider.Right = "|"
		}
		panel := lipgloss.NewStyle().Border(divider).BorderForeground(borderColor).Padding(0, 1).Width(24).Height(max(1, bodyHeight-2)).MaxHeight(bodyHeight).Render(side)
		conversation = lipgloss.JoinHorizontal(lipgloss.Top, panel, conversation)
	}
	input := lipgloss.NewStyle().Border(border).BorderForeground(accent).Padding(0, 1).Width(max(1, width-2)).MaxHeight(4).Render(m.chat.View())
	status := m.status
	if !m.busy && hasActiveProjectRun(m.project) {
		status = "Execução ativa"
	}
	if m.busy {
		status = m.spinner.View() + " " + status
	}
	if m.palette {
		status = i18n.T("tui.status.palette")
	}
	var measurements []string
	if metrics := m.lastAgentMetrics; metrics != nil {
		var parts []string
		if metrics.InputTokensKnown {
			parts = append(parts, fmt.Sprintf("in %d", metrics.InputTokens))
		}
		if metrics.OutputTokensKnown {
			parts = append(parts, fmt.Sprintf("out %d", metrics.OutputTokens))
		}
		if metrics.CostKnown {
			parts = append(parts, fmt.Sprintf("US$ %.4f", metrics.CostUSD))
		}
		if len(parts) > 0 {
			measurements = append(measurements, "Últ. "+metrics.Agent+" "+strings.Join(parts, " "))
		}
	}
	if m.rssKnown {
		label := "RSS"
		if runtime.GOOS == "windows" {
			label = "Working set"
		}
		measurements = append(measurements, fmt.Sprintf("%s %.1f MiB", label, float64(m.rssBytes)/(1024*1024)))
	} else if m.heapBytes > 0 {
		measurements = append(measurements, fmt.Sprintf("Go heap %.1f MiB", float64(m.heapBytes)/(1024*1024)))
	}
	if len(measurements) > 0 {
		status = strings.Join(measurements, "  |  ") + "  |  " + status
	}
	shortcuts := fmt.Sprintf("  |  %s projects  %s commands", keyLabel(m.keys.Projects), keyLabel(m.keys.Palette))
	if width >= 100 {
		shortcuts += fmt.Sprintf("  %s theme", keyLabel(m.keys.Theme))
	}
	footerText := truncateLine(status, max(1, width-lipgloss.Width(shortcuts))) + shortcuts
	footer := lipgloss.NewStyle().Foreground(muted).Render(truncateLine(footerText, width))
	view := lipgloss.JoinVertical(lipgloss.Left, box, conversation, input, footer)
	if m.quitConfirmation {
		choices := []string{i18n.T("tui.quit.stay"), i18n.T("tui.quit.cancel")}
		if !m.quitYes {
			choices[0] = "> " + choices[0]
		} else {
			choices[1] = "> " + choices[1]
		}
		content := i18n.T("tui.quit.active") + "\n\n" + choices[0] + "    " + choices[1] + "\n\n" + i18n.T("goal.approval.help")
		if m.width < 40 || m.height < 16 {
			return m.smallDialog(content)
		}
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
			if int(m.approval.choice) == index {
				fill, ink := lipgloss.Color("#1ECFA5"), lipgloss.Color("#172A46")
				if m.lightTheme {
					fill, ink = lipgloss.Color("#087E68"), lipgloss.Color("#FFFFFF")
				}
				choices[index] = lipgloss.NewStyle().Background(fill).Foreground(ink).Bold(true).Render(choices[index])
			}
		}
		content := fmt.Sprintf("%s\n\n%s: %s\n%s: %s\n%s: %s\n%s: %s\n\n%s    %s    %s\n\n%s", i18n.T("tui.approval.title"), i18n.T("tui.approval.agent"), valueOr(request.Agent, "GasaCLI"), i18n.T("tui.approval.action"), i18n.T("permission."+request.Action), i18n.T("tui.approval.target"), request.Target, i18n.T("tui.approval.command"), request.ExactCommand, choices[0], choices[1], choices[2], i18n.T("tui.approval.help"))
		if m.width < 40 || m.height < 16 {
			return m.smallDialog(content)
		}
		border := lipgloss.RoundedBorder()
		if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
			border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
		}
		accent := lipgloss.Color("#1ECFA5")
		if m.lightTheme {
			accent = lipgloss.Color("#087E68")
		}
		dialog := lipgloss.NewStyle().Border(border).BorderForeground(accent).Padding(1, 2).Width(max(40, min(76, m.width-4))).Render(content)
		return lipgloss.Place(max(40, m.width), max(12, m.height), lipgloss.Center, lipgloss.Center, dialog)
	}
	return view
}

func (m *interactiveModel) scrollDialog(msg tea.KeyMsg) bool {
	if m.width >= 40 && m.height >= 16 {
		return false
	}
	switch msg.String() {
	case "up", "pgup":
		m.dialogScroll = max(0, m.dialogScroll-1)
		return true
	case "down", "pgdown":
		m.dialogScroll++
		return true
	}
	return false
}

func (m interactiveModel) smallDialog(content string) string {
	w, h := max(4, m.width), max(4, m.height)
	wrapped := lipgloss.NewStyle().Width(max(1, w-4)).Render(content)
	lines := strings.Split(wrapped, "\n")
	visible := max(1, h-3)
	start := min(m.dialogScroll, max(0, len(lines)-visible))
	end := min(len(lines), start+visible)
	text := strings.Join(lines[start:end], "\n") + "\n" + "Up/Down: scroll"
	border := lipgloss.NormalBorder()
	if os.Getenv("TERM") == "dumb" || os.Getenv("NO_COLOR") != "" {
		border = lipgloss.Border{Top: "-", Bottom: "-", Left: "|", Right: "|", TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+"}
	}
	return lipgloss.NewStyle().Border(border).Width(w - 2).MaxHeight(h).Render(text)
}

func (m interactiveModel) trustView() string {
	if m.trustDenied {
		content := "Configuração encerrada\n\nEsta pasta continua sem confiança. O GasaCLI não acessou arquivos do projeto.\n\nPara revisar a confiança depois, execute `gasa setup`.\n\nVocê pode fechar este terminal."
		if m.width < 40 || m.height < 16 {
			return m.smallDialog(content)
		}
		return lipgloss.Place(max(40, m.width), max(12, m.height), lipgloss.Center, lipgloss.Center, lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1, 2).Render(content))
	}
	yes, no := "  Confiar nesta pasta", "  Manter sem confiança"
	if m.trustYes {
		yes = "> Confiar nesta pasta"
	} else {
		no = "> Manter sem confiança"
	}
	content := "Confirme a confiança do workspace\n\nA confiança permite descobrir ferramentas e acessar arquivos do projeto quando você solicitar. Ela não aprova comandos, instalações ou acesso à rede.\n\nPasta:\n" + m.project + "\n\n" + yes + "\n" + no + "\n\nUse ←/→ ou y/n e pressione Enter. Padrão: manter sem confiança."
	if m.width < 40 || m.height < 16 {
		return m.smallDialog(content)
	}
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
	accent, muted := lipgloss.Color("#52D9E8"), lipgloss.Color("#9CB0C4")
	if m.lightTheme {
		accent, muted = lipgloss.Color("#087E86"), lipgloss.Color("#475569")
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(accent)
	secondary := lipgloss.NewStyle().Foreground(muted)
	fmt.Fprintf(&b, "%s\n", title.Render("CONVERSA"))
	fmt.Fprintf(&b, "%s\n", secondary.Render(i18n.T("tui.project", filepath.Base(m.project))+"  |  "+i18n.T("tui.trust.label")+": "+i18n.T("tui.trust.granted")))
	fmt.Fprintf(&b, "%s\n\n", secondary.Render(i18n.T("tui.agents.ready", m.agentsReadyLabel())))
	b.WriteString(title.Render("Comece por aqui") + "\n")
	for _, item := range [][2]string{
		{"/sessions", "listar conversas deste projeto"},
		{"/use <id>", "selecionar uma conversa para o chat"},
		{"/goal", i18n.T("tui.goal.tip")},
		{"/agents", "conferir ferramentas instaladas"},
		{"/doctor", "executar diagnóstico local"},
		{"/help", "ver comandos e atalhos"},
	} {
		b.WriteString(title.Render(fmt.Sprintf("%-12s", item[0])) + secondary.Render(item[1]) + "\n")
	}
	b.WriteString("\n" + secondary.Render("Enter envia  ·  Alt+Enter insere uma linha") + "\n")
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
