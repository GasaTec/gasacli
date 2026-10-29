package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/i18n"
)

func init() {
	Register(Command{Name: "/config", Summary: "command.config", Usage: "/config [set <key> <value>|path]", Category: "general", Handler: configCommand})
}

func configCommand(_ context.Context, cc CommandContext, args []string) (string, error) {
	if cc == nil || cc.ConfigHome() == "" {
		return "", fmt.Errorf("configuration home unavailable")
	}
	home := cc.ConfigHome()
	if len(args) == 1 && args[0] == "path" {
		return config.ConfigPath(home), nil
	}
	if len(args) == 1 && args[0] == "edit" {
		if err := config.EditInEditor(home); err != nil {
			return "", err
		}
		return i18n.T("config.saved"), nil
	}
	if len(args) == 3 && args[0] == "set" {
		if err := config.Set(home, args[1], args[2]); err != nil {
			return "", err
		}
		return i18n.T("config.saved"), nil
	}
	if len(args) != 0 {
		return "", fmt.Errorf("usage: /config [set <key> <value>|path|edit]")
	}
	cfg, err := config.LoadFrom(home)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{fmt.Sprintf("web.mode: %s (user)", cfg.Web.Mode), fmt.Sprintf("web.host: %s (user)", cfg.Web.Host), fmt.Sprintf("web.port: %d (user)", cfg.Web.Port), fmt.Sprintf("web.fallback_port: %t (user)", cfg.Web.FallbackPort), fmt.Sprintf("agents.developer: %s (%s)", configValue(cfg.Agents.Developer), config.SourceOf(home, "agents.developer")), fmt.Sprintf("agents.reviewer: %s (%s)", configValue(cfg.Agents.Reviewer), config.SourceOf(home, "agents.reviewer")), "path: " + filepath.Join(home, "config.yaml")}, "\n"), nil
}

func configValue(value string) string {
	if value == "" {
		return "auto"
	}
	return value
}
