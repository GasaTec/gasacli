package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), "gasacli")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".gasacli"), []byte(`{"app":"gasacli","schema":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GASACLI_HOME", home)
	return home
}

func TestKnownNameMatchCaseFolding(t *testing.T) {
	for _, fold := range []bool{true, false} {
		got := knownNameMatch("Logs", []string{"logs"}, fold)
		if got != fold {
			t.Fatalf("fold=%v match=%v", fold, got)
		}
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		if !knownNameMatch("Logs", []string{"logs"}, true) {
			t.Fatal("case-insensitive filesystem name was not matched")
		}
	}
}

func TestResetPlanShowsFirstLevelFiles(t *testing.T) {
	h := fixture(t)
	put(t, h, "logs/do-usuario.txt")
	out, err := invokeReset(t, []string{"--dry-run"}, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "logs (dir, 1 files") || !strings.Contains(out, "do-usuario.txt") {
		t.Fatalf("user log missing from plan: %s", out)
	}
}

func TestRunCanceledContextReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runReset(ctx, []string{"--dry-run"}, strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("canceled reset returned success")
	}
}
func put(t *testing.T, home, name string) {
	t.Helper()
	p := filepath.Join(home, name)
	if strings.HasSuffix(name, "/") {
		if err := os.MkdirAll(p, 0700); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
func exists(t *testing.T, path string) bool { t.Helper(); _, err := os.Lstat(path); return err == nil }
func invokeReset(t *testing.T, args []string, input io.Reader) (string, error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err := runReset(context.Background(), args, input, &out, &errOut)
	return out.String() + errOut.String(), err
}

func TestResetPreservesAndDeletesPerScope(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		h := fixture(t)
		for _, n := range []string{"config.yaml", "state.json", "trust.json", "trust.json.lock", "logs/", "agents/", "prompts/", "skills/", "plugins/", "segundo-cerebro/", "backups/", "minha-nota.txt"} {
			put(t, h, n)
		}
		if _, err := invokeReset(t, []string{"--yes"}, strings.NewReader("")); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"config.yaml", "state.json", "trust.json", "trust.json.lock", "logs"} {
			if exists(t, filepath.Join(h, n)) {
				t.Errorf("%s remains", n)
			}
		}
		for _, n := range []string{"agents", "prompts", "skills", "plugins", "segundo-cerebro", "backups", "minha-nota.txt"} {
			if !exists(t, filepath.Join(h, n)) {
				t.Errorf("%s was removed", n)
			}
		}
		if !exists(t, filepath.Join(h, ".gasacli")) {
			t.Fatal("default reset removed identity marker")
		}
	})
	t.Run("all", func(t *testing.T) {
		h := fixture(t)
		for _, n := range []string{"config.yaml", "agents/", "prompts/", "skills/", "plugins/", "segundo-cerebro/", "gasacli.db", "gasacli.db-wal", "gasacli.db-shm", "backups/", "minha-nota.txt"} {
			put(t, h, n)
		}
		if _, err := invokeReset(t, []string{"--all", "--yes"}, strings.NewReader("")); err != nil {
			t.Fatal(err)
		}
		for _, n := range []string{"config.yaml", "agents", "prompts", "skills", "plugins", "segundo-cerebro", "gasacli.db", "gasacli.db-wal", "gasacli.db-shm"} {
			if exists(t, filepath.Join(h, n)) {
				t.Errorf("%s remains", n)
			}
		}
		if exists(t, filepath.Join(h, ".gasacli")) {
			t.Fatal("--all retained identity marker")
		}
		for _, n := range []string{"backups", "minha-nota.txt"} {
			if !exists(t, filepath.Join(h, n)) {
				t.Errorf("%s was removed", n)
			}
		}
	})
	t.Run("with backups and empty home", func(t *testing.T) {
		h := fixture(t)
		for _, n := range []string{"config.yaml", "backups/", "minha-nota.txt"} {
			put(t, h, n)
		}
		if _, err := invokeReset(t, []string{"--all", "--include-backups", "--yes"}, strings.NewReader("")); err != nil {
			t.Fatal(err)
		}
		if exists(t, filepath.Join(h, "backups")) {
			t.Fatal("backups remains")
		}
		if !exists(t, filepath.Join(h, "minha-nota.txt")) {
			t.Fatal("unknown file removed")
		}
		if !exists(t, h) {
			t.Fatal("home removed despite unknown entry")
		}
	})
}

func TestResetDryRunNonInteractiveConfirmationAndMissingHome(t *testing.T) {
	t.Run("dry run", func(t *testing.T) {
		h := fixture(t)
		put(t, h, "config.yaml")
		before := []string{}
		filepath.WalkDir(h, func(p string, d os.DirEntry, e error) error {
			if e == nil {
				before = append(before, p)
			}
			return e
		})
		output, err := invokeReset(t, []string{"--dry-run"}, strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		after := []string{}
		filepath.WalkDir(h, func(p string, d os.DirEntry, e error) error {
			if e == nil {
				after = append(after, p)
			}
			return e
		})
		if strings.Join(before, "|") != strings.Join(after, "|") {
			t.Fatal("dry-run changed directory tree")
		}
		if !strings.Contains(output, "config.yaml") {
			t.Fatalf("plan omitted entry: %s", output)
		}
	})
	t.Run("non interactive", func(t *testing.T) {
		h := fixture(t)
		put(t, h, "config.yaml")
		_, err := invokeReset(t, []string{}, strings.NewReader("RESET\n"))
		if err == nil || !strings.Contains(err.Error(), "non-interactive: pass --yes") {
			t.Fatalf("error=%v", err)
		}
		if !exists(t, filepath.Join(h, "config.yaml")) {
			t.Fatal("data deleted")
		}
	})
	t.Run("interactive words", func(t *testing.T) {
		old := terminalInput
		terminalInput = func(io.Reader) bool { return true }
		defer func() { terminalInput = old }()
		for _, tc := range []struct {
			args        []string
			input, want string
			ok          bool
		}{{nil, "RESET\n", "config.yaml", true}, {[]string{"--all"}, "DELETE\n", "config.yaml", true}, {nil, "wrong\n", "config.yaml", false}} {
			h := fixture(t)
			put(t, h, tc.want)
			_, err := invokeReset(t, tc.args, strings.NewReader(tc.input))
			if (err == nil) != tc.ok {
				t.Fatalf("args=%v input=%q err=%v", tc.args, tc.input, err)
			}
			if exists(t, filepath.Join(h, tc.want)) == tc.ok {
				t.Fatalf("unexpected deletion args=%v", tc.args)
			}
		}
	})
	t.Run("missing", func(t *testing.T) {
		h := filepath.Join(t.TempDir(), "missing")
		t.Setenv("GASACLI_HOME", h)
		out, err := invokeReset(t, []string{"--yes"}, strings.NewReader(""))
		if err == nil || strings.Contains(out, "Nothing to reset.") {
			t.Fatalf("out=%q err=%v", out, err)
		}
	})
}

func TestResetRefusesKnownSymlinkAndContinuesAfterRemovalFailure(t *testing.T) {
	t.Run("known symlink", func(t *testing.T) {
		h := fixture(t)
		target := filepath.Join(t.TempDir(), "target")
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "safe"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h, "config.yaml"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(h, "logs")); err != nil {
			t.Skipf("symlink unavailable in this environment: %v", err)
		}
		_, err := invokeReset(t, []string{"--yes"}, strings.NewReader(""))
		if err == nil {
			t.Fatal("expected refusal")
		}
		if !strings.Contains(err.Error(), "is a link or reparse point, not a directory; refusing to follow") {
			t.Fatalf("unclear symlink refusal: %v", err)
		}
		if !exists(t, filepath.Join(target, "safe")) {
			t.Fatal("link target was changed")
		}
	})
	t.Run("continues", func(t *testing.T) {
		h := fixture(t)
		put(t, h, "config.yaml")
		put(t, h, "state.json")
		put(t, h, "logs/")
		old := beforeRemoveOpen
		beforeRemoveOpen = func(name string) error {
			if name == "logs" {
				return errors.New("injected failure")
			}
			return nil
		}
		defer func() { beforeRemoveOpen = old }()
		_, err := invokeReset(t, []string{"--yes"}, strings.NewReader(""))
		if err == nil {
			t.Fatal("expected aggregate error")
		}
		if exists(t, filepath.Join(h, "state.json")) {
			t.Fatal("later entries were not attempted")
		}
	})
}

func TestUninstallInstructionsAndBinaryPreserved(t *testing.T) {
	oldVersion := version
	version = "dev"
	defer func() { version = oldVersion }()
	localAppData := filepath.Join(t.TempDir(), "AppData", "Local")
	t.Setenv("LOCALAPPDATA", localAppData)
	cases := []struct{ path, want string }{{filepath.Join(localAppData, "Programs", "gasacli", "bin", "gasa.exe"), "$env:GASA_UNINSTALL='1'; irm https://raw.githubusercontent.com/GasaTec/gasacli/main/scripts/install.ps1 | iex"}, {filepath.Join("C:\\x\\node_modules\\gasacli\\bin\\gasa"), "npm rm -g gasacli"}, {"/home/test/.local/bin/gasa", "GASA_UNINSTALL=1 curl -fsSL https://raw.githubusercontent.com/GasaTec/gasacli/main/scripts/install.sh | sh"}, {"/tmp/custom/gasa", "Delete the binary at"}}
	for _, tc := range cases {
		var b bytes.Buffer
		err := writeUninstallInstructions(&b, tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(b.String(), tc.want) {
			t.Errorf("path=%q got=%q want substring %q", tc.path, b.String(), tc.want)
		}
	}
	h := fixture(t)
	put(t, h, "config.yaml")
	binary := filepath.Join(t.TempDir(), "gasa")
	if err := os.WriteFile(binary, []byte("bin"), 0700); err != nil {
		t.Fatal(err)
	}
	old := executablePath
	executablePath = func() (string, error) { return binary, nil }
	defer func() { executablePath = old }()
	var out bytes.Buffer
	if err := runUninstall(context.Background(), []string{"--yes"}, strings.NewReader(""), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !exists(t, binary) {
		t.Fatal("uninstall deleted executable")
	}
	if !strings.Contains(out.String(), "Delete the binary at") {
		t.Fatalf("missing instructions: %s", out.String())
	}
}

func TestUninstallInstructionUsesReleaseTag(t *testing.T) {
	oldVersion := version
	version = "1.2.3"
	defer func() { version = oldVersion }()
	var out bytes.Buffer
	if err := writeUninstallInstructions(&out, "/home/test/.local/bin/gasa"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/v1.2.3/scripts/install.sh") {
		t.Fatalf("release URL is not tag-pinned: %q", out.String())
	}
}

func TestUnexpectedKnownTypeIsKeptAndReported(t *testing.T) {
	h := fixture(t)
	put(t, h, "config.yaml")
	if err := os.Mkdir(filepath.Join(h, "state.json"), 0700); err != nil {
		t.Fatal(err)
	}
	out, err := invokeReset(t, []string{"--yes"}, strings.NewReader(""))
	if err == nil || !strings.Contains(out, "kept (unexpected type): state.json") || !exists(t, filepath.Join(h, "state.json")) {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestOnboardingMessageRequiresSuccessfulStateRemovalAndReset(t *testing.T) {
	t.Run("removed by reset", func(t *testing.T) {
		h := fixture(t)
		put(t, h, "state.json")
		out, err := invokeReset(t, []string{"--yes"}, strings.NewReader(""))
		if err != nil || !strings.Contains(out, "Run `gasa` to start onboarding again.") {
			t.Fatalf("out=%q err=%v", out, err)
		}
	})
	t.Run("uninstall suppresses", func(t *testing.T) {
		h := fixture(t)
		put(t, h, "state.json")
		bin := filepath.Join(t.TempDir(), "gasa")
		old := executablePath
		executablePath = func() (string, error) { return bin, nil }
		defer func() { executablePath = old }()
		var out bytes.Buffer
		if err := runUninstall(context.Background(), []string{"--yes"}, strings.NewReader(""), &out, io.Discard); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "Run `gasa` to start onboarding again.") {
			t.Fatalf("uninstall printed onboarding restart: %s", out.String())
		}
	})
	t.Run("partial failure suppresses", func(t *testing.T) {
		h := fixture(t)
		put(t, h, "state.json")
		put(t, h, "logs/")
		old := beforeRemoveOpen
		beforeRemoveOpen = func(name string) error {
			if name == "logs" {
				return errors.New("injected")
			}
			return nil
		}
		defer func() { beforeRemoveOpen = old }()
		out, err := invokeReset(t, []string{"--yes"}, strings.NewReader(""))
		if err == nil || strings.Contains(out, "Run `gasa` to start onboarding again.") {
			t.Fatalf("out=%q err=%v", out, err)
		}
	})
}

func TestUninstallWithoutDataStillPrintsBinaryInstructions(t *testing.T) {
	h := filepath.Join(t.TempDir(), "never-created")
	t.Setenv("GASACLI_HOME", h)
	binary := filepath.Join(t.TempDir(), "gasa")
	old := executablePath
	executablePath = func() (string, error) { return binary, nil }
	defer func() { executablePath = old }()
	var out bytes.Buffer
	if err := runUninstall(context.Background(), []string{"--yes"}, strings.NewReader(""), &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "No GasaCLI data directory") || !strings.Contains(out.String(), "Delete the binary at") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestWindowsRootRefusesJunctionSwapDuringReset(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction behavior is Windows-specific")
	}
	h := fixture(t)
	put(t, h, "agents/")
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	oldHook := beforeRemoveOpen
	junctionRejected := false
	beforeRemoveOpen = func(name string) error {
		if name != "agents" {
			return nil
		}
		original := filepath.Join(h, "agents")
		moved := filepath.Join(h, "agents-original")
		if err := os.Rename(original, moved); err != nil {
			return err
		}
		cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", original, target)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("mklink junction: %w: %s", err, output)
		}
		root, err := os.OpenRoot(h)
		if err != nil {
			return err
		}
		probe, err := root.OpenRoot("agents")
		junctionRejected = err != nil
		if probe != nil {
			_ = probe.Close()
		}
		_ = root.Close()
		return nil
	}
	defer func() { beforeRemoveOpen = oldHook }()
	_, err := invokeReset(t, []string{"--all", "--yes"}, strings.NewReader(""))
	if err == nil {
		t.Fatal("reset followed junction installed after Lstat")
	}
	if !junctionRejected {
		t.Fatalf("os.Root.OpenRoot accepted the junction: %v", err)
	}
	if data, readErr := os.ReadFile(sentinel); readErr != nil || string(data) != "safe" {
		t.Fatalf("outside sentinel changed: %q, %v", data, readErr)
	}
}
