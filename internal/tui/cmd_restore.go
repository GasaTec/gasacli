package tui

func init() {
	Register(Command{Name: "/restore", Summary: "command.restore", Usage: "/restore", Category: "general", Planned: true, IssueRef: "#11"})
}
