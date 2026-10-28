package tui

func init() {
	Register(Command{Name: "/exit", Summary: "command.exit", Usage: "/exit", Category: "general", Planned: false, IssueRef: ""})
}
