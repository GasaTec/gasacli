package adapters

import "os/exec"

// StructuredText extracts user-visible text from a provider stream event.
func StructuredText(line []byte, eventType string) string { return structuredText(line, eventType) }

// ConfigureChildProcess and KillProcessTree share the executor's platform
// specific child-process lifecycle with session transports.
func ConfigureChildProcess(cmd *exec.Cmd) { configureProcessTree(cmd) }

func KillProcessTree(cmd *exec.Cmd) error { return terminateProcessTree(cmd) }
