package tui

func init() {
	Register(Command{Name: "/permissions", Summary: "command.permissions", Usage: "/permissions", Category: "general", Planned: true, IssueRef: "#31"})
}
