package sessions

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/adapters"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

const (
	defaultSendTimeout = 2 * time.Minute
	maxSendText        = 32 << 10
	maxSendOutput      = 1 << 20
	maxSendLine        = 256 << 10
	sendRateLimit      = 2 * time.Second
)

var providerSessionID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// Resumable checks the provider identifier shape, not trust or permissions.
func Resumable(item Session) bool {
	return (item.Agent == "Claude Code" || item.Agent == "Codex") && providerSessionID.MatchString(item.ProviderID)
}

type approvalContextKey struct{}
type auditedApprovalContextKey struct{}
type expectedProjectContextKey struct{}

// WithExpectedProject binds a UI selection to the project it displayed.
// The sender reindexes sessions before launch, so it must verify that the
// selected session has not moved to a different trusted project in between.
func WithExpectedProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, expectedProjectContextKey{}, project)
}

// WithApproval marks a request as approved by the local dashboard or TUI.
func WithApproval(ctx context.Context) context.Context {
	return context.WithValue(ctx, approvalContextKey{}, true)
}

// WithAuditedApproval marks a UI approval that has already been appended to
// the local approval log.
func WithAuditedApproval(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditedApprovalContextKey{}, true)
}

// CLISender resumes only indexed sessions whose working directory is trusted.
// Approval is supplied by the local UI; a missing approver fails closed.
type CLISender struct {
	Roots       Roots
	Policy      *permissions.Engine
	Approver    permissions.Approver
	LookPath    func(string) (string, error)
	Command     func(context.Context, string, ...string) *exec.Cmd
	Timeout     time.Duration
	OutputLimit int
	Now         func() time.Time
}

type sessionSendState struct {
	mu     sync.Mutex
	active bool
	last   time.Time
}

var sessionSendStates sync.Map

func NewCLISender(roots Roots, policy *permissions.Engine, approver permissions.Approver) *CLISender {
	return &CLISender{Roots: roots, Policy: policy, Approver: approver}
}

func (s *CLISender) Capabilities() Capabilities {
	return Capabilities{Send: true, Agents: []string{"Claude Code", "Codex"}}
}

