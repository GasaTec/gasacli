package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeCommandContext struct {
	home  string
	ask   bool
	asked int
}

func (f *fakeCommandContext) ProjectPath() string                 { return "" }
func (f *fakeCommandContext) Trusted() bool                       { return true }
func (f *fakeCommandContext) ConfigHome() string                  { return f.home }
func (f *fakeCommandContext) DiscoveryResults() any               { return nil }
func (f *fakeCommandContext) Storage() any                        { return nil }
func (f *fakeCommandContext) Notify(string)                       {}
func (f *fakeCommandContext) AppendMessage(string)                {}
func (f *fakeCommandContext) Emit(string)                         {}
func (f *fakeCommandContext) Ask(PermissionRequest) (bool, error) { f.asked++; return f.ask, nil }

func TestBackupAndRestoreCommandHandlers(t *testing.T) {
	home := t.TempDir()
	ctx := &fakeCommandContext{home: home, ask: true}
	content := []byte("user config\n")
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := backupCommand(context.Background(), ctx, []string{"--scope", "config"}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(home, "backups", "gasacli-backup.zip")
	preview, err := restoreCommand(context.Background(), ctx, []string{archive, "--preview"})
	if err != nil || !strings.Contains(preview, "overwrite config.yaml") {
		t.Fatalf("preview=%q err=%v", preview, err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = restoreCommand(context.Background(), ctx, []string{archive}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil || string(got) != string(content) || ctx.asked != 1 {
		t.Fatalf("restored=%q asked=%d err=%v", got, ctx.asked, err)
	}
}

func TestRestoreCommandRequiresApproval(t *testing.T) {
	home := t.TempDir()
	ctx := &fakeCommandContext{home: home, ask: false}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := backupCommand(context.Background(), ctx, []string{"--scope", "config"}); err != nil {
		t.Fatal(err)
	}
	_, err := restoreCommand(context.Background(), ctx, []string{filepath.Join(home, "backups", "gasacli-backup.zip")})
	if err == nil || ctx.asked != 1 {
		t.Fatalf("restore err=%v asks=%d", err, ctx.asked)
	}
}
