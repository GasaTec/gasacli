# Adding a TUI command

Create one `cmd_<name>.go` file in this package and call `Register` from its `init` function. Set the slash `Name`, aliases, localized `Summary` catalog key, usage, category, and optional `Planned`/`IssueRef` fields. The command palette, autocomplete candidates, and `/help` are generated from registered metadata.

Handlers run on a worker goroutine in the interactive model and receive a cancellable context. Use `Emit` to stream complete transcript lines while work continues; return a string for the final response. Check `ctx.Done()` in long operations. `Ask` blocks until the user answers in the framed approval dialog (or a y/N prompt in plain mode); the permission policy is checked and the decision is recorded before the handler continues.

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
