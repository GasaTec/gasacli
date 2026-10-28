// Package brain implements the local read-only Markdown vault commands.
package brain

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/storage"
	"github.com/GasaTec/gasacli/internal/vault"
)

func Command(ctx context.Context, args []string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	state, _ := config.LoadState(home)
	language := ""
	if state.Onboarding.Completed {
		language = state.Onboarding.Language
	}
	i18n.Configure(language)
	store, err := storage.Open(filepath.Join(home, "gasacli.db"))
	if err != nil {
		return "", err
	}
	defer store.Close()
	index, err := vault.New(store)
	if err != nil {
		return "", err
	}
	if len(args) == 0 {
		return i18n.T("brain.usage"), nil
	}
	switch args[0] {
	case "connect":
		if len(args) < 2 || len(args) > 3 {
			return "", errors.New(i18n.T("brain.usage.connect"))
		}
		confirmed := len(args) == 3 && args[2] == "--yes"
		engine, err := permissions.Default()
		if err != nil {
			return "", err
		}
		d := engine.Decide(ctx, permissions.Request{Action: permissions.Permission("read_vault"), Target: args[1]})
		if d.Effect == permissions.Deny {
			return "", fmt.Errorf(i18n.T("brain.permission.denied"), d.Reason)
		}
		if d.Effect == permissions.Ask && !confirmed {
			return "", errors.New(i18n.T("brain.permission.confirm"))
		}
		if d.Effect != permissions.Allow && d.Effect != permissions.Ask {
			return "", errors.New(i18n.T("brain.permission.unknown"))
		}
		if err = index.Connect(ctx, args[1]); err != nil {
			return "", err
		}
		s, err := index.Reindex(ctx)
		if err != nil {
			return "", err
		}
		return i18n.T("brain.connected", args[1], s.Files, s.Chunks, s.Bytes, s.Duration.Round(time.Millisecond)), nil
	case "status":
		files, chunks, root, err := index.Counts(ctx)
		if err != nil {
			return "", err
		}
		return i18n.T("brain.status", root, files, chunks), nil
	case "reindex":
		s, err := index.Reindex(ctx)
		if err != nil {
			return "", err
		}
		return i18n.T("brain.reindexed", s.Files, s.Chunks, s.Skipped, s.Bytes, s.Duration.Round(time.Millisecond)), nil
	case "search":
		if len(args) < 2 {
			return "", errors.New("usage: /brain search <query>")
		}
		results, err := index.Search(ctx, strings.Join(args[1:], " "), 10)
		if err != nil {
			return "", err
		}
		var out strings.Builder
		for _, s := range results {
			fmt.Fprintf(&out, "%s#%s:%d-%d (modified %s)\n%s\n\n", s.Path, s.Heading, s.StartLine, s.EndLine, s.Modified.Format(time.RFC3339), s.Text)
		}
		if len(results) == 0 {
			return i18n.T("brain.search.empty"), nil
		}
		return strings.TrimSpace(out.String()), nil
	case "disconnect":
		if err := index.Disconnect(ctx); err != nil {
			return "", err
		}
		return i18n.T("brain.disconnected"), nil
	default:
		return "", errors.New(i18n.T("brain.usage"))
	}
}
