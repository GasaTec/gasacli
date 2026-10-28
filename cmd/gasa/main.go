package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/onboarding"
	"github.com/GasaTec/gasacli/internal/trust"
	"github.com/GasaTec/gasacli/internal/tui"
	"github.com/GasaTec/gasacli/internal/web"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() { <-ctx.Done(); stop() }()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gasa:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	if len(args) == 1 && (args[0] == "--version" || args[0] == "version") {
		printVersion(os.Stdout)
		return nil
	}
	// Destructive maintenance commands run before config.Ensure/Load and before flag parsing:
	// a broken or missing config must not block them, they must not recreate the files they
	// are about to delete, and they own their flags (--all, --yes, --dry-run, ...).
	if index, command := firstMaintenanceCommand(args); index >= 0 {
		remaining := make([]string, 0, len(args)-1)
		for i, arg := range args {
			if i == index || arg == "--no-browser" || arg == "--no-onboarding" {
				continue
			}
			remaining = append(remaining, arg)
		}
		if command == "reset" {
			return runReset(ctx, remaining, os.Stdin, os.Stdout, os.Stderr)
		}
		return runUninstall(ctx, remaining, os.Stdin, os.Stdout, os.Stderr)
	}
	flags := flag.NewFlagSet("gasa", flag.ContinueOnError)
	noBrowser := flags.Bool("no-browser", false, "Do not open the dashboard automatically")
	noOnboarding := flags.Bool("no-onboarding", false, "Skip first-run onboarding")
	flags.Usage = help
	// Accept the global switch on either side of a subcommand.
	var positional []string
	for _, arg := range args {
		if arg == "--no-browser" {
			*noBrowser = true
		} else if arg == "--no-onboarding" {
			*noOnboarding = true
		} else {
			positional = append(positional, arg)
		}
	}
	if err := flags.Parse(positional); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	rest := flags.Args()
	if len(rest) == 1 && rest[0] == "help" {
		help()
		return nil
	}
	if err := config.Ensure(); err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if cfg.Web.Mode == "always" {
		fmt.Println("always: serviço em background ainda não implementado; usando auto")
	}
	if len(rest) > 0 {
		switch {
		case len(rest) == 1 && rest[0] == "setup":
			interactive := onboarding.Interactive(os.Stdin, os.Stdout)
			if err := onboarding.Run(ctx, onboarding.Options{In: os.Stdin, Out: os.Stdout, InFile: os.Stdin, OutFile: os.Stdout, Interactive: interactive}); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if interactive {
				home, err := config.Home()
				if err != nil {
					return err
				}
				return config.SaveOnboardingState(home, time.Now())
			}
			return nil
		case len(rest) == 1 && rest[0] == "doctor":
			return diagnostics(ctx, os.Stdout)
		case len(rest) == 2 && rest[0] == "web" && rest[1] == "open":
			server, err := startDashboard(ctx, cfg)
			if err != nil {
				return err
			}
			defer shutdown(server)
			if !*noBrowser {
				launchBrowser(server.LaunchURL(), os.Stderr)
			}
			fmt.Println("Press Ctrl+C to stop")
			<-ctx.Done()
			return nil
		default:
			return fmt.Errorf("unknown command %q (try gasa --help)", strings.Join(rest, " "))
		}
	}
	if len(rest) == 0 && !*noOnboarding && os.Getenv("GASA_SKIP_ONBOARDING") != "1" && onboarding.Interactive(os.Stdin, os.Stdout) {
		home, err := config.Home()
		if err != nil {
			return err
		}
		state, stateWritable := loadStartupState(home, os.Stderr)
		if !state.Onboarding.Completed {
			if err := onboarding.Run(ctx, onboarding.Options{In: os.Stdin, Out: os.Stdout, InFile: os.Stdin, OutFile: os.Stdout, Interactive: true}); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if stateWritable {
				if err := config.SaveOnboardingState(home, time.Now()); err != nil {
					return fmt.Errorf("save onboarding state: %w", err)
				}
			}
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	fmt.Println("GasaCLI pre-alpha |", cwd)
	fmt.Println("This build is a local chat shell; /goal is not implemented.")
	var server *web.Server
	defer func() { shutdown(server) }()
	// Closing the pipe on cancellation unblocks the TUI even while stdin is idle.
	input, writer := io.Pipe()
	defer input.Close()
	done := make(chan struct{})
	defer close(done)
	stdin := os.Stdin
	setupBytes := make(chan byte, 256)
	inputMode := make(chan bool, 4)
	rawInput := false
	go func() {
		buffer := []byte{0}
		for {
			n, readErr := stdin.Read(buffer)
			if n > 0 {
				for {
					select {
					case raw := <-inputMode:
						rawInput = raw
					default:
						goto modeReady
					}
				}
			modeReady:
				if rawInput {
					select {
					case setupBytes <- buffer[0]:
					case <-ctx.Done():
						_ = writer.CloseWithError(ctx.Err())
						return
					}
				} else if _, err := writer.Write(buffer[:n]); err != nil {
					return
				}
			}
			if readErr != nil {
				_ = writer.CloseWithError(readErr)
				return
			}
		}
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = writer.CloseWithError(ctx.Err())
		case <-done:
		}
	}()
	session := tui.Session{
		In: input, Out: os.Stdout, ProjectPath: cwd, TrustStore: trust.NewStore(home),
		Agents: func(out io.Writer) {
			if err := diagnostics(ctx, out); err != nil {
				fmt.Fprintln(os.Stderr, "discovery:", err)
			}
		},
		Doctor: func(out io.Writer) {
			if err := diagnostics(ctx, out); err != nil {
				fmt.Fprintln(os.Stderr, "discovery:", err)
			}
		},
		OnTrusted: func() error {
			var err error
			server, err = startDashboard(ctx, cfg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Warning: dashboard unavailable:", err)
				return nil
			}
			if shouldOpen(cfg.Web.Mode, *noBrowser) {
				launchBrowser(server.LaunchURL(), os.Stderr)
			}
			return nil
		},
		Web: func(out io.Writer, args []string) error {
			action := "status"
			if len(args) > 1 {
				return fmt.Errorf("usage: /web [open|status]")
			}
			if len(args) == 1 {
				action = args[0]
			}
			if action != "open" && action != "status" {
				return fmt.Errorf("%s: planejado (not implemented)", action)
			}
			if server == nil {
				return fmt.Errorf("dashboard unavailable")
			}
			fmt.Fprintln(out, "Dashboard:", server.URL())
			if action == "open" && !*noBrowser {
				launchBrowser(server.LaunchURL(), out)
			}
			return nil
		},
		Setup: func(out io.Writer) error {
			interactive := onboarding.Interactive(os.Stdin, os.Stdout)
			if !interactive {
				return onboarding.Run(ctx, onboarding.Options{In: os.Stdin, Out: out, Interactive: false, PlanOnly: true})
			}
			// Inside the chat the console is in line mode with a line read already pending, so single
			// keystrokes would only arrive after Enter. Use the numbered prompt here; `gasa setup`
			// (outside the chat) has the checkbox list with arrows and space.
			select {
			case inputMode <- true:
			case <-ctx.Done():
				return ctx.Err()
			}
			defer func() { inputMode <- false }()
			if err := onboarding.Run(ctx, onboarding.Options{In: channelByteReader{bytes: setupBytes}, Out: out, Interactive: true, Numbered: true, PlanOnly: true}); err != nil {
				return err
			}
			return nil
		},
	}
	err = session.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func loadStartupState(home string, errOut io.Writer) (config.State, bool) {
	state, err := config.LoadState(home)
	if err != nil {
		fmt.Fprintln(errOut, "Warning: could not read onboarding state; treating onboarding as incomplete:", err)
		return config.State{}, false
	}
	return state, true
}

func firstMaintenanceCommand(args []string) (int, string) {
	for i, arg := range args {
		if arg == "--no-browser" || arg == "--no-onboarding" {
			continue
		}
		if arg == "reset" || arg == "uninstall" {
			return i, arg
		}
		return -1, ""
	}
	return -1, ""
}

type channelByteReader struct{ bytes <-chan byte }

func (r channelByteReader) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	value, ok := <-r.bytes
	if !ok {
		return 0, io.EOF
	}
	buffer[0] = value
	return 1, nil
}

