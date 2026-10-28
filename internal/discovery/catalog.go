// Package discovery locates known agent CLIs and probes their versions and
// documented non-interactive readiness commands without invoking a shell.
package discovery

// Agent describes a trusted, code-defined CLI probe. Do not populate catalogs
// with user input: Binaries and VersionFlag are executable probe configuration.
type Agent struct {
	ID          string
	Name        string
	Binaries    []string
	VersionFlag string   // Empty means --version.
	RequireText string   // If set, a successful probe must contain this text, case-insensitively.
	ReadyArgs   []string // Documented non-interactive authentication status command; nil means unknown.
}

// DefaultCatalog returns a fresh catalog in display order. Claude Code and Codex
// are MVP agents; the remaining entries are discovered as other CLIs only.
func DefaultCatalog() []Agent {
	return []Agent{
		{ID: "claude", Name: "Claude Code", Binaries: []string{"claude"}},
		{ID: "codex", Name: "Codex", Binaries: []string{"codex"}, ReadyArgs: []string{"login", "status"}},
		{ID: "gemini", Name: "Gemini CLI", Binaries: []string{"gemini"}},
		{ID: "cursor-agent", Name: "Cursor CLI", Binaries: []string{"cursor-agent", "agent"}, RequireText: "cursor"},
		{ID: "opencode", Name: "OpenCode", Binaries: []string{"opencode"}, ReadyArgs: []string{"auth", "list", "--format", "json"}},
		{ID: "orca", Name: "Orca", Binaries: []string{"orca"}},
		{ID: "kimi", Name: "Kimi Code CLI", Binaries: []string{"kimi"}},
		{ID: "gh", Name: "GitHub CLI", Binaries: []string{"gh"}},
	}
}
