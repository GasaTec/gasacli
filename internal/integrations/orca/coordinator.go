package orca

import (
	"context"
	"errors"
	"strings"
	"time"
)

type LaunchRequest struct{ AgentID, Worktree, Prompt, Model, Effort, RetryLaunch string }

// LaunchCoordinator is a real public agent launch, used only after the product
// persists/authorizes its outbox entry. RetryLaunch must come from the original
// host receipt; timeout/silence never authorizes a fresh launch.
func (b *Bridge) LaunchCoordinator(ctx context.Context, in LaunchRequest) (Receipt, error) {
	if !agentAllowed(in.AgentID) || !validSelector(in.Worktree) || in.Prompt == "" || len(in.Prompt) > 128*1024 || strings.ContainsRune(in.Prompt, 0) {
		return Receipt{}, errors.New("invalid coordinator launch request")
	}
	if in.Effort != "" && in.Model == "" {
		return Receipt{}, errors.New("effort requires a model")
	}
	flags := []string{"--agent", in.AgentID, "--worktree", in.Worktree, "--surface", "terminal", "--prompt", in.Prompt}
	for _, option := range [][2]string{{"--model", in.Model}, {"--effort", in.Effort}, {"--retry-launch", in.RetryLaunch}} {
		if option[1] != "" {
			if !validSelector(option[1]) {
				return Receipt{}, errors.New("invalid launch option")
			}
			flags = append(flags, option[0], option[1])
		}
	}
	return b.publicMutation(ctx, "agent launch", flags, 45*time.Second)
}
func agentAllowed(id string) bool {
	return contains([]string{"claude", "codex", "gemini", "cursor", "opencode", "kimi", "antigravity"}, id)
}

// SendCoordinator submits to the exact terminal from a persisted launch receipt.
// It does not report task completion or claim that input_accepted is turn_started.
// A caller must reconcile an unknown receipt before another fresh submission.
func (b *Bridge) SendCoordinator(ctx context.Context, terminal, text, retryRequest string) (Receipt, error) {
	if !validSelector(terminal) || text == "" || len(text) > 128*1024 || strings.ContainsRune(text, 0) {
		return Receipt{}, errors.New("invalid coordinator submission")
	}
	flags := []string{"--terminal", terminal, "--text", text, "--enter", "--wait-submit", "5"}
	if retryRequest != "" {
		if !validSelector(retryRequest) {
			return Receipt{}, errors.New("invalid retry request")
		}
		flags = append(flags, "--retry-request", retryRequest)
	}
	return b.publicMutation(ctx, "terminal send", flags, 15*time.Second)
}
func (b *Bridge) publicMutation(ctx context.Context, command string, flags []string, timeout time.Duration) (Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.capability.Compatible {
		return Receipt{}, errors.New("runtime contract not verified")
	}
	if command == "agent launch" && !b.capability.CanLaunchCoordinator {
		return Receipt{}, errors.New("runtime lacks durable coordinator launch replay")
	}
	if command == "terminal send" && !b.capability.CanSubmitCoordinator {
		return Receipt{}, errors.New("runtime lacks durable coordinator prompt delivery")
	}
	allowed, ok := b.commands[command]
	if !ok {
		return Receipt{}, errors.New("public mutation absent from runtime schema")
	}
	for index := 0; index < len(flags); index++ {
		flag := strings.TrimPrefix(flags[index], "--")
		if !allowed[flag] {
			return Receipt{}, errors.New("mutation flag absent from public schema")
		}
		if flag != "enter" {
			index++
			if index >= len(flags) {
				return Receipt{}, errors.New("mutation argument missing")
			}
		}
	}
	// Fence before effects. Detecting a changed runtime only in the mutation reply
	// would be too late to prevent submitting work to the replacement runtime.
	status, err := b.run(ctx, []string{"status", "--json"}, 8*time.Second, false)
	if err != nil {
		return status, err
	}
	if status.RuntimeID == "" || status.RuntimeID != b.capability.RuntimeID {
		b.capability.CanObserve = false
		b.capability.Compatible = false
		b.capability.CanExecute = false
		return Receipt{}, errors.New("runtime identity changed; reconcile before mutation")
	}
	args := append(strings.Split(command, " "), flags...)
	args = append(args, "--json")
	receipt, err := b.run(ctx, args, timeout, true)
	if receipt.RuntimeID != "" && receipt.RuntimeID != b.capability.RuntimeID {
		receipt.Unknown = true
		return receipt, errors.New("runtime changed while mutation was submitted; reconcile original receipt")
	}
	return receipt, err
}

