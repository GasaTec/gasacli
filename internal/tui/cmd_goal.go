package tui

func init() {
	Register(Command{Name: "/goal", Summary: "command.goal", Usage: "/goal <text>", Category: "general", Handler: goalCommand})
}
