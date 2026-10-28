package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/backup"
	"github.com/GasaTec/gasacli/internal/i18n"
)

func init() {
	Register(Command{Name: "/backup", Summary: "command.backup", Usage: "/backup [--scope name] [--encrypt]", Category: "general", Handler: backupCommand})
	Register(Command{Name: "/backups", Summary: "command.backups", Usage: "/backups", Category: "general", Handler: backupsCommand})
}

func backupCommand(ctx context.Context, cc CommandContext, args []string) (string, error) {
	if cc == nil || cc.ConfigHome() == "" {
		return "", fmt.Errorf("configuration home unavailable")
	}
	var scopes []string
	encrypted := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--scope":
			if i+1 >= len(args) {
				return "", fmt.Errorf("usage: /backup --scope <name>")
			}
			i++
			scopes = append(scopes, strings.Split(args[i], ",")...)
		case "--encrypt":
			encrypted = true
		default:
			return "", fmt.Errorf("unknown /backup option %q", args[i])
		}
	}
	pass := ""
	if encrypted {
		if cc == nil {
			return "", fmt.Errorf("encryption requires an interactive passphrase prompt")
		}
		return "", fmt.Errorf("encryption requires gasa backup --encrypt with a passphrase")
	}
	home := cc.ConfigHome()
	dest := filepath.Join(home, "backups", "gasacli-backup.zip")
	if err := backup.Create(ctx, home, dest, backup.Options{Scopes: scopes, Passphrase: pass, Version: "dev"}); err != nil {
		return "", err
	}
	return i18n.T("backup.created", dest), nil
}
func backupsCommand(_ context.Context, cc CommandContext, _ []string) (string, error) {
	if cc == nil {
		return "", fmt.Errorf("configuration home unavailable")
	}
	dir := filepath.Join(cc.ConfigHome(), "backups")
	files, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	var entries []string
	for _, f := range files {
		if !f.IsDir() {
			entries = append(entries, filepath.Join(dir, f.Name()))
		}
	}
	if len(entries) == 0 {
		return i18n.T("backup.none"), nil
	}
	return strings.Join(entries, "\n"), nil
}
