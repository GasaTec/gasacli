// Package permissions evaluates and persists the canonical GasaCLI permission policy.
package permissions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/trust"
	"gopkg.in/yaml.v3"
)

type Permission string
type Effect string
type Scope string

const (
	Allow Effect = "allow"
	Ask   Effect = "ask"
	Deny  Effect = "deny"

	UserScope    Scope = "user"
	ProjectScope Scope = "project"
	AgentScope   Scope = "agent"
	ProfileScope Scope = "profile default"
)

var Canonical = []Permission{"read_project", "write_project", "run_tests", "execute_commands", "install_dependencies", "network_access", "git_mutations", "read_vault", "write_vault", "issue_write", "plugin_install", "secret_access"}

var profiles = map[string]map[Permission]Effect{
	"restricted": {"read_project": Allow, "write_project": Ask, "run_tests": Ask, "execute_commands": Deny, "install_dependencies": Deny, "network_access": Deny, "git_mutations": Deny, "read_vault": Deny, "write_vault": Deny, "issue_write": Deny, "plugin_install": Deny, "secret_access": Deny},
	"balanced":   {"read_project": Allow, "write_project": Allow, "run_tests": Allow, "execute_commands": Ask, "install_dependencies": Ask, "network_access": Ask, "git_mutations": Ask, "read_vault": Ask, "write_vault": Ask, "issue_write": Ask, "plugin_install": Ask, "secret_access": Deny},
	"autonomous": {"read_project": Allow, "write_project": Allow, "run_tests": Allow, "execute_commands": Allow, "install_dependencies": Ask, "network_access": Ask, "git_mutations": Ask, "read_vault": Ask, "write_vault": Ask, "issue_write": Ask, "plugin_install": Ask, "secret_access": Ask},
}

type Policy struct {
	Profile string                `yaml:"profile"`
	Rules   map[Permission]Effect `yaml:"rules,omitempty"`
}
type Request struct {
	Agent   string     `json:"agent_id"`
	Action  Permission `json:"permission"`
	Target  string     `json:"target,omitempty"`
	Project string     `json:"project,omitempty"`
	RunID   string     `json:"run_id,omitempty"`
}
type Decision struct {
	Effect Effect     `json:"effect"`
	Reason string     `json:"reason"`
	Rule   Permission `json:"rule"`
	Scope  Scope      `json:"scope"`
}
type Rule struct {
	Permission Permission `json:"permission"`
	Effect     Effect     `json:"effect"`
	Scope      Scope      `json:"scope"`
	Override   Effect     `json:"override,omitempty"`
}

type Engine struct {
	Home  string
	Trust *trust.Store
}

func New(home string, trustStore *trust.Store) *Engine { return &Engine{Home: home, Trust: trustStore} }
func Default() (*Engine, error) {
	home, err := config.Home()
	if err != nil {
		return nil, err
	}
	return New(home, trust.NewStore(home)), nil
}

func ValidPermission(p Permission) bool {
	for _, candidate := range Canonical {
		if candidate == p {
			return true
		}
	}
	return false
}
func ValidEffect(e Effect) bool { return e == Allow || e == Ask || e == Deny }

func (e *Engine) userPolicy() (Policy, error) {
	p, err := loadPolicy(filepath.Join(e.Home, "permissions.yaml"))
	if err != nil {
		return Policy{}, err
	}
	if p.Profile == "" {
		p.Profile = "balanced"
		if state, err := config.LoadState(e.Home); err != nil {
			return Policy{}, err
		} else if profiles[state.Onboarding.Permissions] != nil {
			p.Profile = state.Onboarding.Permissions
		}
	}
	return p, nil
}

func (e *Engine) Decide(ctx context.Context, req Request) Decision {
	decision := Decision{Effect: Deny, Reason: "unknown permission fails closed", Rule: req.Action, Scope: ProfileScope}
	if ctx.Err() != nil {
		decision.Reason = "request context is cancelled"
		return decision
	}
	if !ValidPermission(req.Action) {
		return decision
	}
	user, err := e.userPolicy()
	if err != nil {
		decision.Reason = "invalid user policy: " + err.Error()
		return decision
	}
	effect := profiles[user.Profile][req.Action]
	scope := ProfileScope
	if override, ok := user.Rules[req.Action]; ok {
		effect, scope = override, UserScope
	}
	if req.Project != "" {
		if e.Trust == nil {
			decision.Reason = "project trust cannot be verified"
			return decision
		}
		trusted, err := e.Trust.IsTrusted(req.Project)
		if err != nil || !trusted {
			decision.Reason = "project is not trusted"
			decision.Scope = ProjectScope
			return decision
		}
		project, err := loadPolicy(filepath.Join(req.Project, ".gasa", "permissions.yaml"))
		if err != nil {
			decision.Reason = "invalid project policy: " + err.Error()
			return decision
		}
		if override, ok := project.Rules[req.Action]; ok {
			effect = restrictive(effect, override)
			scope = ProjectScope
		}
	}
	if req.Agent != "" {
		agent, err := loadPolicy(filepath.Join(e.Home, "agents", req.Agent, "permissions.yaml"))
		if err != nil {
			decision.Reason = "invalid agent policy: " + err.Error()
			return decision
		}
		if override, ok := agent.Rules[req.Action]; ok {
			effect = restrictive(effect, override)
			scope = AgentScope
		}
	}
	decision.Effect, decision.Scope, decision.Reason = effect, scope, "effective policy"
	return decision
}

