// Package adapters runs installed coding CLIs with bounded output and explicit permissions.
package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GasaTec/gasacli/internal/permissions"
)

const defaultOutputLimit = 1 << 20
const defaultTimeout = 15 * time.Minute

type Request struct {
	Agent       string
	Project     string
	Prompt      string
	RunID       string
	Profile     string
	Model       string
	Timeout     time.Duration
	Environment map[string]string
	ReadOnly    bool
}
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}
type Result struct {
	ExitCode      int           `json:"exit_code"`
	Events        int           `json:"events"`
	OutputBytes   int           `json:"output_bytes"`
	Duration      time.Duration `json:"duration"`
	CostUSD       float64       `json:"cost_usd,omitempty"`
	CostKnown     bool          `json:"cost_known"`
	InputTokens   int64         `json:"input_tokens,omitempty"`
	OutputTokens  int64         `json:"output_tokens,omitempty"`
	Text          string        `json:"-"`
	TextTruncated bool          `json:"-"`
}
type Executor struct {
	Policy      *permissions.Engine
	Approver    permissions.Approver
	OutputLimit int
	OnEvent     func(Event)
	LookPath    func(string) (string, error)
}

func (e *Executor) Execute(ctx context.Context, request Request) (result Result, err error) {
	startedAt := time.Now()
	defer func() { result.Duration = time.Since(startedAt) }()
	if e.Policy == nil {
		return Result{}, errors.New("adapter: permission engine is required")
	}
	if request.Project == "" || request.Prompt == "" {
		return Result{}, errors.New("adapter: project and prompt are required")
	}
	project, err := filepath.Abs(request.Project)
	if err != nil {
		return Result{}, err
	}
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		return Result{}, errors.New("adapter: project must be an existing directory")
	}
	profile, err := e.Policy.Profile()
	if err != nil {
		return Result{}, fmt.Errorf("adapter: load permission profile: %w", err)
	}
	request.Profile = profile
	command, args, err := commandFor(request)
	if err != nil {
		return Result{}, err
	}
	if strings.EqualFold(request.Agent, "codex") && request.ReadOnly {
		var schemaPath string
		args, schemaPath, err = addReviewSchema(args)
		if err != nil {
			return Result{}, fmt.Errorf("adapter: prepare reviewer output schema: %w", err)
		}
		defer os.Remove(schemaPath)
	}
	if e.LookPath != nil {
		path, lookErr := e.LookPath(command)
		if lookErr != nil {
			return Result{}, fmt.Errorf("adapter: %s CLI not found", request.Agent)
		}
		command = path
	}
	actions := []permissions.Permission{"read_project"}
	exactCommand := command
	for _, arg := range args {
		exactCommand += " " + strconv.Quote(arg)
	}
	if !request.ReadOnly {
		actions = append(actions, "agent_run", "write_project")
	}
	for _, action := range actions {
		req := permissions.Request{Agent: request.Agent, Action: action, Project: project, Target: project, Command: exactCommand, RunID: request.RunID}
		decision := e.Policy.Decide(ctx, req)
		if decision.Effect == permissions.Ask {
			if e.Approver == nil {
				return Result{}, fmt.Errorf("adapter: permission %s requires approval", action)
			}
			decision, err = e.Approver.Ask(ctx, req)
			if err != nil {
				return Result{}, fmt.Errorf("adapter: permission %s approval: %w", action, err)
			}
		}
		if decision.Effect != permissions.Allow {
			return Result{}, fmt.Errorf("adapter: permission %s denied: %s", action, decision.Reason)
		}
	}
	commandCtx := ctx
	cancel := func() {}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	commandCtx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(commandCtx, command, args...)
	cmd.Dir = project
	cmd.Env = allowedEnvironment(request.Environment)
	cmd.WaitDelay = time.Second
	configureProcessTree(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Result{}, err
	}
	if err = cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("adapter: start %s: %w", request.Agent, err)
	}
	limit := e.OutputLimit
	if limit <= 1 || limit > defaultOutputLimit {
		limit = defaultOutputLimit
	}
	result = Result{ExitCode: -1}
	stdoutLimit, stderrLimit := limit/2, limit-limit/2
	var stderrBytes atomic.Int64
	stderrDone := make(chan struct{})
	stderrOverflow := make(chan struct{}, 1)
	go func() {
		defer close(stderrDone)
		count, _ := io.Copy(io.Discard, io.LimitReader(stderr, int64(stderrLimit)+1))
		stderrBytes.Store(count)
		if count > int64(stderrLimit) {
			stderrOverflow <- struct{}{}
		}
	}()
	go func() {
		select {
		case <-stderrOverflow:
			_ = terminateProcessTree(cmd)
		case <-commandCtx.Done():
		}
	}()
	lines := bufio.NewScanner(stdout)
	lines.Buffer(make([]byte, 32*1024), stdoutLimit)
	for lines.Scan() {
		line := lines.Bytes()
		result.OutputBytes += len(line) + 1
		if result.OutputBytes > stdoutLimit {
			_ = terminateProcessTree(cmd)
			_ = cmd.Wait()
			<-stderrDone
			return result, errors.New("adapter: output limit exceeded")
		}
		if strings.TrimSpace(string(line)) != "" {
			var event Event
			if err := json.Unmarshal(line, &event); err != nil {
				_ = terminateProcessTree(cmd)
				_ = cmd.Wait()
				<-stderrDone
				return result, errors.New("adapter: invalid structured event")
			}
			result.Events++
			captureMetrics(line, &result)
			if text := structuredText(line, event.Type); text != "" {
				if event.Type == "content_block_delta" {
					result.Text += text
				} else {
					result.Text = text
				}
				if len(result.Text) > 64<<10 {
					result.Text = result.Text[:64<<10]
					result.TextTruncated = true
				}
			}
			if e.OnEvent != nil {
				event.Data = nil
				e.OnEvent(event)
			}
		}
		if commandCtx.Err() != nil {
			_ = terminateProcessTree(cmd)
			_ = cmd.Wait()
			<-stderrDone
			return result, commandCtx.Err()
		}
	}
	if scanErr := lines.Err(); scanErr != nil {
		_ = terminateProcessTree(cmd)
		_ = cmd.Wait()
		<-stderrDone
		return result, fmt.Errorf("adapter: read output: %w", scanErr)
	}
	waitErr := cmd.Wait()
	<-stderrDone
	result.OutputBytes += int(stderrBytes.Load())
	if result.OutputBytes > limit {
		return result, errors.New("adapter: output limit exceeded")
	}
	if waitErr != nil {
		if commandCtx.Err() != nil {
			return result, commandCtx.Err()
		}
		if exit, ok := waitErr.(*exec.ExitError); ok {
			result.ExitCode = exit.ExitCode()
		}
		return result, fmt.Errorf("adapter: %s exited with status %d", request.Agent, result.ExitCode)
	}
	result.ExitCode = 0
	return result, nil
}

