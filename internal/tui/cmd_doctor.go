package tui

func init() {
	Register(Command{Name: "/doctor", Summary: "command.doctor", Usage: "/doctor", Category: "general", Planned: false, IssueRef: ""})
}
