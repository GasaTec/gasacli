package tui

func init() {
	Register(Command{Name: "/quit", Summary: "command.exit", Usage: "/quit", Category: "general"})
}
