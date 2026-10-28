package tui

func init() {
	Register(Command{Name: "/status", Summary: "command.status", Usage: "/status", Category: "general", Planned: false, IssueRef: ""})
}
