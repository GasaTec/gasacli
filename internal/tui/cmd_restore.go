package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/GasaTec/gasacli/internal/backup"
)

func init() {
	Register(Command{Name: "/restore", Summary: "command.restore", Usage: "/restore <file> [--preview]", Category: "general", Handler: restoreCommand})
}

func restoreCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if cc == nil || cc.ConfigHome() == "" {
		return "", fmt.Errorf("configuration home unavailable")
	}
	if len(args) == 0 {
		return "", fmt.Errorf("usage: /restore <file> [--preview]")
	}
	preview := false
	archive := ""
	for _, arg := range args {
		if arg == "--preview" {
			preview = true
		} else if archive == "" {
			archive = arg
		} else {
			return "", fmt.Errorf("unexpected restore argument")
		}
	}
	if archive == "" {
		return "", fmt.Errorf("usage: /restore <file> [--preview]")
	}
	m, files, err := backup.Read(archive, "")
	if err != nil {
		return "", err
	}
	changes := backup.Preview(cc.ConfigHome(), m, files)
	if preview {
		return strings.Join(changes, "\n"), nil
	}
	if cc.Ask == nil {
		return "", fmt.Errorf("restore permission approval unavailable")
	}
	ok, err := cc.Ask(PermissionRequest{Action: "write_vault", Target: "GasaCLI global state"})
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("restore denied")
	}
	lines, err := backup.Restore(ctx, cc.ConfigHome(), archive, "", "dev")
	return strings.Join(lines, "\n"), err
}
