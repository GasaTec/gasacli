package tui

func init() {
	Register(Command{Name: "/backup", Summary: "command.backup", Usage: "/backup", Category: "general", Planned: true, IssueRef: "#11"})
}
