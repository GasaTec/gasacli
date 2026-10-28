package tui

func init() {
	Register(Command{Name: "/runs", Summary: "command.runs", Usage: "/runs", Category: "general", Handler: runsCommand})
}
