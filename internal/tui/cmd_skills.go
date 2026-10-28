package tui

func init() {
	Register(Command{Name: "/skills", Summary: "command.skills", Usage: "/skills", Category: "general", Planned: true, IssueRef: "#11"})
}
