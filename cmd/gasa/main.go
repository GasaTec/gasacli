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
			if i == index || arg == "--no-browser" || arg == "--no-onboarding" || arg == "--plain" {
				continue
			}
			remaining = append(remaining, arg)
		}
		if command == "reset" {
			return runReset(ctx, remaining, os.Stdin, os.Stdout, os.Stderr)
		}
		return runUninstall(ctx, remaining, os.Stdin, os.Stdout, os.Stderr)
	}
	if index, command := firstLocalStateCommand(args); index >= 0 {
		remaining := make([]string, 0, len(args)-1)
		for i, arg := range args {
			if i == index || arg == "--no-browser" || arg == "--no-onboarding" || arg == "--plain" {
				continue
			}
			remaining = append(remaining, arg)
		}
		return runBackup(ctx, append([]string{command}, remaining...), os.Stdout)
	}
	configureCLILanguage()
	flags := flag.NewFlagSet("gasa", flag.ContinueOnError)
	noBrowser := flags.Bool("no-browser", false, "Do not open the dashboard automatically")
	noOnboarding := flags.Bool("no-onboarding", false, "Skip first-run onboarding")
	plain := flags.Bool("plain", false, "Use line-oriented mode instead of the interactive terminal UI")
	doctorJSON := flags.Bool("json", false, "Write doctor results as JSON")
	flags.Usage = help
	// Accept the global switch on either side of a subcommand.
	var positional []string
	for _, arg := range args {
		if arg == "--no-browser" {
			*noBrowser = true
		} else if arg == "--no-onboarding" {
			*noOnboarding = true
		} else if arg == "--plain" {
			*plain = true
		} else if arg == "--json" {
			*doctorJSON = true
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
	headless := headlessMode()
	rest := flags.Args()
	if len(rest) == 1 && rest[0] == "help" {
		localizedHelp()
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
		case rest[0] == "config":
			return runConfig(ctx, rest[1:], os.Stdout)
		case len(rest) == 1 && rest[0] == "setup":
			interactive := !*plain && !headless && onboarding.Interactive(os.Stdin, os.Stdout)
			if interactive {
				home, err := config.Home()
				if err != nil {
					return err
				}
				cwd, err := os.Getwd()
				if err != nil {
					return err
				}
				return onboarding.RunWizard(ctx, onboarding.WizardOptions{Home: home, Project: cwd, Out: os.Stdout, GrantTrust: trust.NewStore(home).Grant})
			}
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
			return diagnostics(ctx, os.Stdout, *doctorJSON)
		case rest[0] == "permissions":
			return runPermissions(ctx, rest[1:], os.Stdout)
		case rest[0] == "skills":
			return runSkills(ctx, rest[1:], os.Stdout)
		case rest[0] == "brain":
			return runBrain(ctx, rest[1:], os.Stdout)
		case rest[0] == "github":
			return runGitHub(ctx, rest[1:], os.Stdin, os.Stdout)
		case rest[0] == "run":
			return runOrchestration(ctx, rest[1:])
		case rest[0] == "runs":
			return runRuns(rest[1:])
		case rest[0] == "prompts":
			return runPrompts(ctx, rest[1:], os.Stdout)
		case rest[0] == "resume":
			return runResume(ctx, rest[1:])
		case len(rest) == 2 && rest[0] == "web" && rest[1] == "open":
			server, err := startDashboard(ctx, cfg)
			if err != nil {
				return err
			}
			defer shutdown(server)
			if shouldOpen("auto", *noBrowser || headless) {
				launchBrowser(server.LaunchURL(), os.Stderr)
			}
			fmt.Println("Press Ctrl+C to stop")
			<-ctx.Done()
			return nil
		default:
			return fmt.Errorf("unknown command %q (try gasa --help)", strings.Join(rest, " "))
		}
	}
	interactiveTerminal := !*plain && !headless && onboarding.Interactive(os.Stdin, os.Stdout)
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if len(rest) == 0 && !*noOnboarding && os.Getenv("GASA_SKIP_ONBOARDING") != "1" && interactiveTerminal {
		home, err := config.Home()
		if err != nil {
			return err
		}
		state, _ := loadStartupState(home, os.Stderr)
		if !state.Onboarding.Completed || state.Onboarding.Version < 2 {
			if err := onboarding.RunWizard(ctx, onboarding.WizardOptions{Home: home, Project: cwd, Out: os.Stdout, Resume: state.Onboarding.Version > 0 && !state.Onboarding.Completed, GrantTrust: trust.NewStore(home).Grant}); err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			cfg, err = config.Load()
			if err != nil {
				return fmt.Errorf("reload config after onboarding: %w", err)
			}
		}
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	var server *web.Server
	defer func() { shutdown(server) }()
	var sessionInput io.Reader = os.Stdin
	if !interactiveTerminal {
		input, writer := io.Pipe()
		defer input.Close()
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				_ = writer.CloseWithError(ctx.Err())
			case <-done:
				_ = writer.Close()
			}
		}()
		go func() {
			_, copyErr := io.Copy(writer, os.Stdin)
			if copyErr != nil {
				_ = writer.CloseWithError(copyErr)
			} else {
				_ = writer.Close()
			}
		}()
		sessionInput = input
	}
	language := ""
	if state, stateErr := config.LoadState(home); stateErr == nil {
		language = state.Onboarding.Language
	}
	if cfg.Language != "" {
		language = cfg.Language
	}
	if envLanguage := os.Getenv("GASACLI_LANG"); envLanguage != "" {
		language = envLanguage
	}
	var session tui.Session
	session = tui.Session{
		Context: ctx, In: sessionInput, Out: os.Stdout, ProjectPath: cwd, TrustStore: trust.NewStore(home),
		Interactive: interactiveTerminal,
		Language:    language,
		ConfigHome:  home,
		DiscoveryResults: func() any {
			return discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
		},
		DashboardState: "starting",
		DiscoverReady: func() int {
			ready := 0
			for _, result := range discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{}) {
				if result.Status == discovery.Ready {
					ready++
				}
			}
			return ready
		},
		Agents: func(out io.Writer) {
			if err := diagnostics(ctx, out, false); err != nil {
				fmt.Fprintln(os.Stderr, "discovery:", err)
			}
		},
		Doctor: func(out io.Writer) {
			if err := diagnostics(ctx, out, false); err != nil {
				fmt.Fprintln(os.Stderr, "discovery:", err)
			}
		},
		AgentsContext: func(commandCtx context.Context, out io.Writer) {
			if err := diagnostics(commandCtx, out, false); err != nil {
				fmt.Fprintln(os.Stderr, "discovery:", err)
			}
		},
		DoctorContext: func(commandCtx context.Context, out io.Writer) {
			if err := diagnostics(commandCtx, out, false); err != nil {
				fmt.Fprintln(os.Stderr, "discovery:", err)
			}
		},
		OnTrusted: func() error {
			var err error
			if interactiveTerminal {
				server, err = startDashboardQuiet(ctx, cfg)
			} else {
				server, err = startDashboard(ctx, cfg)
			}
			if err != nil {
				session.DashboardState = "unavailable"
				fmt.Fprintln(os.Stderr, "Warning: dashboard unavailable:", err)
				return nil
			}
			session.Dashboard = server.URL()
			session.DashboardState = "running"
			if shouldOpen(cfg.Web.Mode, *noBrowser || headless) {
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
				return fmt.Errorf("%s is planned; see issue #22", action)
			}
			if server == nil {
				return fmt.Errorf("dashboard unavailable")
			}
			fmt.Fprintln(out, "Dashboard:", server.URL())
			if action == "open" && shouldOpen("auto", *noBrowser || headless) {
				launchBrowser(server.LaunchURL(), out)
			}
			return nil
		},
		Setup: func(out io.Writer) error {
			if interactiveTerminal {
				if err := onboarding.RunWizard(ctx, onboarding.WizardOptions{Home: home, Project: cwd, Out: out, GrantTrust: trust.NewStore(home).Grant}); err != nil {
					return err
				}
				cfg, err = config.Load()
				if err != nil {
					return fmt.Errorf("reload config after setup: %w", err)
				}
				return nil
			}
			return onboarding.Run(ctx, onboarding.Options{In: strings.NewReader(""), Out: out, Interactive: false, PlanOnly: true})
		},
	}
	err = session.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func firstLocalStateCommand(args []string) (int, string) {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		switch arg {
		case "backup", "restore", "backups":
			return i, arg
		}
		return -1, ""
	}
	return -1, ""
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
		if arg == "--no-browser" || arg == "--no-onboarding" || arg == "--plain" {
			continue
		}
		if arg == "reset" || arg == "uninstall" {
			return i, arg
		}
		return -1, ""
	}
	return -1, ""
}

