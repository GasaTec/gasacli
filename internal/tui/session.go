// Package tui provides the initial line-oriented chat shell.
package tui

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/GasaTec/gasacli/internal/trust"
)

type Session struct {
	Context        context.Context
	In             io.Reader
	Out            io.Writer
	ProjectPath    string
	TrustStore     *trust.Store
	Agents         func(io.Writer)
	Doctor         func(io.Writer)
	OnTrusted      func() error
	Web            func(io.Writer, []string) error
	Setup          func(out io.Writer) error
	Interactive    bool
	Dashboard      string
	DashboardState string
	ReadyAgents    int
}

func (s *Session) Run() error {
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
		fields := strings.Fields(input)
		var response string
		switch fields[0] {
		case "/exit", "/quit":
			return nil
		case "/help":
			response = help
		case "/agents":
			if s.Agents != nil {
				s.Agents(s.Out)
			} else {
				response = "Agent discovery callback is not configured."
			}
		case "/doctor":
			if s.Doctor != nil {
				s.Doctor(s.Out)
			} else {
				response = "Doctor callback is not configured."
			}
		case "/setup":
			if s.Setup != nil {
				if err := s.Setup(s.Out); err != nil {
					response = fmt.Sprintf("/setup: %v", err)
				}
			} else {
				response = "Setup callback is not configured."
			}
		case "/status":
			response = "sem run ativo; orquestração não implementada"
		case "/web":
			if s.Web != nil {
				if err := s.Web(s.Out, fields[1:]); err != nil {
					response = fmt.Sprintf("/web: %v", err)
				}
			} else {
				response = "Dashboard callback is not configured."
			}
		case "/goal":
			response = "/goal: planejado; not implemented yet."
		default:
			if strings.HasPrefix(input, "/") {
				response = "Unknown command. Use /help."
			} else {
				response = "Interactive model chat is not connected yet. Use /help."
			}
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

const help = `GasaCLI commands:
  /help       Show this help
  /exit       Exit the session
  /quit       Exit the session
  /agents     Discover installed agents
  /doctor     Run diagnostics
  /setup      Reopen agent and app onboarding
  /status     Show session status
  /goal       planejado (not implemented)
  /permissions planejado (not implemented)
  /backup     planejado (not implemented)
  /restore    planejado (not implemented)
  /web [open|status] Show dashboard URL or open browser
  /web stop|restart planejado (not implemented)`
