package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/i18n"
	"gopkg.in/yaml.v3"
)

func runConfig(_ context.Context, args []string, out *os.File) error {
	home, err := config.Home()
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "show" {
		cfg, err := config.LoadFrom(home)
		if err != nil {
			return err
		}
		state, _ := config.LoadState(home)
		language, languageSource := i18n.EffectiveLanguage(cfg.Language, state.Onboarding.Language)
		for _, value := range [][2]string{{"web.mode", cfg.Web.Mode}, {"web.host", cfg.Web.Host}, {"web.port", fmt.Sprint(cfg.Web.Port)}, {"web.fallback_port", fmt.Sprint(cfg.Web.FallbackPort)}, {"language", language + " (" + languageSource + ")"}, {"permissions.profile", permissionProfile(home)}} {
			if value[0] == "language" {
				fmt.Fprintf(out, "%s: %s\n", value[0], value[1])
				continue
			}
			fmt.Fprintf(out, "%s: %s (%s)\n", value[0], value[1], config.SourceOf(home, value[0]))
		}
		for role, agent := range cfg.Agents.Defaults {
			key := "agents.defaults." + role
			fmt.Fprintf(out, "%s: %s (%s)\n", key, agent, config.SourceOf(home, key))
		}
		for _, setting := range []struct{ key, value string }{{"agents.developer", cfg.Agents.Developer}, {"agents.reviewer", cfg.Agents.Reviewer}} {
			value := setting.value
			if value == "" {
				value = "auto"
			}
			fmt.Fprintf(out, "%s: %s (%s)\n", setting.key, value, config.SourceOf(home, setting.key))
		}
		return nil
	}
	switch args[0] {
	case "path":
		fmt.Fprintln(out, config.ConfigPath(home))
		return nil
	case "edit":
		return config.EditInEditor(home)
	case "set":
		if len(args) != 3 {
			return fmt.Errorf("usage: gasa config set <key> <value>")
		}
		return config.Set(home, args[1], args[2])
	default:
		return fmt.Errorf("usage: gasa config [show|set <key> <value>|path|edit]")
	}
}

func permissionProfile(home string) string {
	data, err := os.ReadFile(filepath.Join(home, "permissions.yaml"))
	if err != nil {
		return "balanced"
	}
	var value struct {
		Profile string `yaml:"profile"`
	}
	if yaml.Unmarshal(data, &value) != nil || value.Profile == "" {
		return "balanced"
	}
	return value.Profile
}
