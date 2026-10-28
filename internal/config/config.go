// Package config manages the user's global GasaCLI configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Web WebConfig `yaml:"web"`
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
	home, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config directory: %w", err)
	}
	return filepath.Join(home, "gasacli"), nil
}

// Ensure creates defaults without replacing existing user configuration.
func Ensure() error {
	home, err := Home()
	if err != nil {
		return err
	}
	for _, dir := range []string{"", "agents", "prompts", "skills", "plugins", "segundo-cerebro", "backups", "logs"} {
		if err := os.MkdirAll(filepath.Join(home, dir), 0700); err != nil {
			return fmt.Errorf("create config directory %q: %w", dir, err)
		}
	}
	path := filepath.Join(home, "config.yaml")
	if _, err := os.Lstat(path); err == nil {
		return nil
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
	return nil
}

func Load() (Config, error) {
	home, err := Home()
	if err != nil {
		return Config{}, err
	}
	f, err := os.Open(filepath.Join(home, "config.yaml"))
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
	return cfg, nil
}