func printVersion(out io.Writer) {
	fmt.Fprintf(out, "gasa %s (%s, %s, %s/%s)\n", version, commit, date, runtime.GOOS, runtime.GOARCH)
}

func shouldOpen(mode string, noBrowser bool) bool { return !noBrowser && mode != "manual" }

func diagnostics(ctx context.Context, out io.Writer) error {
	return discovery.Format(out, discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{}))
}

func startDashboard(ctx context.Context, cfg config.Config) (*web.Server, error) {
	server, err := web.Start(ctx, web.Options{Host: cfg.Web.Host, Port: cfg.Web.Port, Fallback: cfg.Web.FallbackPort})
	if err != nil {
		return nil, err
	}
	fmt.Println("Dashboard:", server.URL())
	go func() {
		<-server.Done()
		if err := server.ServeError(); err != nil {
			fmt.Fprintln(os.Stderr, "Warning: dashboard stopped:", err)
		}
	}()
	return server, nil
}

func shutdown(server *web.Server) {
	if server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: dashboard shutdown:", err)
	}
}

func launchBrowser(url string, out io.Writer) {
	if url == "" {
		fmt.Fprintln(out, "Warning: could not create a browser session; open the Dashboard URL manually")
		return
	}
	err := web.OpenBrowser(url, func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
		return nil
	})
	if err != nil {
		fmt.Fprintln(out, "Warning: browser unavailable")
	}
}

func help() {
	fmt.Println(`GasaCLI pre-alpha commands
  gasa --version         Show version and build information
  gasa [--no-browser]     Open terminal shell and local dashboard
  gasa doctor            Discover installed coding CLIs
  gasa setup             Reopen agent and app onboarding
  gasa web open          Start dashboard and wait for Ctrl+C
  gasa reset [--all] [--include-backups] [--dry-run] [--yes]
                         Delete GasaCLI settings, trust and onboarding state (--all: also
                         agents, prompts, skills, plugins, notes and database). Shows a plan first.
  gasa uninstall [--include-backups] [--dry-run] [--yes]
                         Delete all GasaCLI data and print how to remove the binary
  gasa help | --help     Show this help
  --no-browser           Suppress browser launches, including /web open
  --no-onboarding        Skip first-run onboarding
  web.mode auto          Open browser at startup
  web.mode manual        Open browser only with /web open
  web.mode always        Background service not implemented; uses auto with warning
  /help /exit /quit /agents /doctor /setup /status /goal /web [open|status]
  /permissions /backup /restore /web stop|restart: planejado (not implemented)`)
}
