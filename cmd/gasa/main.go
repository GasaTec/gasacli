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
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/discovery"
	"github.com/GasaTec/gasacli/internal/trust"
	"github.com/GasaTec/gasacli/internal/tui"
	"github.com/GasaTec/gasacli/internal/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gasa:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("gasa", flag.ContinueOnError)
	noBrowser := flags.Bool("no-browser", false, "Do not open the dashboard automatically")
	flags.Usage = help
	// Accept the global switch on either side of a subcommand.
	var positional []string
	for _, arg := range args {
		if arg == "--no-browser" {
			*noBrowser = true
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
	go func() { _, err := io.Copy(writer, stdin); _ = writer.CloseWithError(err) }()
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
	}
	err = session.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
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
  gasa [--no-browser]     Open terminal shell and local dashboard
  gasa doctor            Discover installed coding CLIs
  gasa web open          Start dashboard and wait for Ctrl+C
  gasa help | --help     Show this help
  --no-browser           Suppress browser launches, including /web open
  web.mode auto          Open browser at startup
  web.mode manual        Open browser only with /web open
  web.mode always        Background service not implemented; uses auto with warning
  /help /exit /quit /agents /doctor /status /goal /web [open|status]
  /permissions /backup /restore /web stop|restart: planejado (not implemented)`)
}
