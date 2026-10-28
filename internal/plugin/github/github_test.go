package github

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
)

type fakeRunner struct {
	outputs  map[string][]byte
	commands [][]string
	err      error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	command := append([]string{name}, args...)
	f.commands = append(f.commands, command)
	if f.err != nil {
		return nil, f.err
	}
	if name == "git" && len(args) >= 2 && args[len(args)-2] == "remote" && args[len(args)-1] == "-v" {
		return []byte("origin https://github.com/acme/project.git (fetch)\n"), nil
	}
	return f.outputs[strings.Join(command, " ")], nil
}

type fakeApprover struct {
	decision permissions.Decision
	requests []permissions.Request
}

func (a *fakeApprover) Ask(_ context.Context, request permissions.Request) (permissions.Decision, error) {
	a.requests = append(a.requests, request)
	return a.decision, nil
}

func setupClient(t *testing.T, runner Runner, approver Approver) (*Client, string, *permissions.Engine) {
	t.Helper()
	project, home := t.TempDir(), t.TempDir()
	trustStore := trust.NewStore(home)
	if err := trustStore.Grant(project); err != nil {
		t.Fatal(err)
	}
	policy := permissions.New(home, trustStore)
	if err := policy.SetProfile("balanced"); err != nil {
		t.Fatal(err)
	}
	client, err := New(Options{Project: project, Home: home, Trust: trustStore, Policy: policy, Approver: approver, Runner: runner})
	if err != nil {
		t.Fatal(err)
	}
	return client, project, policy
}

func TestRepositoryParsing(t *testing.T) {
	for _, input := range []string{
		"origin https://github.com/acme/project.git (fetch)\norigin https://github.com/acme/project.git (push)",
		"origin git@github.com:acme/project.git (fetch)",
		"origin ssh://git@github.com/acme/project.git (fetch)",
	} {
		got, err := parseRepository(input)
		if err != nil || got != "acme/project" {
			t.Fatalf("parseRepository(%q) = %q, %v", input, got, err)
		}
	}
	if _, err := parseRepository("origin https://example.com/acme/project.git (fetch)"); err == nil {
		t.Fatal("accepted non-GitHub remote")
	}
}

func TestOptInAndListIssues(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "issues.json"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	runner.outputs = map[string][]byte{}
	client, _, _ := setupClient(t, runner, nil)
	if _, err := client.ListIssues(context.Background(), "open", 30); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("unopted project error = %v", err)
	}
	if len(runner.commands) != 1 {
		t.Fatalf("unopted operation ran more commands: %#v", runner.commands)
	}
	if err := client.SetOptIn(true); err != nil {
		t.Fatal(err)
	}
	optedIn, err := client.OptedIn()
	if err != nil || !optedIn {
		t.Fatalf("opt-in = %t, %v", optedIn, err)
	}
	runner.outputs["gh issue list --repo acme/project --state open --limit 30 --json number,title,body,state,url,labels,assignees"] = fixture
	issues, err := client.ListIssues(context.Background(), "open", 30)
	if err != nil || len(issues) != 1 || issues[0].Number != 13 {
		t.Fatalf("issues=%#v, err=%v", issues, err)
	}
	goal := IssueToGoal(issues[0])
	if goal.Title != "Add GitHub issue integration" || !reflect.DeepEqual(goal.AcceptanceCriteria, []string{"Require opt-in", "Keep tokens private"}) {
		t.Fatalf("goal=%#v", goal)
	}
	runner.outputs["gh issue list --repo acme/project --state closed --limit 10 --assignee gabriel --label bug --search is:closed --json number,title,body,state,url,labels,assignees"] = []byte("[]")
	filters, err := ParseIssueFilters([]string{"closed", "--limit", "10", "--assignee", "gabriel", "--label", "bug", "--search", "is:closed"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListIssuesWithFilters(context.Background(), filters); err != nil {
		t.Fatal(err)
	}
}

func TestWriteApprovalPolicyAndDryRun(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{"gh issue comment 13 --repo acme/project --body progress": []byte("https://github.com/acme/project/issues/13#issuecomment-1")}}
	approver := &fakeApprover{decision: permissions.Decision{Effect: permissions.Deny}}
	client, _, policy := setupClient(t, runner, approver)
	if err := client.SetOptIn(true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Comment(context.Background(), 13, "progress"); err == nil || !strings.Contains(err.Error(), "not approved") {
		t.Fatalf("denied write error=%v", err)
	}
	if len(approver.requests) != 1 || !strings.Contains(approver.requests[0].Target, `"--body" "progress"`) {
		t.Fatalf("approval did not show exact command: %#v", approver.requests)
	}
	for _, command := range runner.commands {
		if strings.Contains(strings.Join(command, " "), "issue comment") {
			t.Fatal("denied write executed gh")
		}
	}
	if err := policy.SetRule(permissions.UserScope, "", "", "issue_write", permissions.Deny); err != nil {
		t.Fatal(err)
	}
	approver.decision.Effect = permissions.Allow
	if _, err := client.Comment(context.Background(), 13, "progress"); err == nil || !strings.Contains(err.Error(), "denied by policy") {
		t.Fatalf("policy deny error=%v", err)
	}
	if err := policy.SetRule(permissions.UserScope, "", "", "issue_write", permissions.Ask); err != nil {
		t.Fatal(err)
	}
	client.options.DryRun = true
	result, err := client.Comment(context.Background(), 13, "progress")
	if err != nil || !strings.Contains(result, "DRY RUN") {
		t.Fatalf("dry run=%q, %v", result, err)
	}
	if len(approver.requests) != 1 {
		t.Fatalf("dry run requested approval: %d", len(approver.requests))
	}
}

func TestAuthParseAndRedaction(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{"gh auth status": []byte("Logged in to github.com account gabriel (keyring)\nToken scopes: 'gist'\nToken: gho_very_secret\n")}}
	client, _, _ := setupClient(t, runner, nil)
	status, err := client.Auth(context.Background())
	if err != nil || !status.LoggedIn || status.Account != "gabriel" || !reflect.DeepEqual(status.Scopes, []string{"gist"}) || !reflect.DeepEqual(status.Missing, []string{"repo"}) {
		t.Fatalf("auth=%#v, %v", status, err)
	}
	data, _ := json.Marshal(status)
	if strings.Contains(string(data), "very_secret") {
		t.Fatal("auth result leaked token")
	}
	if got := redactIssues([]Issue{{Body: "Authorization: Bearer abc123"}})[0].Body; strings.Contains(got, "abc123") {
		t.Fatalf("body was not redacted: %q", got)
	}
}

