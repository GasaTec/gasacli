package tui

func init() {
	Register(Command{Name: "/config", Summary: "command.config", Usage: "/config", Category: "general", Planned: true, IssueRef: "#11"})
}
