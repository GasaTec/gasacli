package tui

import (
	"context"

	"github.com/GasaTec/gasacli/internal/brain"
)

func init() {
	Register(Command{Name: "/brain", Summary: "command.brain", Usage: "/brain connect|status|reindex|search|disconnect", Category: "general", Planned: false, Handler: func(ctx context.Context, _ CommandContext, args []string) (string, error) {
		return brain.Command(ctx, args)
	}})
}