func restrictive(a, b Effect) Effect {
	rank := map[Effect]int{Allow: 0, Ask: 1, Deny: 2}
	if !ValidEffect(a) || !ValidEffect(b) || rank[b] > rank[a] {
		return b
	}
	return a
}

func (e *Engine) Explain(ctx context.Context, project, agent string) []Rule {
	out := make([]Rule, 0, len(Canonical))
	for _, p := range Canonical {
		d := e.Decide(ctx, Request{Action: p, Project: project, Agent: agent})
		out = append(out, Rule{Permission: p, Effect: d.Effect, Scope: d.Scope})
	}
	return out
}

func loadPolicy(path string) (Policy, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return Policy{Rules: map[Permission]Effect{}}, nil
	}
	if err != nil {
		return Policy{}, fmt.Errorf("open policy %s: %w", path, err)
	}
	defer f.Close()
	var policy Policy
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err := dec.Decode(&policy); err != nil {
		return Policy{}, fmt.Errorf("decode policy %s: %w", path, err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Policy{}, fmt.Errorf("decode policy trailing document %s: %w", path, err)
		}
		return Policy{}, fmt.Errorf("policy %s must contain one YAML document", path)
	}
	if policy.Profile != "" && profiles[policy.Profile] == nil {
		return Policy{}, fmt.Errorf("invalid profile %q", policy.Profile)
	}
	if policy.Rules == nil {
		policy.Rules = map[Permission]Effect{}
	}
	for p, effect := range policy.Rules {
		if !ValidPermission(p) {
			return Policy{}, fmt.Errorf("unknown permission %q", p)
		}
		if !ValidEffect(effect) {
			return Policy{}, fmt.Errorf("invalid effect %q for %s", effect, p)
		}
	}
	return policy, nil
}

func (e *Engine) Save(scope Scope, project, agent string, policy Policy) error {
	if policy.Profile != "" && profiles[policy.Profile] == nil {
		return fmt.Errorf("invalid profile %q", policy.Profile)
	}
	for p, v := range policy.Rules {
		if !ValidPermission(p) || !ValidEffect(v) {
			return fmt.Errorf("invalid policy rule %q: %q", p, v)
		}
	}
	var path string
	switch scope {
	case UserScope:
		path = filepath.Join(e.Home, "permissions.yaml")
	case ProjectScope:
		if project == "" || e.Trust == nil {
			return errors.New("project policy requires a trusted project")
		}
		trusted, err := e.Trust.IsTrusted(project)
		if err != nil || !trusted {
			return errors.New("project policy requires a trusted project")
		}
		path = filepath.Join(project, ".gasa", "permissions.yaml")
	case AgentScope:
		if agent == "" {
			return errors.New("agent policy requires an agent id")
		}
		path = filepath.Join(e.Home, "agents", agent, "permissions.yaml")
	default:
		return fmt.Errorf("invalid policy scope %q", scope)
	}
	return writePolicy(path, policy)
}

func writePolicy(path string, policy Policy) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(policy)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".permissions-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return nil
}

func (e *Engine) SetProfile(profile string) error {
	if profiles[profile] == nil {
		return fmt.Errorf("invalid profile %q", profile)
	}
	p, err := e.userPolicy()
	if err != nil {
		return err
	}
	p.Profile = profile
	return e.Save(UserScope, "", "", p)
}
func (e *Engine) SetRule(scope Scope, project, agent string, permission Permission, effect Effect) error {
	if !ValidPermission(permission) || !ValidEffect(effect) {
		return fmt.Errorf("invalid permission or effect")
	}
	var p Policy
	var err error
	path := filepath.Join(e.Home, "permissions.yaml")
	if scope == ProjectScope {
		path = filepath.Join(project, ".gasa", "permissions.yaml")
	}
	if scope == AgentScope {
		path = filepath.Join(e.Home, "agents", agent, "permissions.yaml")
	}
	p, err = loadPolicy(path)
	if err != nil {
		return err
	}
	p.Rules[permission] = effect
	return e.Save(scope, project, agent, p)
}
func (e *Engine) Reset(scope Scope, project, agent string) error {
	p := Policy{Rules: map[Permission]Effect{}}
	if scope == UserScope {
		p.Profile = "balanced"
	}
	return e.Save(scope, project, agent, p)
}

func SortedPermissions() []Permission {
	out := append([]Permission(nil), Canonical...)
	sort.Slice(out, func(i, j int) bool { return strings.Compare(string(out[i]), string(out[j])) < 0 })
	return out
}