func commandFor(request Request) (string, []string, error) {
	switch strings.ToLower(request.Agent) {
	case "claude":
		args := []string{"-p", request.Prompt, "--output-format", "stream-json", "--verbose"}
		if request.Model != "" {
			args = append(args, "--model", request.Model)
		}
		if request.ReadOnly {
			args = append(args, "--permission-mode", "plan")
		} else {
			switch request.Profile {
			case "restricted":
				args = append(args, "--permission-mode", "acceptEdits", "--disallowedTools", "Bash", "WebFetch", "WebSearch")
			case "balanced":
				args = append(args, "--permission-mode", "acceptEdits")
			case "autonomous":
				args = append(args, "--permission-mode", "acceptEdits")
			}
		}
		return "claude", args, nil
	case "codex":
		args := []string{"exec", "--json", "--cd", request.Project, "--sandbox", "workspace-write", "--skip-git-repo-check"}
		if request.ReadOnly {
			args = []string{"exec", "--json", "--cd", request.Project, "--sandbox", "read-only", "--skip-git-repo-check"}
		} else if request.Profile == "restricted" {
			args = []string{"exec", "--json", "--cd", request.Project, "--sandbox", "workspace-write", "--skip-git-repo-check"}
		} else if request.Profile == "balanced" {
			args = []string{"exec", "--json", "--cd", request.Project, "--sandbox", "workspace-write", "--skip-git-repo-check"}
		}
		if request.Profile == "autonomous" {
			args = []string{"exec", "--json", "--cd", request.Project, "--sandbox", "danger-full-access", "--skip-git-repo-check"}
		}
		args = append(args, request.Prompt)
		return "codex", args, nil
	case "opencode":
		return "", nil, errors.New("adapter: OpenCode non-interactive event mode is not supported by this build")
	default:
		return "", nil, fmt.Errorf("adapter: unsupported CLI %q", request.Agent)
	}
}