func (s *CLISender) Send(ctx context.Context, request SendRequest) (<-chan Event, error) {
	if request.SessionID == "" || strings.TrimSpace(request.Text) == "" || len(request.Text) > maxSendText {
		return nil, errors.New("message or session is invalid")
	}
	if s.Policy == nil {
		return nil, errors.New("permission policy is unavailable")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	stateValue, _ := sessionSendStates.LoadOrStore(request.SessionID, &sessionSendState{})
	state := stateValue.(*sessionSendState)
	state.mu.Lock()
	if state.active {
		state.mu.Unlock()
		return nil, errors.New("a message is already being sent to this session")
	}
	if previous := state.last; !previous.IsZero() && now().Sub(previous) < sendRateLimit {
		state.mu.Unlock()
		return nil, errors.New("messages are being sent too quickly")
	}
	state.active = true
	state.mu.Unlock()
	release := func() {
		state.mu.Lock()
		state.active = false
		state.last = now()
		state.mu.Unlock()
	}
	streamStarted := false
	defer func() {
		if !streamStarted {
			release()
		}
	}()

	var selected Session
	indexedSessions, err := ScanMetadataContext(ctx, s.Roots)
	if err != nil {
		return nil, err
	}
	for _, indexed := range indexedSessions {
		if indexed.ID == request.SessionID {
			selected = indexed
			break
		}
	}
	if selected.ID == "" || !Resumable(selected) {
		return nil, errors.New("session is unknown or expired")
	}
	if s.Policy.Trust == nil {
		return nil, errors.New("project trust cannot be verified")
	}
	root, err := ProjectRootContext(ctx, selected.CWD)
	if err != nil {
		return nil, err
	}
	if root == "" {
		return nil, errors.New("session project cannot be verified")
	}
	trusted, err := s.Policy.Trust.IsTrusted(root)
	if err != nil || !trusted {
		return nil, errors.New("session project is not trusted")
	}
	root, err = trust.Canonical(root)
	if err != nil {
		return nil, errors.New("session project cannot be verified")
	}
	if expected, ok := ctx.Value(expectedProjectContextKey{}).(string); ok {
		canonical, err := trust.Canonical(expected)
		if err != nil || canonical != root {
			return nil, errors.New("session project changed since selection")
		}
	}
	cwd, err := trust.Canonical(selected.CWD)
	if err != nil || !withinProject(root, cwd) {
		return nil, errors.New("session working directory is outside the trusted project")
	}
	decision := permissions.Decision{Effect: permissions.Allow, Reason: "effective policy"}
	for _, action := range []permissions.Permission{"execute_commands", "network_access"} {
		req := permissions.Request{Agent: agentID(selected.Agent), Action: action, Project: root, Target: cwd}
		current := s.Policy.Decide(ctx, req)
		if current.Effect == permissions.Ask {
			if confirmed, _ := ctx.Value(approvalContextKey{}).(bool); confirmed {
				current.Effect, current.Reason = permissions.Allow, "user decision"
			} else if s.Approver == nil {
				return nil, errors.New("sending a message requires approval")
			} else {
				current, err = s.Approver.Ask(ctx, permissions.Request{Agent: agentID(selected.Agent), Action: action, Project: root, Target: "Send a message to " + selected.Agent + " session " + selected.ID})
				if err != nil {
					return nil, errors.New("sending a message was not approved")
				}
			}
		}
		if current.Effect != permissions.Allow {
			return nil, errors.New("sending a message is denied by policy")
		}
		if current.Reason == "user decision" {
			decision = current
		}
	}
	if recorded, _ := ctx.Value(auditedApprovalContextKey{}).(bool); !recorded {
		if err := s.audit(selected, root, request.Text, decision); err != nil {
			return nil, errors.New("could not record send approval")
		}
	}

	command, args, err := s.command(selected, request.Text)
	if err != nil {
		return nil, err
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = defaultSendTimeout
	}
	sendCtx, cancel := context.WithTimeout(ctx, timeout)
	defer func() {
		if !streamStarted {
			cancel()
		}
	}()
	commandFactory := s.Command
	if commandFactory == nil {
		commandFactory = exec.CommandContext
	}
	cmd := commandFactory(sendCtx, command, args...)
	cmd.Dir = cwd
	cmd.Env = sendEnvironment(os.Environ())
	cmd.WaitDelay = time.Second
	adapters.ConfigureChildProcess(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.New("could not start agent session")
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, errors.New("could not start agent session")
	}
	if err := cmd.Start(); err != nil {
		cancel()
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("%s CLI is not installed", selected.Agent)
		}
		return nil, errors.New("could not start agent session")
	}
	limit := s.OutputLimit
	if limit <= 0 || limit > maxSendOutput {
		limit = maxSendOutput
	}
	// CommandContext cancels the process; this also kills descendants on each OS.
	done := make(chan struct{})
	go func() {
		select {
		case <-sendCtx.Done():
			_ = adapters.KillProcessTree(cmd)
		case <-done:
		}
	}()
	events := make(chan Event, 8)
	streamStarted = true
	go func() {
		defer cancel()
		defer release()
		defer close(done)
		defer close(events)
		waited := false
		defer func() {
			if !waited {
				_ = cmd.Wait()
			}
		}()
		var stderrCapture cappedBuffer
		stderrDone := make(chan struct{})
		go func() {
			defer close(stderrDone)
			_, _ = io.Copy(&stderrCapture, stderr)
		}()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 32*1024), maxSendLine)
		consumed := 0
		for scanner.Scan() {
			line := scanner.Bytes()
			consumed += len(line) + 1
			if consumed > limit {
				events <- Event{Type: "error", Err: "agent output limit exceeded"}
				_ = adapters.KillProcessTree(cmd)
				return
			}
			var envelope struct {
				Type string `json:"type"`
			}
			if json.Unmarshal(line, &envelope) != nil {
				continue
			}
			if strings.Contains(strings.ToLower(envelope.Type), "error") || strings.Contains(strings.ToLower(envelope.Type), "failed") {
				events <- Event{Type: "error", Err: classifyProviderError(selected.Agent, 1, string(line))}
				_ = adapters.KillProcessTree(cmd)
				return
			}
			if text := adapters.StructuredText(line, envelope.Type); text != "" {
				select {
				case events <- Event{Type: "text", Text: Redact(text)}:
				case <-sendCtx.Done():
					events <- Event{Type: "error", Err: sendError(sendCtx.Err())}
					return
				}
			}
		}
		if err := scanner.Err(); err != nil {
			_ = adapters.KillProcessTree(cmd)
			events <- Event{Type: "error", Err: "agent output could not be read"}
			return
		}
		waitErr := cmd.Wait()
		waited = true
		<-stderrDone
		if waitErr != nil {
			if sendCtx.Err() != nil {
				events <- Event{Type: "error", Err: sendError(sendCtx.Err())}
			} else if exit, ok := waitErr.(*exec.ExitError); ok {
				events <- Event{Type: "error", Err: classifyProviderError(selected.Agent, exit.ExitCode(), stderrCapture.String())}
			} else {
				events <- Event{Type: "error", Err: "agent session failed"}
			}
			return
		}
		events <- Event{Type: "done"}
	}()
	return events, nil
}

