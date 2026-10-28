package permissions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GasaTec/gasacli/internal/trust"
)

func TestProfileMatrix(t *testing.T) {
	want := map[string][]Effect{
		"restricted": {Allow, Ask, Ask, Deny, Deny, Deny, Deny, Deny, Deny, Deny, Deny, Deny},
		"balanced":   {Allow, Allow, Allow, Ask, Ask, Ask, Ask, Ask, Ask, Ask, Ask, Deny},
		"autonomous": {Allow, Allow, Allow, Allow, Ask, Ask, Ask, Ask, Ask, Ask, Ask, Ask},
	}
	for profile, effects := range want {
		t.Run(profile, func(t *testing.T) {
			home := t.TempDir()
			e := New(home, nil)
			if err := e.Save(UserScope, "", "", Policy{Profile: profile}); err != nil {
				t.Fatal(err)
			}
			for i, p := range Canonical {
				d := e.Decide(context.Background(), Request{Action: p})
				if d.Effect != effects[i] {
					t.Errorf("%s: got %s want %s", p, d.Effect, effects[i])
				}
			}
		})
	}
}

func TestNarrowScopesNeverElevate(t *testing.T) {
	for _, user := range []Effect{Allow, Ask, Deny} {
		for _, project := range []Effect{Allow, Ask, Deny} {
			for _, agent := range []Effect{Allow, Ask, Deny} {
				t.Run(string(user)+"/"+string(project)+"/"+string(agent), func(t *testing.T) {
					home := t.TempDir()
					root := filepath.Join(t.TempDir(), "project")
					if err := os.Mkdir(root, 0700); err != nil {
						t.Fatal(err)
					}
					store := trust.NewStore(home)
					if err := store.Grant(root); err != nil {
						t.Fatal(err)
					}
					e := New(home, store)
					p := Policy{Profile: "autonomous", Rules: map[Permission]Effect{"execute_commands": user}}
					if err := e.Save(UserScope, "", "", p); err != nil {
						t.Fatal(err)
					}
					if err := e.Save(ProjectScope, root, "", Policy{Rules: map[Permission]Effect{"execute_commands": project}}); err != nil {
						t.Fatal(err)
					}
					if err := e.Save(AgentScope, root, "agent-1", Policy{Rules: map[Permission]Effect{"execute_commands": agent}}); err != nil {
						t.Fatal(err)
					}
					got := e.Decide(context.Background(), Request{Action: "execute_commands", Project: root, Agent: "agent-1"}).Effect
					if rank[got] < rank[user] {
						t.Fatalf("effective %s is more permissive than user %s", got, user)
					}
					want := restrictive(restrictive(user, project), agent)
					if got != want {
						t.Fatalf("got %s want %s", got, want)
					}
				})
			}
		}
	}
}

var rank = map[Effect]int{Allow: 0, Ask: 1, Deny: 2}

func TestUntrustedProjectCannotApplyProjectPolicy(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	e := New(home, trust.NewStore(home))
	d := e.Decide(context.Background(), Request{Action: "read_project", Project: project})
	if d.Effect != Deny {
		t.Fatalf("untrusted project decision=%+v", d)
	}
}

func TestProjectPolicyAndTargetBoundary(t *testing.T) {
	home, project, outside := t.TempDir(), t.TempDir(), t.TempDir()
	store := trust.NewStore(home)
	if err := store.Grant(project); err != nil {
		t.Fatal(err)
	}
	e := New(home, store)
	if err := e.SetRule(ProjectScope, project, "", "read_project", Deny); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(project, "gasacli", "permissions.yaml")
	if _, err := os.Stat(policyPath); err != nil {
		t.Fatalf("project policy path missing: %v", err)
	}
	if got := e.Decide(context.Background(), Request{Action: "read_project", Project: project, Target: "inside.txt"}).Effect; got != Deny {
		t.Fatalf("project rule ignored: %s", got)
	}
	if got := e.Decide(context.Background(), Request{Action: "read_project", Project: project, Target: filepath.Join(outside, "secret.txt")}); got.Effect != Deny || got.Reason != "target is outside the trusted project boundary" {
		t.Fatalf("outside target decision=%+v", got)
	}
}

func TestProjectTargetRejectsSymlinkEscape(t *testing.T) {
	home, project, outside := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(project, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := trust.NewStore(home)
	if err := store.Grant(project); err != nil {
		t.Fatal(err)
	}
	d := New(home, store).Decide(context.Background(), Request{Action: "write_project", Project: project, Target: filepath.Join("escape", "file.txt")})
	if d.Effect != Deny {
		t.Fatalf("symlink escape decision=%+v", d)
	}
}

func TestAgentIDCannotEscapePolicyDirectory(t *testing.T) {
	e := New(t.TempDir(), nil)
	for _, agent := range []string{"../other", "..", "a\\b"} {
		if err := e.Save(AgentScope, "", agent, Policy{}); err == nil {
			t.Errorf("accepted unsafe agent id %q", agent)
		}
	}
	d := e.Decide(context.Background(), Request{Action: "read_project", Agent: "../other"})
	if d.Effect != Deny || d.Reason != "invalid agent identifier" {
		t.Fatalf("unsafe agent decision=%+v", d)
	}
}

func TestPolicyRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "permissions.yaml")
	if err := os.WriteFile(path, []byte("profile: balanced\nrules:\n  invented: allow\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPolicy(path); err == nil {
		t.Fatal("unknown permission accepted")
	}
}

func TestOnboardingProfileIsUserDefault(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "state.json"), []byte(`{"onboarding":{"permissions":"restricted"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	d := New(home, nil).Decide(context.Background(), Request{Action: "execute_commands"})
	if d.Effect != Deny {
		t.Fatalf("onboarding profile ignored: %+v", d)
	}
}
