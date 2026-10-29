package tui

func init() {
	Register(Command{Name: "/goal", Summary: "command.goal", Usage: "/goal [--agent <id>] <text>", Category: "general", Handler: goalCommand})
}
