package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/trust"
)

func TestHome(t *testing.T) {
	t.Setenv("GASACLI_HOME", "")
	if runtime.GOOS == "linux" {
		t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	}
	base, err := userDataDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Home()
	if err != nil || got != filepath.Join(base, "gasacli") {
		t.Fatalf("Home() = %q, %v", got, err)
	}
	t.Setenv("GASACLI_HOME", t.TempDir())
	got, err = Home()
	if err != nil || got != os.Getenv("GASACLI_HOME") {
		t.Fatalf("override = %q, %v", got, err)
	}
}

func TestHomeRejectsRelativeXDGDataHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG paths are Linux-specific")
	}
	t.Setenv("GASACLI_HOME", "")
	t.Setenv("XDG_DATA_HOME", "relative/data")
	if _, err := Home(); err == nil {
		t.Fatal("accepted relative XDG_DATA_HOME")
	}
}

func TestEnsureDefaultsAndIdempotence(t *testing.T) {
	home := filepath.Join(t.TempDir(), "gasacli")
	t.Setenv("GASACLI_HOME", home)
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	marker, err := os.ReadFile(filepath.Join(home, ".gasacli"))
	if err != nil || strings.TrimSpace(string(marker)) != `{"app":"gasacli","schema":1}` {
		t.Fatalf("identity marker=%q err=%v", marker, err)
	}
	got, err := Load()
	if err != nil || got != defaults() {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	for _, dir := range []string{"", "agents", "prompts", "skills", "plugins", "segundo-cerebro", "backups", "logs"} {
		info, err := os.Stat(filepath.Join(home, dir))
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not directory", dir)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
			t.Errorf("%s mode %o", dir, info.Mode().Perm())
		}
	}
	if _, err := os.Stat(filepath.Join(home, "gasacli.db")); err != nil {
		t.Fatalf("database not initialized: %v", err)
	}
	path := filepath.Join(home, "config.yaml")
	custom := "# retained\nweb:\n  mode: manual\n  port: 4312\n  fallback_port: false\n"
	if err := os.WriteFile(path, []byte(custom), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != custom {
		t.Fatalf("config overwritten: %q, %v", data, err)
	}
	got, err = Load()
	if err != nil || got.Web.Mode != "manual" || got.Web.Port != 4312 || got.Web.FallbackPort || got.Web.Host != "127.0.0.1" {
		t.Fatalf("custom = %+v, %v", got, err)
	}
}

func TestEnsureRejectsForeignExistingDirectoryWithoutMutation(t *testing.T) {
	for _, foreign := range []string{".git", "package.json", "logs"} {
		t.Run(foreign, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "foreign")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("web: {}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if foreign == ".git" || foreign == "logs" {
				if err := os.Mkdir(filepath.Join(home, foreign), 0700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(filepath.Join(home, foreign), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GASACLI_HOME", home)
			before := snapshotTree(t, home)
			if err := Ensure(); err == nil || !strings.Contains(err.Error(), "choose an empty or new directory") {
				t.Fatalf("Ensure error = %v", err)
			}
			if after := snapshotTree(t, home); before != after {
				t.Fatalf("Ensure mutated rejected directory\nbefore=%s\nafter=%s", before, after)
			}
		})
	}
}

func TestEnsureRejectsForeignReadmeEmptyConfigAndValidConfigWithGit(t *testing.T) {
	for _, tc := range []struct{ name, config, sign string }{
		{"empty config with nested skill", "", "skills/mine/notes.md"},
		{"readme", "", "readme.txt"},
		{"valid config with git", "web: {mode: manual, host: localhost, port: 4321}\n", ".git"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "foreign")
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			if tc.config != "" || tc.name == "empty config with nested skill" {
				if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(tc.config), 0600); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(home, filepath.FromSlash(tc.sign))
			if strings.HasSuffix(tc.sign, ".md") || tc.sign == "readme.txt" {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("user data"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GASACLI_HOME", home)
			before := snapshotTree(t, home)
			if err := Ensure(); err == nil {
				t.Fatal("accepted foreign directory")
			}
			if got := snapshotTree(t, home); got != before {
				t.Fatalf("tree changed: before=%s after=%s", before, got)
			}
		})
	}
}

func TestEnsureRejectsJunctionWithoutWritingTarget(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction behavior is Windows-specific")
	}
	base := t.TempDir()
	target := filepath.Join(base, "target")
	link := filepath.Join(base, "home")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "sentinel"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, out)
	}
	t.Setenv("GASACLI_HOME", link)
	if err := Ensure(); err == nil {
		t.Fatal("Ensure accepted junction")
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("junction target changed: %v, %v", entries, err)
	}
}

func TestEnsureRejectsExistingUnmarkedConfigOutsideDefaultHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "preexisting")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("web: {mode: manual, host: localhost, port: 4321}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GASACLI_HOME", home)
	before := snapshotTree(t, home)
	if err := Ensure(); err == nil {
		t.Fatal("accepted existing unmarked custom directory")
	}
	if after := snapshotTree(t, home); before != after {
		t.Fatalf("Ensure mutated rejected directory\nbefore=%s\nafter=%s", before, after)
	}
}

