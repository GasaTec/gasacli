package tui

func init() {
	Register(Command{Name: "/agents", Summary: "command.agents", Usage: "/agents", Category: "general", Planned: false, IssueRef: ""})
}
