package tui

func init() {
	Register(Command{Name: "/prompts", Summary: "command.prompts", Usage: "/prompts", Category: "general", Planned: true, IssueRef: "#11"})
}
