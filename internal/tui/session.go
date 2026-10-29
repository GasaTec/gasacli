// Package tui provides the initial line-oriented chat shell.
package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/trust"
)

type Session struct {
	Version          string
	Context          context.Context
	In               io.Reader
	Out              io.Writer
	ProjectPath      string
	TrustStore       *trust.Store
	Agents           func(io.Writer)
	Doctor           func(io.Writer)
	AgentsContext    func(context.Context, io.Writer)
	DoctorContext    func(context.Context, io.Writer)
	OnTrusted        func() error
	Web              func(io.Writer, []string) error
	Setup            func(out io.Writer) error
	Interactive      bool
	Dashboard        string
	DashboardState   string
	ReadyAgents      int
	DiscoverReady    func() int
	Language         string
	ConfigHome       string
	Storage          any
	DiscoveryResults func() any
	Notify           func(string)
	sendMessage      func(any)
}

func (s *Session) Run() error {
	i18n.Configure(s.Language)
	if s.In == nil || s.Out == nil || s.TrustStore == nil {
		return fmt.Errorf("session requires input, output and trust store")
	}
	if s.Interactive {
		return s.runInteractive()
	}
	project, err := trust.Canonical(s.ProjectPath)
	if err != nil {
		return fmt.Errorf("resolve project path: %w", err)
	}
	trusted, err := s.TrustStore.IsTrusted(project)
	if err != nil {
		return fmt.Errorf("check project trust: %w", err)
	}
	scanner := bufio.NewScanner(s.In)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	if !trusted {
		if _, err := fmt.Fprintf(s.Out, "Trust this project directory \"%s\"? [y/N]: ", project); err != nil {
			return fmt.Errorf("write trust prompt: %w", err)
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				if errors.Is(err, bufio.ErrTooLong) {
					return fmt.Errorf("trust answer exceeds 1 MiB input limit")
				}
				return fmt.Errorf("read trust answer: %w", err)
			}
			return fmt.Errorf("project not trusted")
		}
		if strings.ToLower(strings.TrimSpace(scanner.Text())) != "y" {
			return fmt.Errorf("project not trusted")
		}
		if err := s.TrustStore.Grant(project); err != nil {
			return fmt.Errorf("grant project trust: %w", err)
		}
	}
	if s.OnTrusted != nil {
		if err := s.OnTrusted(); err != nil {
			return err
		}
	}
	for {
		if _, err := fmt.Fprint(s.Out, "gasa> "); err != nil {
			return fmt.Errorf("write chat prompt: %w", err)
		}
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		var response string
		if strings.HasPrefix(input, "/") {
			parent := s.Context
			if parent == nil {
				parent = context.Background()
			}
			commandCtx, cancel := context.WithCancel(parent)
			response, err = executeCommand(commandCtx, s, project, true, input, func(chunk string) {
				_, _ = fmt.Fprintln(s.Out, chunk)
			}, func(ctx context.Context, request PermissionRequest) (ApprovalChoice, error) {
				if _, writeErr := fmt.Fprint(s.Out, i18n.T("goal.cli.approval", request.Action, request.Target)); writeErr != nil {
					return ApprovalDeny, writeErr
				}
				if !scanner.Scan() {
					if scanErr := scanner.Err(); scanErr != nil {
						return ApprovalDeny, scanErr
					}
					return ApprovalDeny, nil
				}
				switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
				case "o", "u", "y":
					return ApprovalOnce, nil
				case "r", "e":
					return ApprovalRun, nil
				default:
					return ApprovalDeny, nil
				}
			})
			cancel()
			if errors.Is(err, errExitCommand) {
				return nil
			}
			if err != nil {
				response = err.Error()
			}
		} else {
			response = i18n.T("command.model_unavailable")
		}
		if response != "" {
			if _, err := fmt.Fprintln(s.Out, response); err != nil {
				return fmt.Errorf("write chat response: %w", err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return fmt.Errorf("chat input exceeds 1 MiB input limit")
		}
		return fmt.Errorf("read chat input: %w", err)
	}
	return nil
}
