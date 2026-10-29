package tui

func init() {
	Register(Command{Name: "/runs", Summary: "command.runs", Usage: "/runs [run-id]", Category: "general", Handler: runsCommand})
}
