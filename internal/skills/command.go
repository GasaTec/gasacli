package skills

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/permissions"
)

func Command(ctx context.Context, args []string) (string, error) {
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
	if len(args) == 0 || args[0] == "list" {
		items, err := (Manager{Home: home}).Discover()
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return i18n.T("skills.none"), nil
		}
		var b strings.Builder
		for _, s := range items {
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\n", s.Name, s.Source, versionOrUnknown(s.Version), s.Hash, s.Description)
		}
		return strings.TrimSpace(b.String()), nil
	}
	m := Manager{Home: home}
	switch args[0] {
	case "show":
		if len(args) != 2 {
			return "", errors.New(i18n.T("skills.usage"))
		}
		items, err := m.Discover()
		if err != nil {
			return "", err
		}
		for _, s := range items {
			if s.Name == args[1] {
				return fmt.Sprintf("%s\nsource: %s\nversion: %s\nsha256: %s\npath: %s\n%s", s.Name, s.Source, versionOrUnknown(s.Version), s.Hash, s.Path, s.Description), nil
			}
		}
		return "", fmt.Errorf("skill %q not found", args[1])
	case "install":
		if len(args) < 2 || len(args) > 4 {
			return "", errors.New(i18n.T("skills.usage"))
		}
		sourcePath := args[1]
		remote, _, isGit := ParseGitSource(args[1])
		fetch := len(args) == 3 && args[2] == "--fetch"
		if isGit {
			if fetch {
				if err := authorizeNetwork(ctx, remote); err != nil {
					return "", err
				}
			}
			sourcePath, err = m.ResolveGitSource(ctx, args[1], fetch)
		} else if fetch {
			return "", errors.New("--fetch requires a pinned HTTPS Git URL")
		}
		if err != nil {
			return "", err
		}
		preview, err := m.PreviewLocal(sourcePath)
		if err != nil {
			return "", err
		}
		if len(args) == 3 && !fetch {
			return "", errors.New(i18n.T("skills.usage"))
		}
		confirmed := len(args) == 4 && args[2] == "--confirm" && args[3] == preview.Hash
		if len(args) > 2 && !confirmed && !fetch {
			return "", fmt.Errorf("confirmation hash does not match preview %s", preview.Hash)
		}
		if !confirmed {
			var b strings.Builder
			b.WriteString(i18n.T("skills.install.preview", preview.Name, preview.Hash, preview.Destination))
			b.WriteByte('\n')
			for _, f := range preview.Files {
				fmt.Fprintf(&b, "  %s\n", f)
			}
			if isGit {
				b.WriteString(i18n.T("skills.install.pinned", args[1]))
				b.WriteByte('\n')
			}
			b.WriteString(i18n.T("skills.install.confirm", preview.Hash))
			return b.String(), nil
		}
		if err := authorize(ctx, preview.Name); err != nil {
			return "", err
		}
		if err := m.InstallLocal(preview, true); err != nil {
			return "", err
		}
		return i18n.T("skills.installed", preview.Name, preview.Hash), nil
	case "remove":
		if len(args) != 3 || args[2] != "--yes" {
			return "", errors.New(i18n.T("skills.usage"))
		}
		if err := authorize(ctx, args[1]); err != nil {
			return "", err
		}
		if err := m.Remove(args[1], true); err != nil {
			return "", err
		}
		return i18n.T("skills.removed", args[1]), nil
	case "updates":
		changed, err := m.CheckUpdates()
		if err != nil {
			return "", err
		}
		if len(changed) == 0 {
			return i18n.T("skills.updates.none"), nil
		}
		return i18n.T("skills.updates.found", strings.Join(changed, ", ")), nil
	case "recommendations":
		return i18n.T("skills.recommendations"), nil
	default:
		return "", errors.New(i18n.T("skills.usage"))
	}
}

func versionOrUnknown(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}
func authorize(ctx context.Context, target string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := permissions.Default()
	if err != nil {
		return err
	}
	decision := engine.Decide(ctx, permissions.Request{Action: permissions.Permission("plugin_install"), Target: target})
	switch decision.Effect {
	case permissions.Allow:
		return nil
	case permissions.Deny:
		return fmt.Errorf(i18n.T("skills.permission.denied"), decision.Reason)
	case permissions.Ask:
		return nil
	default:
		return errors.New(i18n.T("skills.permission.unknown"))
	}
}

func authorizeNetwork(ctx context.Context, target string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	engine, err := permissions.Default()
	if err != nil {
		return err
	}
	decision := engine.Decide(ctx, permissions.Request{Action: permissions.Permission("network_access"), Target: target})
	switch decision.Effect {
	case permissions.Allow, permissions.Ask:
		return nil
	case permissions.Deny:
		return fmt.Errorf(i18n.T("skills.network.denied"), decision.Reason)
	default:
		return errors.New("unknown network permission; refusing Git access")
	}
}
