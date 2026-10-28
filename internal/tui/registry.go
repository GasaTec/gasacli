package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GasaTec/gasacli/internal/i18n"
)

// CommandContext exposes the session services command handlers may use.
type CommandContext interface {
	ProjectPath() string
	Trusted() bool
	ConfigHome() string
	DiscoveryResults() any
	Storage() any
	Notify(string)
	AppendMessage(string)
	Emit(string)
	Ask(PermissionRequest) (bool, error)
}

type PermissionRequest struct {
	Action string
	Target string
}

// CommandHandler returns the user-visible result for a command.
type CommandHandler func(context.Context, CommandContext, []string) (string, error)

// Command describes one registered slash command.
type Command struct {
	Name     string
	Aliases  []string
	Summary  string
	Usage    string
	Category string
	Planned  bool
	IssueRef string
	Handler  CommandHandler
}

var commandRegistry = map[string]Command{}

// Register adds a command and its aliases. Duplicate names and aliases panic at startup.
func Register(command Command) {
	command.Name = normalizeCommandName(command.Name)
	if command.Name == "" {
		panic("tui: command name is required")
	}
	if _, exists := commandRegistry[command.Name]; exists {
		panic("tui: duplicate command " + command.Name)
	}
	aliases := make(map[string]struct{}, len(command.Aliases))
	for _, alias := range command.Aliases {
		alias = normalizeCommandName(alias)
		if alias == "" || alias == command.Name {
			panic("tui: invalid alias for " + command.Name)
		}
		if _, exists := commandRegistry[alias]; exists {
			panic("tui: duplicate command alias " + alias)
		}
		if _, exists := aliases[alias]; exists {
			panic("tui: duplicate alias " + alias)
		}
		aliases[alias] = struct{}{}
	}
	if command.Planned && command.Handler == nil {
		planned := command
		command.Handler = func(context.Context, CommandContext, []string) (string, error) {
			return plannedCommandResult(planned), nil
		}
	}
	commandRegistry[command.Name] = command
	for _, alias := range command.Aliases {
		alias = normalizeCommandName(alias)
		commandRegistry[alias] = command
	}
}

func normalizeCommandName(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

func RegisteredCommands() []Command {
	commands := make([]Command, 0, len(commandRegistry))
	seen := make(map[string]struct{})
	for _, command := range commandRegistry {
		if _, ok := seen[command.Name]; ok {
			continue
		}
		seen[command.Name] = struct{}{}
		commands = append(commands, command)
	}
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name < commands[j].Name })
	return commands
}

func lookupCommand(name string) (Command, bool) {
	command, ok := commandRegistry[normalizeCommandName(name)]
	return command, ok
}

func commandSummary(command Command) string {
	if strings.HasPrefix(command.Summary, "command.") {
		return i18n.T(command.Summary)
	}
	return command.Summary
}

func helpText() string {
	var output strings.Builder
	output.WriteString("GasaCLI commands:\n")
	for _, command := range RegisteredCommands() {
		description := commandSummary(command)
		if command.Planned {
			description += " (" + i18n.T("command.planned", command.IssueRef) + ")"
		}
		fmt.Fprintf(&output, "  %-16s %s\n", command.Name, description)
		for _, alias := range command.Aliases {
			fmt.Fprintf(&output, "  %-16s %s\n", alias, description)
		}
	}
	return strings.TrimRight(output.String(), "\n")
}

func plannedCommandResult(command Command) string {
	if command.IssueRef == "" {
		return fmt.Sprintf("%s (%s).", command.Name, i18n.T("command.planned", ""))
	}
	return fmt.Sprintf("%s (%s). No action was started.", command.Name, i18n.T("command.planned", command.IssueRef))
}

func unknownCommandMessage(name string) string {
	name = normalizeCommandName(name)
	best, distance := "", len(name)
	for _, command := range RegisteredCommands() {
		candidate := editDistance(name, command.Name)
		if candidate < distance {
			best, distance = command.Name, candidate
		}
	}
	if best != "" && distance <= 2 {
		return i18n.T("command.suggestion", best)
	}
	return i18n.T("command.unknown")
}

func editDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	for i := range previous {
		previous[i] = i
	}
	for i, l := range left {
		current := make([]int, len(right)+1)
		current[0] = i + 1
		for j, r := range right {
			cost := 1
			if l == r {
				cost = 0
			}
			current[j+1] = min(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(right)]
}
