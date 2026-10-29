# Adding a TUI command

The interactive shell automatically loads the most recent resumable local Claude
Code or Codex session for the trusted project and displays its agent and history.
`/sessions` lists available sessions and `/use <id>` switches the selected
session; `/send <id> <message>` sends directly. The automatic selection excludes
sessions marked as working. If no resumable session is found, creating a new
provider conversation is not yet supported. The session index is bounded, so
older projects may not appear if they fall outside its latest entries. Both resume the
provider CLI and require execution and network permission. The sender checks
project trust and records the decision before starting the provider. A session
must already exist; discovering a provider does not create a conversation.
`/history <id>` reads the selected session's available text and tool results
after verifying that it belongs to the trusted project.
`/use` is available only in the interactive shell. `/goal` continues to use the
shared orchestration flow.

The sidebar shows trusted projects and discovered agent status. `Ctrl+P` opens
the trusted-project picker, `Ctrl+B` hides the sidebar, `Ctrl+K` opens command
completion, and `Ctrl+T` changes the session's light/dark theme. Page Up and
Page Down scroll the transcript; Enter sends and Alt+Enter or Ctrl+J inserts a
newline. `Ctrl+C` cancels the foreground command; use `/cancel` for a running
background goal. Project switching is blocked while a command or project run
is active or a draft is present. The sidebar marks agent processes started by
the current GasaCLI run as running until their attempt finishes. Memory is the
GasaCLI process RSS on Linux or working set on Windows, excluding child
providers; other systems show Go heap instead. The footer displays tokens and
cost reported for the last agent only when supplied by its provider. It is not
a total for the session. Unknown values are omitted.

The navigation shortcuts can be configured in `config.yaml` under `tui.keys`
using the fields `projects`, `palette`, `sidebar` and `theme`. Their defaults
are `ctrl+p`, `ctrl+k`, `ctrl+b` and `ctrl+t`; function keys `f1`–`f12` are
also supported. Colliding or reserved bindings are rejected.

Create one `cmd_<name>.go` file in this package and call `Register` from its `init` function. Set the slash `Name`, aliases, localized `Summary` catalog key, usage, category, and optional `Planned`/`IssueRef` fields. The command palette, autocomplete candidates, and `/help` are generated from registered metadata.

Trust and approval dialogs use vertical lists: Up/Down moves the highlight, Space or Enter chooses, and Esc exits the trust prompt or denies approval. Left/Right, Tab, and unassigned letters do not change a choice. The approval list starts on Deny. Onboarding uses the same keys; its welcome screen has no list, so Enter starts and Esc exits. Plain onboarding remains numbered.

Handlers run on a worker goroutine in the interactive model and receive a cancellable context. Use `Emit` to stream complete transcript lines while work continues; return a string for the final response. Check `ctx.Done()` in long operations. `Ask` blocks until the user answers in the framed approval dialog (or a plain prompt in line-oriented mode); the permission policy is checked and the decision is recorded before the handler continues.

```go
package tui

import "context"

func init() {
	Register(Command{
		Name:     "/example",
		Summary:  "command.example",
		Category: "general",
		Handler: func(ctx context.Context, cc CommandContext, args []string) (string, error) {
			cc.Emit("Starting…")
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			default:
			}
			return "Done", nil
		},
	})
}
```

Keep handler tests deterministic by invoking the handler with a fake `CommandContext`; collect `Emit` values and return a chosen result from `Ask` rather than opening a terminal. For example, verify stream order and cancellation by sending a context cancellation while a fake handler waits on `ctx.Done()`.
