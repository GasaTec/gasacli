package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Set writes one supported user setting atomically and validates the resulting file.
func Set(home, key, value string) error {
	cfg, err := loadConfigFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		return err
	}
	switch key {
	case "web.mode":
		if value != "auto" && value != "manual" && value != "always" {
			return fmt.Errorf("invalid web.mode")
		}
		cfg.Web.Mode = value
	case "web.port":
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("invalid web.port")
		}
		cfg.Web.Port = port
	case "language":
		if value != "en" && value != "pt-BR" {
			return fmt.Errorf("language must be en or pt-BR")
		}
		cfg.Language = value
	case "permissions.profile":
		if value != "restricted" && value != "balanced" && value != "autonomous" {
			return fmt.Errorf("invalid permission profile")
		}
		path := filepath.Join(home, "permissions.yaml")
		policy := map[string]any{}
		if existing, readErr := os.ReadFile(path); readErr == nil {
			if err := yaml.Unmarshal(existing, &policy); err != nil {
				return err
			}
		} else if !os.IsNotExist(readErr) {
			return readErr
		}
		policy["profile"] = value
		data, err := yaml.Marshal(policy)
		if err != nil {
			return err
		}
		return atomicWriteConfig(home, path, data)
	default:
		const prefix = "agents.defaults."
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			return fmt.Errorf("unsupported configuration key %q", key)
		}
		role := key[len(prefix):]
		if !validConfigName(role) || !validConfigName(value) {
			return fmt.Errorf("invalid default agent assignment")
		}
		if cfg.Agents.Defaults == nil {
			cfg.Agents.Defaults = make(map[string]string)
		}
		cfg.Agents.Defaults[role] = value
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return atomicWriteConfig(home, filepath.Join(home, "config.yaml"), data)
}

func atomicWriteConfig(home, path string, data []byte) error {
	if err := os.MkdirAll(home, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(home, ".config-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Edit validates an edited temporary copy before atomically publishing it.
func Edit(home string, data []byte) error {
	tmp, err := os.CreateTemp(home, ".config-validate-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if _, err := loadConfigFile(name); err != nil {
		return err
	}
	return atomicWriteConfig(home, filepath.Join(home, "config.yaml"), data)
}

func EditInEditor(home string) error {
	path := filepath.Join(home, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(home, ".config-edit-*")
	if err != nil {
		return err
	}
	editPath := tmp.Name()
	defer os.Remove(editPath)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" && runtime.GOOS == "windows" {
		editor = "notepad.exe"
	}
	if editor == "" {
		return fmt.Errorf("EDITOR is not set")
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], editPath)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err = cmd.Run(); err != nil {
		return err
	}
	edited, err := os.ReadFile(editPath)
	if err != nil {
		return err
	}
	return Edit(home, edited)
}

func ConfigPath(home string) string { return filepath.Join(home, "config.yaml") }

func SourceOf(home, key string) string {
	if key == "permissions.profile" {
		if data, err := os.ReadFile(filepath.Join(home, "permissions.yaml")); err == nil {
			var raw map[string]any
			if yaml.Unmarshal(data, &raw) == nil && raw["profile"] != "" {
				return "user"
			}
		}
		return "default"
	}
	if key == "language" && os.Getenv("GASACLI_LANG") != "" {
		return "env"
	}
	data, err := os.ReadFile(ConfigPath(home))
	if err != nil {
		return "default"
	}
	var raw map[string]any
	if yaml.Unmarshal(data, &raw) != nil {
		return "default"
	}
	if key == "language" {
		if raw[key] != "" {
			return "user"
		}
		return "default"
	}
	parts := strings.Split(key, ".")
	value := any(raw)
	for _, part := range parts {
		m, ok := value.(map[string]any)
		if !ok {
			return "default"
		}
		value = m[part]
	}
	if value == nil {
		return "default"
	}
	return "user"
}
