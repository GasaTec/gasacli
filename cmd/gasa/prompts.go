package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/prompts"
)

func runPrompts(ctx context.Context, args []string, out *os.File) error {
	if len(args) < 1 || len(args) > 2 {
		return errors.New("usage: gasa prompts show|path|reset [role]")
	}
	home, err := config.Home()
	if err != nil {
		return err
	}
	language := "en"
	if state, stateErr := config.LoadState(home); stateErr == nil && state.Onboarding.Language != "" {
		language = state.Onboarding.Language
	}
	project, _ := os.Getwd()
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = prompts.Ensure(home, ""); err != nil {
		return err
	}
	role := "orchestrator"
	if len(args) == 2 {
		role = args[1]
	}
	switch args[0] {
	case "show":
		prompt, err := prompts.Effective(home, project, role, language)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, prompt.Body)
		return err
	case "path":
		_, err := fmt.Fprintln(out, prompts.Path(home, project, role, language))
		return err
	case "reset":
		if strings.TrimSpace(role) == "" {
			return errors.New("prompt role is required")
		}
		return prompts.Reset(home, "", role, language)
	default:
		return errors.New("usage: gasa prompts show|path|reset [role]")
	}
}
