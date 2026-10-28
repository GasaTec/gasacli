// Package discovery locates known agent CLIs and probes their versions.
// Discovery never checks authentication or invokes a shell.
package discovery

// Agent describes a trusted, code-defined CLI probe. Do not populate catalogs
// with user input: Binaries and VersionFlag are executable probe configuration.
type Agent struct {
	ID          string
	Name        string
	Binaries    []string
	VersionFlag string // Empty means --version.
	RequireText string // If set, a successful probe must contain this text, case-insensitively.
}

// DefaultCatalog returns a fresh catalog in display order. Claude Code and Codex
// are MVP agents; the remaining entries are discovered as other CLIs only.
func DefaultCatalog() []Agent {
	return []Agent{
		{ID: "claude", Name: "Claude Code", Binaries: []string{"claude"}},
		{ID: "codex", Name: "Codex", Binaries: []string{"codex"}},
		{ID: "gemini", Name: "Gemini CLI", Binaries: []string{"gemini"}},
		{ID: "cursor-agent", Name: "Cursor CLI", Binaries: []string{"cursor-agent", "agent"}, RequireText: "cursor"},
		{ID: "opencode", Name: "OpenCode", Binaries: []string{"opencode"}},
		{ID: "orca", Name: "Orca", Binaries: []string{"orca"}},
		{ID: "kimi", Name: "Kimi Code CLI", Binaries: []string{"kimi"}},
		{ID: "gh", Name: "GitHub CLI", Binaries: []string{"gh"}},
	}
}
