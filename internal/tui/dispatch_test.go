package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/trust"
)

func TestHandlerDispatchStreamsParsesAndRecovers(t *testing.T) {
	pinTestLanguage(t, "en")
	registerTestCommand(t, Command{Name: "/test-dispatch-stream", Handler: func(_ context.Context, cc CommandContext, args []string) (string, error) {
		if !reflect.DeepEqual(args, []string{"two words", "three"}) {
			return "", errors.New("quoted arguments were not parsed")
		}
		cc.Emit("first")
		cc.Emit("second")
		return "finished", nil
	}})
	var chunks []string
	got, err := executeCommand(context.Background(), &Session{}, "/tmp/project", true, "/test-dispatch-stream \"two words\" three", func(line string) { chunks = append(chunks, line) }, nil)
	if err != nil || got != "finished" || !reflect.DeepEqual(chunks, []string{"first", "second"}) {
		t.Fatalf("result=%q err=%v chunks=%v", got, err, chunks)
	}
	fake := &fakeHandlerContext{}
	command, _ := lookupCommand("/test-dispatch-stream")
	if result, err := command.Handler(context.Background(), fake, []string{"two words", "three"}); err != nil || result != "finished" || !reflect.DeepEqual(fake.emitted, []string{"first", "second"}) {
		t.Fatalf("fake context result=%q err=%v emitted=%v", result, err, fake.emitted)
	}
	registerTestCommand(t, Command{Name: "/test-dispatch-panic", Handler: func(context.Context, CommandContext, []string) (string, error) { panic("unexpected") }})
	got, err = executeCommand(context.Background(), &Session{}, "/tmp/project", true, "/test-dispatch-panic", nil, nil)
	if got != "" || err == nil || !strings.Contains(err.Error(), "handler panic") {
		t.Fatalf("panic result=%q err=%v", got, err)
	}
}

type fakeHandlerContext struct{ emitted []string }

func (*fakeHandlerContext) ProjectPath() string                 { return "/tmp/project" }
func (*fakeHandlerContext) Trusted() bool                       { return true }
func (*fakeHandlerContext) ConfigHome() string                  { return "/tmp/gasacli" }
func (*fakeHandlerContext) DiscoveryResults() any               { return nil }
func (*fakeHandlerContext) Storage() any                        { return nil }
func (*fakeHandlerContext) Notify(string)                       {}
func (*fakeHandlerContext) AppendMessage(string)                {}
func (c *fakeHandlerContext) Emit(message string)               { c.emitted = append(c.emitted, message) }
func (*fakeHandlerContext) Ask(PermissionRequest) (bool, error) { return false, nil }

func TestHandlerDispatchCancellationAndPermissionDecisionLog(t *testing.T) {
	pinTestLanguage(t, "pt-BR")
	registerTestCommand(t, Command{Name: "/test-dispatch-cancel", Handler: func(ctx context.Context, _ CommandContext, _ []string) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := executeCommand(ctx, &Session{}, "/tmp/project", true, "/test-dispatch-cancel", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error=%v", err)
	}

	home, project := t.TempDir(), t.TempDir()
	store := trust.NewStore(home)
	if err := store.Grant(project); err != nil {
		t.Fatal(err)
	}
	registerTestCommand(t, Command{Name: "/test-dispatch-ask", Handler: func(_ context.Context, cc CommandContext, _ []string) (string, error) {
		approved, err := cc.Ask(PermissionRequest{Agent: "tester", Action: "write_vault", Target: "vault/item.md", ExactCommand: "write item"})
		return []string{"denied", "approved"}[boolIndex(approved)], err
	}})
	for _, approved := range []bool{false, true} {
		choice := ApprovalDeny
		if approved {
			choice = ApprovalOnce
		}
		got, err := executeCommand(context.Background(), &Session{ConfigHome: home, TrustStore: store}, project, true, "/test-dispatch-ask", nil, func(context.Context, PermissionRequest) (ApprovalChoice, error) { return choice, nil })
		if err != nil {
			t.Fatal(err)
		}
		want := "denied"
		if approved {
			want = "approved"
		}
		if got != want {
			t.Fatalf("approval=%v result=%q, want %q", approved, got, want)
		}
	}
	log, err := os.ReadFile(filepath.Join(home, "approvals", "events.jsonl"))
	if err != nil || strings.Count(string(log), "\"decision\":\"allow\"") != 1 || strings.Count(string(log), "\"decision\":\"deny\"") != 1 {
		t.Fatalf("approval log=%s err=%v", log, err)
	}
}

func TestBuiltInDiagnosticsReturnCancellation(t *testing.T) {
	for _, name := range []string{"/agents", "/doctor"} {
		t.Run(name, func(t *testing.T) {
			started := make(chan struct{})
			callback := func(ctx context.Context, _ io.Writer) {
				close(started)
				<-ctx.Done()
			}
			session := &Session{}
			if name == "/agents" {
				session.AgentsContext = callback
			} else {
				session.DoctorContext = callback
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := executeCommand(ctx, session, ".", true, name, nil, nil)
				result <- err
			}()
			<-started
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("diagnostics error = %v, want context.Canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("diagnostics handler did not stop after cancellation")
			}
		})
	}
}

func registerTestCommand(t *testing.T, command Command) {
	t.Helper()
	Register(command)
	t.Cleanup(func() {
		delete(commandRegistry, command.Name)
		for _, alias := range command.Aliases {
			delete(commandRegistry, alias)
		}
	})
}

func pinTestLanguage(t *testing.T, language string) {
	t.Helper()
	previous := i18n.Language()
	t.Setenv("GASACLI_LANG", language)
	i18n.Configure("")
	t.Cleanup(func() { i18n.SetLanguage(previous) })
}

func boolIndex(value bool) int {
	if value {
		return 1
	}
	return 0
}
