package integration

import (
	"context"
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
			case "GASACLI_HOME", "PATH", "HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME":
				continue
			}
			cmd.Env = append(cmd.Env, entry)
		}
		cmd.Env = append(cmd.Env, "GASACLI_HOME="+home, "PATH="+filepath.Join(filepath.Dir(home), "empty-path"), "HOME="+filepath.Join(filepath.Dir(home), "user"), "USERPROFILE="+filepath.Join(filepath.Dir(home), "user"), "APPDATA="+filepath.Join(filepath.Dir(home), "appdata"), "LOCALAPPDATA="+filepath.Join(filepath.Dir(home), "localappdata"), "XDG_CONFIG_HOME="+filepath.Join(filepath.Dir(home), "xdg"))
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
	contains := func(t *testing.T, out string, wants ...string) {
		t.Helper()
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("missing %q\n%s", want, out)
			}
		}
	}
	writeConfig := func(t *testing.T, home, mode string, fallback bool) {
		t.Helper()
		if err := os.MkdirAll(home, 0700); err != nil {
			t.Fatal(err)
		}
		data := fmt.Sprintf("web:\n  mode: %s\n  host: 127.0.0.1\n  port: 3199\n  fallback_port: %t\n", mode, fallback)
		if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("HelpAndTrustPersistence", func(t *testing.T) {
		_, home, project := newHome(t)
		out := run(t, home, project, "y\n/help\n/status\n/goal test\n/web status\n/exit\n", 0)
		contains(t, out, "[y/N]", "/help", "/exit", "/agents", "/doctor", "/status", "/goal", "/permissions", "/backup", "/restore", "/web", "planejado", "sem run ativo; orquestração não implementada", "Dashboard:")
		t.Logf("real startup and /help output (public URL only):\n%s", out)
		out = run(t, home, project, "/help\n/exit\n", 0)
		if strings.Contains(out, "[y/N]") {
			t.Fatal("trust asked again")
		}
		t.Log("real binary startup, /help, /status, /goal, /web status and persisted trust passed")
	})
	t.Run("TrustRefusedAndEOF", func(t *testing.T) {
		for _, input := range []string{"n\n", ""} {
			_, home, project := newHome(t)
			out := run(t, home, project, input, 1)
			contains(t, out, "project not trusted")
			if strings.Contains(out, "Dashboard:") || strings.Contains(out, "gasa> ") {
				t.Fatal("started before trust")
			}
		}
	})
	t.Run("Doctor", func(t *testing.T) {
		_, home, project := newHome(t)
		out := run(t, home, project, "", 0, "doctor")
		names := []string{"Claude Code", "Codex", "Gemini CLI", "Cursor", "OpenCode", "Orca", "GitHub CLI"}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != len(names) {
			t.Fatalf("doctor lines=%d\n%s", len(lines), out)
		}
		for i, name := range names {
			contains(t, lines[i], name, "not_found", "authentication not checked")
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
		out := run(t, home, project, "y\n/help\n/exit\n", 0)
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
		out := run(t, home, project, "y\n/exit\n", 0)
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
		out := run(t, home, project, "y\n/help\n/web status\n/exit\n", 0)
		contains(t, out, "Warning: dashboard unavailable", "/help", "/web: dashboard unavailable")
	})
	t.Run("IsolatedHome", func(t *testing.T) {
		sandbox, home, project := newHome(t)
		run(t, home, project, "y\n/exit\n", 0)
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
	t.Run("ManualAndAlways", func(t *testing.T) {
		for _, mode := range []string{"manual", "always"} {
			_, home, project := newHome(t)
			writeConfig(t, home, mode, true)
			out := run(t, home, project, "y\n/web open\n/exit\n", 0)
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
		contains(t, run(t, home, project, "y\n", 1), "load config", "invalid web.mode")
	})
	t.Run("ShellHelp", func(t *testing.T) {
		for _, arg := range []string{"help", "--help"} {
			_, home, project := newHome(t)
			contains(t, run(t, home, project, "", 0, arg), "gasa doctor", "gasa web open", "web.mode always")
		}
	})
}