func writeReviewSchema() (string, error) {
	file, err := os.CreateTemp("", "gasacli-review-schema-*.json")
	if err != nil {
		return "", err
	}
	path := file.Name()
	const schema = `{"type":"object","additionalProperties":false,"required":["verdict","findings"],"properties":{"verdict":{"type":"string","enum":["approve","changes_requested"]},"findings":{"type":"array","items":{"type":"string"}}}}`
	if _, err = file.WriteString(schema); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func addReviewSchema(args []string) ([]string, string, error) {
	if len(args) == 0 {
		return nil, "", errors.New("reviewer command is missing its prompt")
	}
	path, err := writeReviewSchema()
	if err != nil {
		return nil, "", err
	}
	prompt := args[len(args)-1]
	args = append(args[:len(args)-1], "--output-schema", path, prompt)
	return args, path, nil
}

func structuredText(line []byte, eventType string) string {
	var event struct {
		Result string `json:"result"`
		Delta  struct {
			Text string `json:"text"`
		} `json:"delta"`
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	if event.Result != "" {
		return event.Result
	}
	if eventType == "content_block_delta" && event.Delta.Text != "" {
		return event.Delta.Text
	}
	if eventType == "item.completed" && event.Item.Type == "agent_message" {
		return event.Item.Text
	}
	return ""
}

func captureMetrics(line []byte, result *Result) {
	var event struct {
		CostUSD      *float64 `json:"cost_usd"`
		TotalCostUSD *float64 `json:"total_cost_usd"`
		Usage        struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(line, &event) != nil {
		return
	}
	if event.CostUSD != nil {
		result.CostUSD, result.CostKnown = *event.CostUSD, true
	} else if event.TotalCostUSD != nil {
		result.CostUSD, result.CostKnown = *event.TotalCostUSD, true
	}
	if event.Usage.InputTokens != 0 {
		result.InputTokens = event.Usage.InputTokens
	}
	if event.Usage.OutputTokens != 0 {
		result.OutputTokens = event.Usage.OutputTokens
	}
}

func allowedEnvironment(extra map[string]string) []string {
	allowed := map[string]bool{"PATH": true, "HOME": true, "USERPROFILE": true, "SystemRoot": true, "WINDIR": true, "PATHEXT": true, "TEMP": true, "TMP": true, "TMPDIR": true, "LOCALAPPDATA": true, "APPDATA": true, "CODEX_HOME": true, "ANTHROPIC_API_KEY": false, "OPENAI_API_KEY": false}
	values := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok && allowed[key] {
			values[key] = value
		}
	}
	for key, value := range extra {
		if allowed[key] && allowed[key] == true {
			values[key] = value
		}
	}
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func outputLimit(limit int) int {
	if limit <= 0 {
		return defaultOutputLimit
	}
	return limit
}
