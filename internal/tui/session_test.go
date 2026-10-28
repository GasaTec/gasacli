package tui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/i18n"
	"github.com/GasaTec/gasacli/internal/trust"
)

func TestScriptedSession(t *testing.T) {
	i18n.SetLanguage("pt-BR")
	var out bytes.Buffer
	store := trust.NewStore(t.TempDir())
	project := t.TempDir()
	agents, doctor := 0, 0
	s := Session{In: strings.NewReader("y\n\n/help\n/agents\n/doctor\n/status\n/goal implement\nhello\n/unknown\n/exit\nignored\n"), Out: &out, ProjectPath: project, TrustStore: store,
		Agents: func(w io.Writer) { agents++; fmt.Fprintln(w, "agents callback") },
		Doctor: func(w io.Writer) { doctor++; fmt.Fprintln(w, "doctor callback") },
	}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[y/N]", "gasa> ", "/help", "/exit", "/quit", "/agents", "/doctor", "/status", "/goal", "/permissions", "/backup", "/restore", "/web", "planejado", i18n.T("command.model_unavailable"), i18n.T("command.unknown")} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %q", want, out.String())
		}
	}
	if agents != 1 || doctor != 1 {
		t.Fatalf("callbacks = %d, %d", agents, doctor)
	}
	if got, err := store.IsTrusted(project); err != nil || !got {
		t.Fatalf("trust = %v, %v", got, err)
	}
}

func TestSetupHookAndHelp(t *testing.T) {
	var out bytes.Buffer
	calls := 0
	s := Session{In: strings.NewReader("y\n/help\n/setup\n/exit\n"), Out: &out, ProjectPath: t.TempDir(), TrustStore: trust.NewStore(t.TempDir()), Setup: func(w io.Writer) error { calls++; fmt.Fprintln(w, "setup callback"); return nil }}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !strings.Contains(out.String(), "/setup") || !strings.Contains(out.String(), "setup callback") {
		t.Fatalf("calls=%d output=%s", calls, out.String())
	}
}

func TestTrustRefused(t *testing.T) {
	for _, input := range []string{"", "n\n/help\n", "\n", "yes\n"} {
		var out bytes.Buffer
		store := trust.NewStore(t.TempDir())
		project := t.TempDir()
		s := Session{In: strings.NewReader(input), Out: &out, ProjectPath: project, TrustStore: store}
		if err := s.Run(); err == nil || err.Error() != "project not trusted" {
			t.Fatalf("error = %v", err)
		}
		if strings.Contains(out.String(), "gasa> ") {
			t.Fatal("chat started after denial")
		}
		if got, err := store.IsTrusted(project); err != nil || got {
			t.Fatalf("denied = %v, %v", got, err)
		}
	}
}

func TestEOFAndQuit(t *testing.T) {
	for _, input := range []string{"", "y\n", "y\n/quit", "y\nhello"} {
		s := Session{In: strings.NewReader(input), Out: io.Discard, ProjectPath: t.TempDir(), TrustStore: trust.NewStore(t.TempDir())}
		err := s.Run()
		if input == "" {
			if err == nil || err.Error() != "project not trusted" {
				t.Fatalf("EOF before trust: %v", err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("input %q: %v", input, err)
		}
	}
}

func TestTrustedSessionSkipsQuestion(t *testing.T) {
	store := trust.NewStore(t.TempDir())
	project := t.TempDir()
	if err := store.Grant(project); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s := Session{In: strings.NewReader("/exit\n"), Out: &out, ProjectPath: project, TrustStore: store}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "[y/N]") {
		t.Fatal("asked again for trusted project")
	}
}

type brokenIO struct{}

func (brokenIO) Read([]byte) (int, error)  { return 0, errors.New("read failed") }
func (brokenIO) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestSessionIOErrors(t *testing.T) {
	s := Session{In: brokenIO{}, Out: io.Discard, ProjectPath: t.TempDir(), TrustStore: trust.NewStore(t.TempDir())}
	if err := s.Run(); err == nil || !strings.Contains(err.Error(), "read trust answer") {
		t.Fatalf("read error = %v", err)
	}
	s.In, s.Out = strings.NewReader("y\n"), brokenIO{}
	if err := s.Run(); err == nil || !strings.Contains(err.Error(), "write trust prompt") {
		t.Fatalf("write error = %v", err)
	}
}

func TestTrustedHooks(t *testing.T) {
	for _, answer := range []string{"", "n\n", "y\n"} {
		t.Run(fmt.Sprintf("answer=%q", answer), func(t *testing.T) {
			store := trust.NewStore(t.TempDir())
			project := t.TempDir()
			started, called := 0, 0
			var out bytes.Buffer
			session := Session{In: strings.NewReader(answer + "/web open\n/web status\n/exit\n"), Out: &out, ProjectPath: project, TrustStore: store,
				OnTrusted: func() error {
					trusted, err := store.IsTrusted(project)
					if err != nil || !trusted {
						t.Fatalf("hook before trust: %v %v", trusted, err)
					}
					started++
					return nil
				},
				Web: func(w io.Writer, args []string) error {
					if started != 1 {
						t.Fatal("web before startup")
					}
					if len(args) != 1 || (args[0] != "open" && args[0] != "status") {
						t.Fatalf("web args: %v", args)
					}
					called++
					return fmt.Errorf("dashboard unavailable")
				},
			}
			err := session.Run()
			if answer != "y\n" {
				if err == nil || started != 0 || called != 0 {
					t.Fatalf("denial: err=%v started=%d web=%d", err, started, called)
				}
			} else if err != nil || started != 1 || called != 2 || !strings.Contains(out.String(), "dashboard unavailable") {
				t.Fatalf("session: err=%v started=%d web=%d", err, started, called)
			}
		})
	}
}

func TestOnTrustedFailure(t *testing.T) {
	session := Session{In: strings.NewReader("y\n/exit\n"), Out: io.Discard, ProjectPath: t.TempDir(), TrustStore: trust.NewStore(t.TempDir()), OnTrusted: func() error { return errors.New("startup failed") }}
	if err := session.Run(); err == nil || err.Error() != "startup failed" {
		t.Fatalf("error: %v", err)
	}
}

func TestInputLimit(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"large supported line", "y\n" + strings.Repeat("x", 128*1024) + "\n/exit\n", ""},
		{"trust limit", strings.Repeat("x", 1024*1024) + "\n", "trust answer exceeds 1 MiB"},
		{"chat limit", "y\n" + strings.Repeat("x", 1024*1024) + "\n", "chat input exceeds 1 MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := Session{In: strings.NewReader(tc.input), Out: io.Discard, ProjectPath: t.TempDir(), TrustStore: trust.NewStore(t.TempDir())}
			err := session.Run()
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v, want %q", err, tc.want)
			}
		})
	}
}

func TestTrustPromptCanonicalPath(t *testing.T) {
	project := t.TempDir()
	canonical, err := trust.Canonical(project)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	session := Session{In: strings.NewReader("n\n"), Out: &out, ProjectPath: project, TrustStore: trust.NewStore(t.TempDir())}
	if err := session.Run(); err == nil {
		t.Fatal("expected denial")
	}
	if !strings.Contains(out.String(), "\""+canonical+"\"") {
		t.Fatalf("prompt does not identify canonical project: %s", out.String())
	}
}