// CoordinatorTransport is a trusted server-side dependency, never an HTTP
// payload. Implementations must run in a persistent, genuinely authorized Orca
// coordinator context and prove its binding without spoofed --from/env values.
// The terminal handoff above is the current supported bootstrap; this interface
// also permits a future authenticated coordinator plugin without private RPC.
type CoordinatorTransport interface {
	Binding(context.Context) (CoordinatorBinding, error)
	Execute(context.Context, CoordinatorBinding, Command) (Receipt, error)
}
type CoordinatorBinding struct {
	RuntimeID, RunID, CoordinatorID string
	ExpiresAt                       time.Time
}

func (b *Bridge) BindCoordinator(ctx context.Context, transport CoordinatorTransport) error {
	if transport == nil {
		return ErrCoordinatorAuthority
	}
	binding, err := transport.Binding(ctx)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.capability.Compatible || binding.RuntimeID != b.capability.RuntimeID || !validSelector(binding.RunID) || !validSelector(binding.CoordinatorID) || !binding.ExpiresAt.After(time.Now()) {
		return ErrCoordinatorAuthority
	}
	b.coordinator = transport
	b.binding = binding
	b.capability.CanExecute = true
	b.capability.Reason = ""
	return nil
}
func (b *Bridge) coordinated(ctx context.Context, runID, command string, flags []string) (Receipt, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.coordinator == nil || !b.capability.CanExecute || runID != b.binding.RunID {
		return Receipt{}, ErrCoordinatorAuthority
	}
	proof, err := b.coordinator.Binding(ctx)
	if err != nil {
		return Receipt{}, err
	}
	if proof.RuntimeID != b.binding.RuntimeID || proof.CoordinatorID != b.binding.CoordinatorID || proof.RunID != runID || !proof.ExpiresAt.After(time.Now()) {
		b.capability.CanExecute = false
		return Receipt{}, ErrCoordinatorAuthority
	}
	allowed, ok := b.commands[command]
	if !ok {
		return Receipt{}, errors.New("coordinator command absent from public schema")
	}
	for n := 0; n < len(flags); n += 2 {
		if n+1 >= len(flags) || !allowed[strings.TrimPrefix(flags[n], "--")] {
			return Receipt{}, errors.New("unsupported coordinator command flag")
		}
	}
	args := append(strings.Split(command, " "), flags...)
	args = append(args, "--json")
	receipt, err := b.coordinator.Execute(ctx, proof, Command{Executable: b.executable, Args: args, Dir: b.dir, Timeout: 45 * time.Second})
	if err != nil {
		receipt.Unknown = true
	}
	if receipt.RuntimeID != "" && receipt.RuntimeID != proof.RuntimeID {
		receipt.Unknown = true
		err = errors.New("coordinator runtime changed; reconcile original operation")
	}
	return receipt, err
}
func (b *Bridge) StartWorker(ctx context.Context, in WorkerRequest) (Receipt, error) {
	if !validSelector(in.RunID) || !validSelector(in.TaskID) || !agentAllowed(in.AgentID) || !validSelector(in.Worktree) {
		return Receipt{}, errors.New("invalid worker request")
	}
	flags := []string{"--run", in.RunID, "--task", in.TaskID, "--agent", in.AgentID, "--worktree", in.Worktree, "--timeout-ms", "30000"}
	if in.Model != "" {
		if !validSelector(in.Model) {
			return Receipt{}, errors.New("invalid model")
		}
		flags = append(flags, "--model", in.Model)
	}
	if in.Effort != "" {
		if in.Model == "" || !validSelector(in.Effort) {
			return Receipt{}, errors.New("effort requires a valid model")
		}
		flags = append(flags, "--effort", in.Effort)
	}
	return b.coordinated(ctx, in.RunID, "orchestration worker-start", flags)
}
func (b *Bridge) StopWorker(ctx context.Context, dispatchID string) (Receipt, error) {
	if !validSelector(dispatchID) {
		return Receipt{}, errors.New("invalid dispatch")
	}
	b.mu.Lock()
	runID := b.binding.RunID
	b.mu.Unlock()
	return b.coordinated(ctx, runID, "orchestration worker-stop", []string{"--dispatch", dispatchID})
}
func (b *Bridge) Send(ctx context.Context, recipient, subject, body string) (Receipt, error) {
	if !validSelector(recipient) || (!strings.HasPrefix(recipient, "run:") && !strings.HasPrefix(recipient, "dispatch:")) || subject == "" || len(subject) > 1000 || len(body) > 128*1024 || strings.ContainsRune(subject+body, 0) {
		return Receipt{}, errors.New("invalid explicit mailbox message")
	}
	b.mu.Lock()
	runID := b.binding.RunID
	b.mu.Unlock()
	return b.coordinated(ctx, runID, "orchestration send", []string{"--to", recipient, "--subject", subject, "--body", body, "--type", "question"})
}
