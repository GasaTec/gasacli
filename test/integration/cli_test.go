package integration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCLI(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "../.."))
	binary := filepath.Join(t.TempDir(), "gasa.exe")
	build := exec.Command("go", "build", "-o", binary, "./cmd/gasa")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	newHome := func(t *testing.T) (string, string, string) {
		t.Helper()
		sandbox := t.TempDir()
		home, project := filepath.Join(sandbox, "gasacli"), filepath.Join(sandbox, "project")
		if err := os.MkdirAll(project, 0700); err != nil {
			t.Fatal(err)
		}
		return sandbox, home, project
	}
	run := func(t *testing.T, home, project, input string, code int, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		args = append([]string{"--no-browser"}, args...)
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir, cmd.Stdin = project, strings.NewReader(input)
		// No discovery probe may invoke real user-installed CLIs in integration tests.
		for _, entry := range os.Environ() {
			key, _, _ := strings.Cut(entry, "=")
			switch strings.ToUpper(key) {
			case "GASACLI_HOME", "GASACLI_LANG", "PATH", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME":
				continue
			}
			cmd.Env = append(cmd.Env, entry)
		}
		cmd.Env = append(cmd.Env, "GASACLI_HOME="+home, "GASACLI_LANG=pt-BR", "PATH="+filepath.Join(filepath.Dir(home), "empty-path"), "HOME="+filepath.Join(filepath.Dir(home), "user"), "USERPROFILE="+filepath.Join(filepath.Dir(home), "user"), "APPDATA="+filepath.Join(filepath.Dir(home), "appdata"), "LOCALAPPDATA="+filepath.Join(filepath.Dir(home), "localappdata"), "XDG_CONFIG_HOME="+filepath.Join(filepath.Dir(home), "xdg"))
		output, err := cmd.CombinedOutput()
		if strings.Contains(string(output), "#token=") || strings.Contains(string(output), "#code=") {
			t.Fatal("session credential leaked into terminal output")
		}
		actual := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				actual = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		out := string(output)
		if ctx.Err() != nil || actual != code {
			t.Fatalf("exit=%d want=%d timeout=%v\n%s", actual, code, ctx.Err(), out)
		}
		return out
	}
	if runtime.GOOS == "windows" {
		t.Run("ResetSafetyProofs", func(t *testing.T) {
			_, _, project := newHome(t)
			programFiles := os.Getenv("ProgramFiles")
			if programFiles != "" {
				out := run(t, filepath.Join(programFiles, "DeskIn"), project, "", 1, "reset", "--dry-run")
				if !strings.Contains(out, "refusing data directory") {
					t.Fatalf("Program Files path was not refused: %s", out)
				}
			}
			_, missing, _ := newHome(t)
			if err := os.Mkdir(missing, 0700); err != nil {
				t.Fatal(err)
			}
			out := run(t, missing, project, "", 1, "reset", "--dry-run")
			if !strings.Contains(out, "missing .gasacli marker") {
				t.Fatalf("unmarked home was not refused: %s", out)
			}
			foreign := filepath.Join(filepath.Dir(missing), "foreign")
			if err := os.Mkdir(foreign, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(foreign, "config.yaml"), []byte("web: {}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadDir(foreign)
			out = run(t, foreign, project, "", 1, "--no-browser", "reset", "--dry-run")
			after, _ := os.ReadDir(foreign)
			if len(before) != len(after) || !strings.Contains(out, "missing .gasacli marker") {
				t.Fatalf("pre-dispatch wrote to foreign home: before=%v after=%v out=%s", before, after, out)
			}
			nulHome := filepath.Join(filepath.Dir(missing), "reserved", "NUL")
			out = run(t, nulHome, project, "", 1, "reset", "--dry-run")
			if !strings.Contains(out, "reserved Windows path component") {
				t.Fatalf("reserved component was not refused: %s", out)
			}
		})
	}
	contains := func(t *testing.T, out string, wants ...string) {
		t.Helper()
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q\n%s", want, out)
			}
		}
	}
	t.Run("DoctorRejectsForeignHomeWithoutMutation", func(t *testing.T) {
		for _, marker := range []string{"readme.txt", ".git"} {
			t.Run(marker, func(t *testing.T) {
				_, _, project := newHome(t)
				home := filepath.Join(t.TempDir(), "foreign")
				if err := os.Mkdir(home, 0700); err != nil {
					t.Fatal(err)
				}
				if marker == ".git" {
					if err := os.Mkdir(filepath.Join(home, marker), 0700); err != nil {
						t.Fatal(err)
					}
				} else if err := os.WriteFile(filepath.Join(home, marker), []byte("foreign"), 0600); err != nil {
					t.Fatal(err)
				}
				before := treeHash(t, home)
				out := run(t, home, project, "", 1, "doctor")
				if !strings.Contains(out, "not a GasaCLI data directory") || treeHash(t, home) != before {
					t.Fatalf("foreign home changed or unclear: %s", out)
				}
			})
		}
	})
	t.Run("ResetDryRunRejectsForeignHomeWithoutMutation", func(t *testing.T) {
		_, _, project := newHome(t)
		home := filepath.Join(t.TempDir(), "foreign")
		if err := os.Mkdir(home, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, "readme.txt"), []byte("foreign"), 0600); err != nil {
			t.Fatal(err)
		}
		before := treeHash(t, home)
		out := run(t, home, project, "", 1, "reset", "--dry-run")
		if !strings.Contains(out, "missing .gasacli marker") || treeHash(t, home) != before {
			t.Fatalf("reset dry-run mutated foreign home or was accepted: %s", out)
		}
	})
	writeConfig := func(t *testing.T, home, mode string, fallback bool) {
		t.Helper()
		if err := os.MkdirAll(home, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".gasacli"), []byte(`{"app":"gasacli","schema":1}`), 0600); err != nil {
			t.Fatal(err)
		}
		data := fmt.Sprintf("web:\n  mode: %s\n  host: 127.0.0.1\n  port: 3199\n  fallback_port: %t\n", mode, fallback)
		if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("HelpAndTrustPersistence", func(t *testing.T) {
		t.Setenv("GASACLI_LANG", "pt-BR")
		_, home, project := newHome(t)
		out := run(t, home, project, "1\n/help\n/status\n/goal test\n/web status\n/exit\n", 0)
		contains(t, out, "[1=trust, 2=continue untrusted]", "/help", "/exit", "/agents", "/doctor", "/status", "/goal", "/permissions", "/backup", "/restore", "/web", "/cancel", "/runs", "No orchestration run for this project.", "no ready or installed supported coding CLI is available", "Dashboard:")
		t.Logf("real startup and /help output (public URL only):\n%s", out)
		out = run(t, home, project, "/help\n/exit\n", 0)
		if strings.Contains(out, "[1=trust, 2=continue untrusted]") {
			t.Fatal("trust asked again")
		}
		t.Log("real binary startup, /help, /status, /goal, /web status and persisted trust passed")
	})
	t.Run("PlainFlagKeepsLineMode", func(t *testing.T) {
		_, home, project := newHome(t)
		out := run(t, home, project, "1\n/help\n/exit\n", 0, "--plain")
		contains(t, out, "[1=trust, 2=continue untrusted]", "gasa> ", "/help")
		if strings.Contains(out, "G> GasaCLI") {
			t.Fatalf("--plain started the interactive renderer: %s", out)
		}
	})
	t.Run("TrustRefusedAndEOF", func(t *testing.T) {
		for _, input := range []string{"2\n", ""} {
			_, home, project := newHome(t)
			out := run(t, home, project, input, 0)
			contains(t, out, "This folder remains untrusted")
			if strings.Contains(out, "Dashboard:") || strings.Contains(out, "gasa> ") {
				t.Fatal("started before trust")
			}
		}
	})
	t.Run("Doctor", func(t *testing.T) {
		_, home, project := newHome(t)
		out := run(t, home, project, "", 0, "doctor")
		names := []string{"Claude Code", "Codex", "Gemini CLI", "Cursor CLI", "OpenCode", "Orca", "Kimi Code CLI", "GitHub CLI"}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != len(names) {
			t.Fatalf("doctor lines=%d\n%s", len(lines), out)
		}
		for i, name := range names {
			contains(t, lines[i], name, "CLI não encontrado no PATH")
			if !strings.Contains(lines[i], "ausente") {
				t.Errorf("doctor row for %s has no readiness status: %s", name, lines[i])
			}
		}
		t.Logf("real doctor output:\n%s", out)
	})
	t.Run("OccupiedPortFallback", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:3199")
		if err != nil {
			t.Skipf("cannot reserve 3199 to establish occupied-port precondition: %v", err)
		}
		defer listener.Close()
		_, home, project := newHome(t)
		out := run(t, home, project, "1\n/help\n/exit\n", 0)
		contains(t, out, "Dashboard: http://127.0.0.1:", "/help", "gasa> ")
		if strings.Contains(out, "http://127.0.0.1:3199/") {
			t.Fatal("did not fall back")
		}
	})
	t.Run("FreePreferredPort", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:3199")
		if err != nil {
			t.Skipf("preferred port unavailable in environment: %v", err)
		}
		listener.Close()
		_, home, project := newHome(t)
		out := run(t, home, project, "1\n/exit\n", 0)
		if !strings.Contains(out, "http://127.0.0.1:3199/") {
			if !strings.Contains(out, "Dashboard: http://127.0.0.1:") {
				t.Fatalf("dashboard did not start after successful preferred-port probe:\n%s", out)
			}
			t.Skipf("runtime used fallback after successful availability probe; preferred port may have been acquired concurrently:\n%s", out)
		}
	})
	t.Run("DashboardFailureKeepsChat", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:3199")
		if err != nil {
			t.Skipf("cannot reserve port: %v", err)
		}
		defer listener.Close()
		_, home, project := newHome(t)
		writeConfig(t, home, "auto", false)
		out := run(t, home, project, "1\n/help\n/web status\n/exit\n", 0)
		contains(t, out, "Warning: dashboard unavailable", "/help", "/web: dashboard unavailable")
	})
	t.Run("IsolatedHome", func(t *testing.T) {
		sandbox, home, project := newHome(t)
		run(t, home, project, "1\n/exit\n", 0)
		for _, name := range []string{"config.yaml", "trust.json", "agents", "prompts", "skills", "plugins", "segundo-cerebro", "backups", "logs"} {
			if _, err := os.Stat(filepath.Join(home, name)); err != nil {
				t.Errorf("home entry %s: %v", name, err)
			}
		}
		if err := filepath.WalkDir(sandbox, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path != sandbox && path != project && path != home && !strings.HasPrefix(path, home+string(os.PathSeparator)) {
				t.Errorf("unexpected write outside GASACLI_HOME: %s", path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("OnboardingNeverPromptsOrCompletesInPipe", func(t *testing.T) {
		_, home, project := newHome(t)
		out := run(t, home, project, "1\n/exit\n", 0)
		if strings.Contains(out, "Select what to install") || strings.Contains(out, "Run the automatic installs") {
			t.Fatalf("first-run onboarding ran without TTY: %s", out)
		}
		if _, err := os.Stat(filepath.Join(home, "state.json")); !os.IsNotExist(err) {
			t.Fatalf("non-TTY run wrote state: %v", err)
		}
	})
	t.Run("SetupPrintsManualGuidanceWithoutTTY", func(t *testing.T) {
		_, home, project := newHome(t)
		out := run(t, home, project, "", 0, "setup")
		contains(t, out, "Claude Code", "Cursor CLI", "Obsidian", "Comandos sugeridos", "Comando:", "Origem oficial:")
		if strings.Contains(out, "Executar as instalações automáticas") {
			t.Fatal("non-TTY setup prompted")
		}
		if _, err := os.Stat(filepath.Join(home, "state.json")); !os.IsNotExist(err) {
			t.Fatalf("non-TTY setup wrote state: %v", err)
		}
	})
	t.Run("ChatSetupIsPlanOnly", func(t *testing.T) {
		_, home, project := newHome(t)
		out := run(t, home, project, "1\n/setup\n/exit\n", 0)
		contains(t, out, "Comando:", "Origem oficial:", "Para instalar, rode `gasa setup` fora do chat.")
		if strings.Contains(out, "Run the automatic installs now?") {
			t.Fatalf("chat setup offered execution: %s", out)
		}
		if _, err := os.Stat(filepath.Join(home, "state.json")); !os.IsNotExist(err) {
			t.Fatalf("chat setup wrote completion state: %v", err)
		}
	})
	t.Run("ManualAndAlways", func(t *testing.T) {
		for _, mode := range []string{"manual", "always"} {
			_, home, project := newHome(t)
			writeConfig(t, home, mode, true)
			out := run(t, home, project, "1\n/web open\n/exit\n", 0)
			contains(t, out, "Dashboard:")
			if strings.Contains(out, "browser unavailable") {
				t.Fatal("attempted browser with --no-browser")
			}
			if mode == "always" {
				contains(t, out, "serviço em background ainda não implementado; usando auto")
			}
		}
	})
	t.Run("ConfigFailure", func(t *testing.T) {
		_, home, project := newHome(t)
		writeConfig(t, home, "invalid", true)
		contains(t, run(t, home, project, "1\n", 1), "load config", "invalid web.mode")
	})
	t.Run("ShellHelp", func(t *testing.T) {
		for _, arg := range []string{"help", "--help"} {
			_, home, project := newHome(t)
			contains(t, run(t, home, project, "", 0, arg), "gasa doctor", "gasa web open", "web.mode always")
		}
	})
}

func treeHash(t *testing.T, root string) string {
	t.Helper()
	var all []byte
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		all = append(all, []byte(filepath.ToSlash(rel))...)
		if !d.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			all = append(all, sum[:]...)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(all))
}
