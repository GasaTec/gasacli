package tui

import (
	"context"

	localSkills "github.com/GasaTec/gasacli/internal/skills"
)

func init() {
	Register(Command{Name: "/skills", Summary: "command.skills", Usage: "/skills [show <name>|install <path>|remove <name> --yes|updates]", Category: "general", Planned: false, Handler: func(ctx context.Context, _ CommandContext, args []string) (string, error) {
		return localSkills.Command(ctx, args)
	}})
}
