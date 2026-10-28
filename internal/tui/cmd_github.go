package tui

func init() {
	Register(Command{Name: "/github", Summary: "command.github", Usage: "/github", Category: "general", Planned: true, IssueRef: "#11"})
}