func printVersion(out io.Writer) {
	fmt.Fprintf(out, "gasa %s (%s, %s, %s/%s)\n", version, commit, date, runtime.GOOS, runtime.GOARCH)
}

func shouldOpen(mode string, noBrowser bool) bool { return !noBrowser && mode != "manual" }

func diagnostics(ctx context.Context, out io.Writer, jsonOutput bool) error {
	results := discovery.Discover(ctx, discovery.DefaultCatalog(), discovery.Deps{})
	if jsonOutput {
		return discovery.FormatJSON(out, results)
	}
	return formatDiagnosticOutput(out, results)
}

func startDashboard(ctx context.Context, cfg config.Config) (*web.Server, error) {
	return startDashboardWithOutput(ctx, cfg, os.Stdout)
}

func startDashboardQuiet(ctx context.Context, cfg config.Config) (*web.Server, error) {
	return startDashboardWithOutput(ctx, cfg, io.Discard)
}

func startDashboardWithOutput(ctx context.Context, cfg config.Config, out io.Writer) (*web.Server, error) {
	home, err := config.Home()
	if err != nil {
		return nil, fmt.Errorf("resolve dashboard data home: %w", err)
	}
	server, err := web.Start(ctx, web.Options{Host: cfg.Web.Host, Port: cfg.Web.Port, Fallback: cfg.Web.FallbackPort, Home: home})
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(out, "Dashboard:", server.URL())
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
	configureCLILanguage()
	localizedHelp()
}
