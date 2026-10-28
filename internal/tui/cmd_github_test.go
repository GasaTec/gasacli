package tui

import (
	"context"
	"strings"
	"testing"
)

type githubCommandContext struct{ trusted bool }

func (c githubCommandContext) ProjectPath() string               { return "" }
func (c githubCommandContext) Trusted() bool                     { return c.trusted }
func (githubCommandContext) ConfigHome() string                  { return "" }
func (githubCommandContext) DiscoveryResults() any               { return nil }
func (githubCommandContext) Storage() any                        { return nil }
func (githubCommandContext) Notify(string)                       {}
func (githubCommandContext) AppendMessage(string)                {}
func (githubCommandContext) Emit(string)                         {}
func (githubCommandContext) Ask(PermissionRequest) (bool, error) { return false, nil }

func TestGitHubCommandRegisteredAndRequiresTrust(t *testing.T) {
	command, ok := lookupCommand("/github")
	if !ok || command.Handler == nil || command.Planned {
		t.Fatalf("GitHub command registration = %#v, found=%t", command, ok)
	}
	if _, err := command.Handler(context.Background(), githubCommandContext{}, []string{"issues"}); err == nil || !strings.Contains(err.Error(), "trusted project") {
		t.Fatalf("untrusted GitHub command error = %v", err)
	}
}
