package tui

func init() {
	Register(Command{Name: "/resume", Summary: "command.resume", Usage: "/resume", Category: "general", Planned: true, IssueRef: "#11"})
}