func TestAuthUsesFakeGHExecutable(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "fake-gh")
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	build := exec.Command("go", "build", "-o", name, "./testdata/fake-gh")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake gh: %v\n%s", err, output)
	}
	argsPath := filepath.Join(dir, "args.txt")
	t.Setenv("GASA_TEST_GH_ARGS", argsPath)
	t.Setenv("GASA_TEST_GH_OUTPUT", "Logged in to github.com account fixture (keyring)\nToken scopes: 'repo'\nToken: gho_fake_test_secret\n")
	client, _, _ := setupClient(t, &fakeRunner{outputs: map[string][]byte{}}, nil)
	client.options.GH, client.options.Runner = name, ExecRunner{}
	status, err := client.Auth(context.Background())
	if err != nil || !status.LoggedIn || status.Account != "fixture" || !reflect.DeepEqual(status.Scopes, []string{"repo"}) {
		t.Fatalf("auth=%#v, %v", status, err)
	}
	if _, err := os.Stat(argsPath); err != nil {
		t.Fatal("gh auth status was not invoked")
	}
	data, err := os.ReadFile(argsPath)
	if err != nil || string(data) != "auth\nstatus" {
		t.Fatalf("fake gh args=%q, %v", data, err)
	}
}

func TestWriteRequiresApprover(t *testing.T) {
	runner := &fakeRunner{}
	client, _, _ := setupClient(t, runner, nil)
	if err := client.SetOptIn(true); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Comment(context.Background(), 1, "hello"); err == nil || !strings.Contains(err.Error(), "requires an interactive approver") {
		t.Fatalf("missing approver error=%v", err)
	}
}

func TestCommentRedactsCredentialBeforeApprovalAndSend(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{}}
	approver := &fakeApprover{decision: permissions.Decision{Effect: permissions.Allow}}
	client, _, _ := setupClient(t, runner, approver)
	if err := client.SetOptIn(true); err != nil {
		t.Fatal(err)
	}
	token := "ghp_" + strings.Repeat("a", 36)
	body := "credential " + token + " " + strings.Repeat("tail", 180)
	if _, err := client.Comment(context.Background(), 13, body); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(approver.requests[0].Target, token) {
		t.Fatal("approval request leaked credential")
	}
	if len(approver.requests[0].Target) <= 512 || !strings.Contains(approver.requests[0].Target, strings.Repeat("tail", 180)) {
		t.Fatal("approval request did not show the complete comment body")
	}
	var sent string
	for _, command := range runner.commands {
		sent += strings.Join(command, " ")
	}
	if strings.Contains(sent, token) || !strings.Contains(sent, "[REDACTED]") {
		t.Fatalf("sent command was not safely redacted: %q", sent)
	}
}

func TestRunnerErrorsAreRedacted(t *testing.T) {
	err := errors.New("gh failed with token=abc123")
	runner := &fakeRunner{outputs: map[string][]byte{"git -C PROJECT remote -v": []byte("origin https://github.com/acme/project.git")}, err: err}
	client, _, _ := setupClient(t, runner, nil)
	if _, err := client.Repository(context.Background()); err == nil || strings.Contains(err.Error(), "abc123") {
		t.Fatalf("unsafe error: %v", err)
	}
}
