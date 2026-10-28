// Package config manages the user's global GasaCLI configuration.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GasaTec/gasacli/internal/lifecycle"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/trust"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Web      WebConfig   `yaml:"web"`
	Language string      `yaml:"language,omitempty"`
	Agents   AgentConfig `yaml:"agents,omitempty"`
}

type AgentConfig struct {
	Defaults map[string]string `yaml:"defaults,omitempty"`
}

type WebConfig struct {
	Mode         string `yaml:"mode"`
	Host         string `yaml:"host"`
	Port         int    `yaml:"port"`
	FallbackPort bool   `yaml:"fallback_port"`
}

func defaults() Config {
	return Config{Web: WebConfig{Mode: "auto", Host: "127.0.0.1", Port: 3199, FallbackPort: true}}
}

func Home() (string, error) {
	if home := os.Getenv("GASACLI_HOME"); home != "" {
		return home, nil
	}
	home, err := userDataDir()
	if err != nil {
		return "", fmt.Errorf("resolve user data directory: %w", err)
	}
	return filepath.Join(home, "gasacli"), nil
}

// userDataDir follows ADR-0016: Linux uses the XDG user data root, while
// Windows and macOS use their OS-standard AppData/Application Support roots.
func userDataDir() (string, error) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return os.UserConfigDir()
	}
	if runtime.GOOS == "linux" {
		if data := os.Getenv("XDG_DATA_HOME"); data != "" {
			if !filepath.IsAbs(data) {
				return "", fmt.Errorf("XDG_DATA_HOME must be an absolute path")
			}
			return data, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share"), nil
	}
	return os.UserConfigDir()
}

func legacyHome() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "gasacli"), nil
}

// Ensure creates defaults without replacing existing user configuration.
func Ensure() error {
	home, err := Home()
	if err != nil {
		return err
	}
	home, err = lifecycle.ValidateEnsureHome(home)
	if err != nil {
		return err
	}
	legacy, legacyErr := legacyHome()
	if os.Getenv("GASACLI_HOME") == "" && legacyErr == nil && !samePath(legacy, home) {
		if _, statErr := os.Lstat(home); os.IsNotExist(statErr) {
			if _, validateErr := lifecycle.ValidateHome(legacy); validateErr == nil {
				if err := copyLegacyHome(legacy, home); err != nil {
					return err
				}
			}
		}
	}
	home, err = lifecycle.ValidateEnsureHome(home)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(home, 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	for _, dir := range []string{"agents", "prompts", "skills", "plugins", "segundo-cerebro", "backups", "logs", "approvals"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0700); err != nil {
			return fmt.Errorf("create config directory %q: %w", dir, err)
		}
	}
	path := filepath.Join(home, "config.yaml")
	if _, err := os.Lstat(path); err == nil {
		if err := writeIdentityMarker(home); err != nil {
			return err
		}
		return ensureDatabase(home)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect config file: %w", err)
	}
	data, err := yaml.Marshal(defaults())
	if err != nil {
		return fmt.Errorf("encode default config: %w", err)
	}
	f, err := os.CreateTemp(home, ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("write default config: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync default config: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close default config: %w", err)
	}
	// Publishing a complete inode with Link is atomic and cannot overwrite a
	// config created concurrently. Rename would replace it on Unix.
	if err := os.Link(f.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("publish default config: %w", err)
	}
	if err := writeIdentityMarker(home); err != nil {
		return err
	}
	return ensureDatabase(home)
}

func ensureDatabase(home string) error {
	db, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		return err
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close initialized database: %w", err)
	}
	return nil
}

// EnsureProjectLayout creates the optional project projection directory only
// for a project already approved by the supplied trust store.
func EnsureProjectLayout(project string, approvals *trust.Store) (string, error) {
	if approvals == nil {
		return "", fmt.Errorf("project layout requires a trust store")
	}
	trusted, err := approvals.IsTrusted(project)
	if err != nil {
		return "", fmt.Errorf("check project trust: %w", err)
	}
	if !trusted {
		return "", fmt.Errorf("refusing project layout for untrusted project")
	}
	canonical, err := trust.Canonical(project)
	if err != nil {
		return "", err
	}
	path := filepath.Join(canonical, "gasacli")
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("create project GasaCLI directory: %w", err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) {
		return "", fmt.Errorf("refusing project GasaCLI path that is not a regular directory")
	}
	return path, nil
}

func copyLegacyHome(source, target string) error {
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return fmt.Errorf("create user data root: %w", err)
	}
	stage, err := os.MkdirTemp(parent, ".gasacli-adopt-*")
	if err != nil {
		return fmt.Errorf("prepare legacy data adoption: %w", err)
	}
	defer os.RemoveAll(stage)
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(stage, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) {
			return fmt.Errorf("refusing legacy data link %s", path)
		}
		if info.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular legacy data file %s", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		inCloseErr := in.Close()
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if inCloseErr != nil {
			return inCloseErr
		}
		return closeErr
	})
	if err != nil {
		return fmt.Errorf("copy legacy GasaCLI data: %w", err)
	}
	if err := os.Rename(stage, target); err != nil {
		if _, statErr := os.Lstat(target); statErr == nil {
			return nil // A concurrent initializer won; the legacy source is untouched.
		}
		return fmt.Errorf("publish adopted GasaCLI data: %w", err)
	}
	return nil
}

func samePath(a, b string) bool {
	left, right := filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

type identityMarker struct {
	App    string `json:"app"`
	Schema int    `json:"schema"`
}

func writeIdentityMarker(home string) error {
	path := filepath.Join(home, ".gasacli")
	data, _ := json.Marshal(identityMarker{App: "gasacli", Schema: 1})
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create GasaCLI identity marker: %w", err)
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func Load() (Config, error) {
	home, err := Home()
	if err != nil {
		return Config{}, err
	}
	return loadConfigFile(filepath.Join(home, "config.yaml"))
}

func loadConfigFile(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()
	cfg := defaults()
	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Config{}, fmt.Errorf("decode config trailing document: %w", err)
		}
		return Config{}, fmt.Errorf("config must contain a single YAML document")
	}
	switch cfg.Web.Mode {
	case "auto", "manual", "always":
	default:
		return Config{}, fmt.Errorf("invalid web.mode: expected auto, manual or always")
	}
	switch cfg.Web.Host {
	case "127.0.0.1", "::1", "localhost":
	default:
		return Config{}, fmt.Errorf("invalid web.host: expected 127.0.0.1, ::1 or localhost")
	}
	if cfg.Web.Port < 1 || cfg.Web.Port > 65535 {
		return Config{}, fmt.Errorf("invalid web.port: expected 1–65535")
	}
	if cfg.Language != "" && cfg.Language != "pt-BR" && cfg.Language != "en" {
		return Config{}, fmt.Errorf("invalid language: expected pt-BR or en")
	}
	for role, agent := range cfg.Agents.Defaults {
		if !validConfigName(role) || !validConfigName(agent) {
			return Config{}, fmt.Errorf("invalid default agent assignment %q", role)
		}
	}
	return cfg, nil
}

func validConfigName(value string) bool {
	if value == "" || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func SaveWebMode(home, mode string) error {
	if mode != "auto" && mode != "manual" && mode != "always" {
		return fmt.Errorf("invalid web.mode: expected auto, manual or always")
	}
	path := filepath.Join(home, "config.yaml")
	cfg, err := loadConfigFile(path)
	if err != nil {
		return err
	}
	cfg.Web.Mode = mode
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	tmp, err := os.CreateTemp(home, ".config-*")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("publish config: %w", err)
	}
	return nil
}
