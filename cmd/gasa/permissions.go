package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GasaTec/gasacli/internal/permissions"
)

func runPermissions(ctx context.Context, args []string, out io.Writer) error {
	agent, project := "", ""
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent", "--project":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			if args[i] == "--agent" {
				agent = args[i+1]
			} else {
				project = args[i+1]
			}
			i++
		default:
			filtered = append(filtered, args[i])
		}
	}
	args = filtered
	engine, err := permissions.Default()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	projectPath := *project
	if projectPath == "" && agent == "" {
		projectPath = cwd
	}
	scope := permissions.UserScope
	if project != "" {
		scope = permissions.ProjectScope
	}
	if agent != "" {
		scope = permissions.AgentScope
	}
	switch {
	case len(args) == 0 || args[0] == "show":
		fmt.Fprintln(out, "Permission\tEffect\tScope")
		for _, rule := range engine.Explain(ctx, projectPath, agent) {
			fmt.Fprintf(out, "%s\t%s\t%s\n", rule.Permission, rule.Effect, rule.Scope)
		}
	case len(args) == 2 && args[0] == "set-profile":
		if scope != permissions.UserScope {
			return fmt.Errorf("set-profile changes the user policy only")
		}
		if err := engine.SetProfile(args[1]); err != nil {
			return err
		}
		fmt.Fprintf(out, "Profile set to %s\n", args[1])
	case len(args) == 2 && (args[0] == "allow" || args[0] == "deny" || args[0] == "ask"):
		if err := engine.SetRule(scope, projectPath, agent, permissions.Permission(args[1]), permissions.Effect(args[0])); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s set to %s at %s scope\n", args[1], args[0], scope)
	case len(args) == 1 && args[0] == "reset":
		if err := engine.Reset(scope, projectPath, agent); err != nil {
			return err
		}
		fmt.Fprintf(out, "Policy reset at %s scope\n", scope)
	default:
		return fmt.Errorf("usage: gasa permissions [show] [--agent ID] [--project PATH] | set-profile PROFILE | allow|deny|ask PERMISSION [--agent ID] [--project PATH] | reset [--agent ID] [--project PATH]")
	}
	return nil
}
