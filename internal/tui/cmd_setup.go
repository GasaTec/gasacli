package tui

func init() {
	Register(Command{Name: "/setup", Summary: "command.setup", Usage: "/setup", Category: "general", Planned: false, IssueRef: ""})
}
