package tui

func init() {
	Register(Command{Name: "/goal", Summary: "command.goal", Usage: "/goal", Category: "general", Planned: true, IssueRef: "#11"})
}
