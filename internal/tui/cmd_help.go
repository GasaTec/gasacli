package tui

func init() {
	Register(Command{Name: "/help", Summary: "command.help", Usage: "/help", Category: "general", Planned: false, IssueRef: ""})
}
