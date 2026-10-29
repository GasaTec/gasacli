package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/GasaTec/gasacli/internal/sessions"
)

type liveSendTestContext struct {
	fakeHandlerContext
	actions []string
	deny    string
	fail    string
	sent    bool
}

func (c *liveSendTestContext) Ask(req PermissionRequest) (bool, error) {
	c.actions = append(c.actions, req.Action)
	if req.Action == c.fail {
		return false, errors.New("approval unavailable")
	}
	return req.Action != c.deny, nil
}
func (*liveSendTestContext) listSessions() []sessions.Session {
	return []sessions.Session{{ID: "codex-12345678", Agent: "Codex"}}
}

func TestCanceledSessionCommandsDoNotRequestApproval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &liveSendTestContext{}
	if _, err := listSessionsCommand(ctx, c, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("list error = %v", err)
	}
	if _, err := sendSessionCommand(ctx, c, []string{"codex-12345678", "message"}); !errors.Is(err, context.Canceled) || len(c.actions) != 0 || c.sent {
		t.Fatalf("send error=%v actions=%v sent=%v", err, c.actions, c.sent)
	}
}

func TestFilterSessionsByProjectUsesDirectoryIdentity(t *testing.T) {
	project := t.TempDir()
	other := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(project, alias); err != nil {
		// Symlink creation may require privileges on Windows; lexical aliases
		// still exercise canonicalization there.
		alias = filepath.Join(project, ".")
	}
	items := []sessions.Session{{ID: "match", CWD: alias}, {ID: "other", CWD: other}, {ID: "empty"}}
	got, err := filterSessionsByProject(context.Background(), items, project)
	if err != nil || len(got) != 1 || got[0].ID != "match" {
		t.Fatalf("sessions=%v err=%v", got, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := filterSessionsByProject(ctx, items, project); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled filter: %v", err)
	}
}
func (c *liveSendTestContext) sendToSession(_ context.Context, _ sessions.SendRequest) (<-chan sessions.Event, error) {
	c.sent = true
	return nil, errors.New("audit failed")
}

func TestSendRequiresBothPermissionsAndFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, deny, fail string
		want             []string
		sent             bool
	}{
		{"execution denied", "execute_commands", "", []string{"execute_commands"}, false},
		{"network denied", "network_access", "", []string{"execute_commands", "network_access"}, false},
		{"network approval error", "", "network_access", []string{"execute_commands", "network_access"}, false},
		{"sender audit failure", "", "", []string{"execute_commands", "network_access"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &liveSendTestContext{deny: tc.deny, fail: tc.fail}
			result, err := sendSessionCommand(context.Background(), c, []string{"codex-12345678", "message"})
			if err != nil || result == "" || c.sent != tc.sent || !reflect.DeepEqual(c.actions, tc.want) {
				t.Fatalf("result=%q err=%v sent=%v actions=%v", result, err, c.sent, c.actions)
			}
		})
	}
}
