package tui

func init() {
	Register(Command{Name: "/cancel", Summary: "command.cancel", Usage: "/cancel [run-id]", Category: "general", Handler: cancelCommand})
}