func TestEnsureAdoptsOnlyValidLegacyDefault(t *testing.T) {
	base := t.TempDir()
	configureTestRoots(t, base)
	t.Setenv("GASACLI_HOME", "")
	legacy, err := legacyHome()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	valid := []byte("web: {mode: manual, host: localhost, port: 4321}\n")
	if err := os.WriteFile(filepath.Join(legacy, "config.yaml"), valid, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "user-data.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(); err != nil {
		t.Fatalf("valid default legacy config rejected: %v", err)
	}
	home, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(home, "user-data.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("legacy user data was not adopted: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".gasacli")); err != nil {
		t.Fatalf("legacy marker missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacy, "user-data.txt")); err != nil {
		t.Fatalf("legacy source was removed: %v", err)
	}

	invalidBase := t.TempDir()
	configureTestRoots(t, invalidBase)
	invalidHome, err := legacyHome()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(invalidHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(invalidHome, "config.yaml"), []byte("web: {port: 0}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, invalidHome)
	ensureErr := Ensure()
	if samePath(invalidHome, func() string { home, _ := Home(); return home }()) {
		if ensureErr == nil {
			t.Fatal("invalid configuration in the unchanged OS data root was accepted")
		}
	} else if ensureErr != nil {
		t.Fatalf("fresh root should initialize without adopting invalid legacy data: %v", ensureErr)
	}
	if got := snapshotTree(t, invalidHome); got != before {
		t.Fatalf("invalid legacy config changed: before=%s after=%s", before, got)
	}
}

func TestEnsureDoesNotAdoptLegacyHomeWhenOverrideIsSet(t *testing.T) {
	base := t.TempDir()
	configureTestRoots(t, base)
	legacy, err := legacyHome()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "config.yaml"), []byte("web: {mode: manual}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "legacy.txt"), []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	override := filepath.Join(t.TempDir(), "custom-home")
	t.Setenv("GASACLI_HOME", override)
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(override, "legacy.txt")); !os.IsNotExist(err) {
		t.Fatalf("legacy data was copied into explicit override: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(legacy, "legacy.txt")); err != nil || string(data) != "preserve" {
		t.Fatalf("legacy data changed: %q %v", data, err)
	}
}

func configureTestRoots(t *testing.T, base string) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", base)
	case "darwin":
		t.Setenv("HOME", base)
	default:
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
		t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	}
}

func TestEnsureProjectLayoutRequiresTrust(t *testing.T) {
	project := t.TempDir()
	trustStore := trust.NewStore(t.TempDir())
	if _, err := EnsureProjectLayout(project, trustStore); err == nil {
		t.Fatal("created layout for untrusted project")
	}
	if _, err := os.Stat(filepath.Join(project, "gasacli")); !os.IsNotExist(err) {
		t.Fatalf("untrusted project was modified: %v", err)
	}
	if err := trustStore.Grant(project); err != nil {
		t.Fatal(err)
	}
	path, err := EnsureProjectLayout(project, trustStore)
	canonicalProject, canonicalErr := trust.Canonical(project)
	if err != nil || canonicalErr != nil || path != filepath.Join(canonicalProject, "gasacli") {
		t.Fatalf("EnsureProjectLayout()=%q, %v", path, err)
	}
}

func TestEnsureAdoptsPreexistingEmptyDirectory(t *testing.T) {
	home := filepath.Join(t.TempDir(), "existing-empty")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GASACLI_HOME", home)
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".gasacli")); err != nil {
		t.Fatalf("preexisting empty directory not adopted: %v", err)
	}
}

func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			b.WriteString("d:")
		} else {
			b.WriteString("f:")
		}
		b.WriteString(filepath.ToSlash(strings.TrimPrefix(path, root)))
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			b.WriteString(":")
			b.WriteString(fmt.Sprintf("%x", sha256.Sum256(data)))
		}
		b.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestLoadValidation(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"mode", "web: {mode: invalid}", "web.mode"},
		{"unknown_root", "future_key: yes", "future_key"},
		{"misspelled_mode", "web: {mdoe: manual}", "mdoe"},
		{"misspelled_fallback", "web: {fallback_prot: true}", "fallback_prot"},
		{"public_host", "web: {host: 0.0.0.0}", "web.host"},
		{"public_ipv6", "web: {host: '::'}", "web.host"},
		{"external_host", "web: {host: example.com}", "web.host"},
		{"empty_host", "web: {host: ''}", "web.host"},
		{"ipv4_loopback", "web: {host: 127.0.0.1}", ""},
		{"ipv6_loopback", "web: {host: '::1'}", ""},
		{"localhost", "web: {host: localhost}", ""},
		{"zero", "web: {port: 0}", "web.port"},
		{"negative", "web: {port: -1}", "web.port"},
		{"overflow", "web: {port: 65536}", "web.port"},
		{"syntax", "web: [", "decode config"},
		{"documents", "web: {}\n---\nweb: {}", "single YAML document"},
		{"always", "web: {mode: always, port: 65535}", ""},
		{"minimum", "web: {port: 1}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("GASACLI_HOME", home)
			if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load()
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}
