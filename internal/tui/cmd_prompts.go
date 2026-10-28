package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
)

func init() {
	Register(Command{Name: "/prompts", Summary: "command.prompts", Usage: "/prompts [show <role>|path]", Category: "general", Handler: promptsCommand})
}

func promptsCommand(_ context.Context, cc CommandContext, args []string) (string, error) {
	if cc == nil || cc.ConfigHome() == "" {
		return "", fmt.Errorf("configuration home unavailable")
	}
	dir := filepath.Join(cc.ConfigHome(), "prompts")
	if len(args) == 1 && args[0] == "path" {
		return dir, nil
	}
	if len(args) == 2 && args[0] == "show" {
		name := filepath.Base(args[1])
		if name != args[1] || name == "." || name == ".." {
			return "", fmt.Errorf("invalid prompt role")
		}
		for _, ext := range []string{".md", ".txt", ".yaml", ".yml"} {
			p := filepath.Join(dir, name+ext)
			if b, err := os.ReadFile(p); err == nil {
				return string(b), nil
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		return "", fmt.Errorf("prompt not found")
	}
	if len(args) != 0 {
		return "", fmt.Errorf("usage: /prompts [show <role>|path]")
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return i18n.T("prompts.none"), nil
	}
	return strings.Join(names, "\n"), nil
}
