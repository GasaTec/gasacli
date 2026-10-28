package tui

func init() {
	Register(Command{Name: "/brain", Summary: "command.brain", Usage: "/brain", Category: "general", Planned: true, IssueRef: "#11"})
}
