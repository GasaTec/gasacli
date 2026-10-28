package tui

func init() {
	Register(Command{Name: "/resume", Summary: "command.resume", Usage: "/resume [run-id]", Category: "general", Handler: resumeCommand})
}