func (s *CLISender) command(session Session, text string) (string, []string, error) {
	name := ""
	var args []string
	switch strings.ToLower(session.Agent) {
	case "claude code":
		name = "claude"
		args = []string{"-p", text, "--resume", session.ProviderID, "--output-format", "stream-json", "--include-partial-messages", "--verbose"}
	case "codex":
		name = "codex"
		args = []string{"exec", "resume", session.ProviderID, text, "--json"}
	default:
		return "", nil, errors.New("sending is not supported for this agent")
	}
	lookPath := s.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	path, err := lookPath(name)
	if err != nil {
		return "", nil, fmt.Errorf("%s CLI is not installed", session.Agent)
	}
	return path, args, nil
}

func (s *CLISender) audit(session Session, project, text string, decision permissions.Decision) error {
	hash := sha256.Sum256([]byte(text))
	event := permissions.ApprovalEvent{Decision: permissions.Allow, Permission: "execute_commands", PolicyRevision: "current"}
	event.Actor.Kind = "policy"
	if decision.Reason == "user decision" {
		event.Actor.Kind = "user"
	}
	event.Requester.AgentID = agentID(session.Agent)
	event.Scope.Kind, event.Scope.ID = "project", project
	event.Operation.Summary = "Send message to " + session.Agent + " session " + session.ID
	event.Operation.Digest = hex.EncodeToString(hash[:])
	return s.Policy.RecordApproval(event)
}

func agentID(name string) string {
	if strings.EqualFold(name, "Claude Code") {
		return "claude"
	}
	if strings.EqualFold(name, "Codex") {
		return "codex"
	}
	return "unsupported"
}

func sendEnvironment(environment []string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "SYSTEMROOT": true, "WINDIR": true, "HOMEDRIVE": true, "HOMEPATH": true, "TMP": true, "TEMP": true, "CODEX_HOME": true}
	out := make([]string, 0, len(allowed))
	for _, item := range environment {
		name, _, ok := strings.Cut(item, "=")
		if ok && allowed[strings.ToUpper(name)] {
			out = append(out, item)
		}
	}
	return out
}

func sendError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "sending the message timed out"
	}
	return "sending the message was cancelled"
}

func classifyProviderError(agent string, _ int, diagnostic string) string {
	diagnostic = strings.ToLower(diagnostic)
	switch {
	case strings.Contains(diagnostic, "not logged in"), strings.Contains(diagnostic, "login required"), strings.Contains(diagnostic, "authentication failed"), strings.Contains(diagnostic, "unauthorized"):
		return agent + " is not signed in"
	case strings.Contains(diagnostic, "session not found"), strings.Contains(diagnostic, "unknown session"), strings.Contains(diagnostic, "no session"), strings.Contains(diagnostic, "session expired"):
		return "session is unknown or expired"
	default:
		return "agent session failed; check that the CLI is signed in and the session is still available"
	}
}

type cappedBuffer struct {
	mu sync.Mutex
	b  []byte
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(value)
	if len(b.b) < 64<<10 {
		remaining := (64 << 10) - len(b.b)
		if len(value) > remaining {
			value = value[:remaining]
		}
		b.b = append(b.b, value...)
	}
	return original, nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.b)
}

func withinProject(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
