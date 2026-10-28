// Package github provides an optional, approval-gated adapter for the gh CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/GasaTec/gasacli/internal/config"
	"github.com/GasaTec/gasacli/internal/lifecycle"
	"github.com/GasaTec/gasacli/internal/permissions"
	"github.com/GasaTec/gasacli/internal/trust"
	"gopkg.in/yaml.v3"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return output.Bytes(), err
	}
	return output.Bytes(), nil
}

type Approver = permissions.Approver

type Options struct {
	Project  string
	Home     string
	Trust    *trust.Store
	Policy   *permissions.Engine
	Approver Approver
	Runner   Runner
	GH       string
	DryRun   bool
}

type Client struct{ options Options }

type repositoryOptIn struct {
	Plugins struct {
		GitHub bool `yaml:"github"`
	} `yaml:"plugins"`
}

type AuthStatus struct {
	LoggedIn bool     `json:"loggedIn"`
	Account  string   `json:"account,omitempty"`
	Scopes   []string `json:"scopes,omitempty"`
	Missing  []string `json:"missingScopes,omitempty"`
}

type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	Labels    []Label   `json:"labels"`
	Assignees []Actor   `json:"assignees"`
	Comments  []Comment `json:"comments,omitempty"`
}

type IssueFilters struct {
	State    string
	Assignee string
	Labels   []string
	Search   string
	Limit    int
}
type Label struct {
	Name string `json:"name"`
}
type Actor struct {
	Login string `json:"login"`
}
type Comment struct {
	Author    Actor  `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
}

type Goal struct {
	Title              string
	Body               string
	AcceptanceCriteria []string
}

func New(options Options) (*Client, error) {
	if options.Project == "" || options.Home == "" || options.Trust == nil || options.Policy == nil {
		return nil, errors.New("GitHub client requires project, config home, trust store and permission policy")
	}
	canonical, err := trust.Canonical(options.Project)
	if err != nil {
		return nil, fmt.Errorf("resolve project: %w", err)
	}
	trusted, err := options.Trust.IsTrusted(canonical)
	if err != nil || !trusted {
		return nil, errors.New("GitHub access requires a trusted project")
	}
	options.Project = canonical
	if options.Runner == nil {
		options.Runner = ExecRunner{}
	}
	if options.GH == "" {
		options.GH = "gh"
	}
	return &Client{options: options}, nil
}

func (c *Client) Repository(ctx context.Context) (string, error) {
	remotes, err := c.options.Runner.Run(ctx, "git", "-C", c.options.Project, "remote", "-v")
	if err != nil {
		return "", fmt.Errorf("read Git remotes: %w", safeError(err))
	}
	return parseRepository(string(remotes))
}

func parseRepository(remotes string) (string, error) {
	for _, line := range strings.Split(remotes, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "origin" {
			continue
		}
		url := strings.TrimSuffix(fields[1], ".git")
		if strings.HasPrefix(url, "git@github.com:") {
			url = strings.TrimPrefix(url, "git@github.com:")
		}
		if strings.HasPrefix(url, "ssh://git@github.com/") {
			url = strings.TrimPrefix(url, "ssh://git@github.com/")
		}
		if strings.HasPrefix(url, "https://github.com/") {
			url = strings.TrimPrefix(url, "https://github.com/")
		}
		parts := strings.Split(strings.Trim(url, "/"), "/")
		if len(parts) == 2 && validRepoPart(parts[0]) && validRepoPart(parts[1]) {
			return parts[0] + "/" + parts[1], nil
		}
	}
	return "", errors.New("origin is not a recognized GitHub repository")
}

var repoPart = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var githubToken = regexp.MustCompile(`(?i)\b(?:gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,})\b`)
var bearerSecret = regexp.MustCompile(`(?i)(?:authorization\s*[:=]\s*)?bearer\s+[^\s"']+`)
var assignedSecret = regexp.MustCompile(`(?i)(--?(?:token|password|secret|api[_-]?key|authorization)(?:=|\s+)[^\s,;"']+|(?:token|password|secret|api[_-]?key|authorization)\s*[:=]\s*[^\s,;"']+|https?://[^:/\s]+:[^@/\s]+@)`)

func validRepoPart(value string) bool {
	return value != "" && value != "." && value != ".." && repoPart.MatchString(value)
}

func (c *Client) OptedIn() (bool, error) {
	dir := filepath.Join(c.options.Project, "gasacli")
	dirInfo, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(dirInfo) {
		return false, errors.New("refusing project config directory that is not a regular directory")
	}
	path := filepath.Join(dir, "config.yaml")
	fileInfo, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(fileInfo) {
		return false, errors.New("refusing project config that is not a regular file")
	}
	if fileInfo.Size() > 1<<20 {
		return false, errors.New("project config exceeds 1 MiB limit")
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(false)
	var cfg repositoryOptIn
	if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("decode project config: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return false, fmt.Errorf("decode project config trailing document: %w", err)
		}
		return false, errors.New("project config must contain one YAML document")
	}
	return cfg.Plugins.GitHub, nil
}

func (c *Client) SetOptIn(enabled bool) error {
	dir, err := config.EnsureProjectLayout(c.options.Project, c.options.Trust)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yaml")
	var cfg map[string]any
	fileInfo, statErr := os.Lstat(path)
	if statErr == nil {
		if !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(fileInfo) {
			return errors.New("refusing project config that is not a regular file")
		}
		if fileInfo.Size() > 1<<20 {
			return errors.New("project config exceeds 1 MiB limit")
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("decode project config: %w", err)
		}
	} else {
		cfg = map[string]any{}
	}
	if cfg == nil {
		cfg = map[string]any{}
	}
	plugins, ok := cfg["plugins"].(map[string]any)
	if _, exists := cfg["plugins"]; exists && !ok {
		return fmt.Errorf("project config plugins must be a mapping")
	}
	if plugins == nil {
		plugins = map[string]any{}
		cfg["plugins"] = plugins
	}
	plugins["github"] = enabled
	data, err = yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func (c *Client) requireEnabled(ctx context.Context) (string, error) {
	repo, err := c.Repository(ctx)
	if err != nil {
		return "", err
	}
	enabled, err := c.OptedIn()
	if err != nil {
		return "", err
	}
	if !enabled {
		return "", fmt.Errorf("GitHub is disabled for this project; run gasa github enable")
	}
	return repo, nil
}

func (c *Client) Auth(ctx context.Context) (AuthStatus, error) {
	output, err := c.options.Runner.Run(ctx, c.options.GH, "auth", "status")
	if errors.Is(err, exec.ErrNotFound) {
		return AuthStatus{}, errors.New("gh CLI is not installed or not on PATH")
	}
	text := redactText(string(output))
	status := AuthStatus{}
	accountPattern := regexp.MustCompile(`(?im)^\s*Logged in to [^\s]+ account ([A-Za-z0-9-]+)`)
	if match := accountPattern.FindStringSubmatch(text); len(match) == 2 {
		status.LoggedIn, status.Account = true, match[1]
	}
	scopePattern := regexp.MustCompile(`(?im)^\s*Token scopes:\s*(.*)$`)
	if match := scopePattern.FindStringSubmatch(text); len(match) == 2 {
		for _, scope := range strings.Split(match[1], ",") {
			scope = strings.Trim(strings.TrimSpace(scope), "[]'\" ")
			if scope != "" {
				status.Scopes = append(status.Scopes, scope)
			}
		}
	}
	for _, want := range []string{"repo"} {
		if !contains(status.Scopes, want) {
			status.Missing = append(status.Missing, want)
		}
	}
	return status, nil
}

func (c *Client) ListIssues(ctx context.Context, state string, limit int) ([]Issue, error) {
	return c.ListIssuesWithFilters(ctx, IssueFilters{State: state, Limit: limit})
}

func (c *Client) ListIssuesWithFilters(ctx context.Context, filters IssueFilters) ([]Issue, error) {
	repo, err := c.requireEnabled(ctx)
	if err != nil {
		return nil, err
	}
	if filters.State == "" {
		filters.State = "open"
	}
	if filters.State != "open" && filters.State != "closed" && filters.State != "all" {
		return nil, errors.New("state must be open, closed or all")
	}
	if filters.Limit <= 0 || filters.Limit > 100 {
		filters.Limit = 30
	}
	args := []string{"issue", "list", "--repo", repo, "--state", filters.State, "--limit", strconv.Itoa(filters.Limit)}
	if filters.Assignee != "" {
		args = append(args, "--assignee", filters.Assignee)
	}
	for _, label := range filters.Labels {
		if strings.TrimSpace(label) != "" {
			args = append(args, "--label", label)
		}
	}
	if filters.Search != "" {
		args = append(args, "--search", filters.Search)
	}
	args = append(args, "--json", "number,title,body,state,url,labels,assignees")
	output, err := c.options.Runner.Run(ctx, c.options.GH, args...)
	if err != nil {
		return nil, fmt.Errorf("list GitHub issues: %w", safeError(err))
	}
	var issues []Issue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("parse GitHub issue list: %w", err)
	}
	return redactIssues(issues), nil
}

func (c *Client) ViewIssue(ctx context.Context, number int) (Issue, error) {
	repo, err := c.requireEnabled(ctx)
	if err != nil {
		return Issue{}, err
	}
	if number <= 0 {
		return Issue{}, errors.New("issue number must be positive")
	}
	output, err := c.options.Runner.Run(ctx, c.options.GH, "issue", "view", strconv.Itoa(number), "--repo", repo, "--json", "number,title,body,state,url,labels,assignees,comments")
	if err != nil {
		return Issue{}, fmt.Errorf("view GitHub issue: %w", safeError(err))
	}
	var issue Issue
	if err := json.Unmarshal(output, &issue); err != nil {
		return Issue{}, fmt.Errorf("parse GitHub issue: %w", err)
	}
	return redactIssues([]Issue{issue})[0], nil
}

func (c *Client) write(ctx context.Context, repo, summary string, args ...string) (string, error) {
	command := append([]string{c.options.GH}, args...)
	shown := formatArgv(append([]string{"gh"}, args...))
	request := permissions.Request{Action: "issue_write", Project: c.options.Project, Target: redactText(shown + "\n" + summary)}
	decision := c.options.Policy.Decide(ctx, request)
	if decision.Effect == permissions.Deny {
		return "", fmt.Errorf("GitHub write denied by policy: %s", permissions.RedactTarget(decision.Reason))
	}
	if c.options.DryRun {
		return "DRY RUN: " + redactText(shown+"\n"+summary), nil
	}
	if c.options.Approver == nil {
		return "", errors.New("GitHub write requires an interactive approver")
	}
	approved, err := c.options.Approver.Ask(ctx, request)
	if err != nil {
		return "", fmt.Errorf("request GitHub write approval: %w", err)
	}
	if approved.Effect != permissions.Allow {
		return "", errors.New("GitHub write was not approved")
	}
	output, err := c.options.Runner.Run(ctx, command[0], command[1:]...)
	if err != nil {
		return "", fmt.Errorf("GitHub write failed: %w", safeError(err))
	}
	return redactText(string(output)), nil
}

func (c *Client) CreateIssue(ctx context.Context, title, body string, labels, assignees []string) (string, error) {
	repo, err := c.requireEnabled(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(title) == "" {
		return "", errors.New("issue title is required")
	}
	title, body = redactText(title), redactText(body)
	args := []string{"issue", "create", "--repo", repo, "--title", title, "--body", body}
	for _, label := range labels {
		args = append(args, "--label", label)
	}
	for _, assignee := range assignees {
		args = append(args, "--assignee", assignee)
	}
	return c.write(ctx, repo, "Create issue: "+title+"\n"+body, args...)
}

func (c *Client) Comment(ctx context.Context, number int, body string) (string, error) {
	repo, err := c.requireEnabled(ctx)
	if err != nil {
		return "", err
	}
	if number <= 0 || strings.TrimSpace(body) == "" {
		return "", errors.New("positive issue number and comment body are required")
	}
	body = redactText(body)
	return c.write(ctx, repo, "Comment on issue #"+strconv.Itoa(number)+":\n"+body, "issue", "comment", strconv.Itoa(number), "--repo", repo, "--body", body)
}

func (c *Client) LinkRun(ctx context.Context, number int, runID, result string) (string, error) {
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(result) == "" {
		return "", errors.New("run id and result summary are required")
	}
	return c.Comment(ctx, number, "GasaCLI run `"+runID+"`\n\nResult summary:\n"+result)
}

func (c *Client) SetLabels(ctx context.Context, number int, labels []string, explicitlyAllowed bool) (string, error) {
	if !explicitlyAllowed {
		return "", errors.New("label changes require --allow-metadata-write")
	}
	repo, err := c.requireEnabled(ctx)
	if err != nil {
		return "", err
	}
	if number <= 0 || len(labels) == 0 {
		return "", errors.New("positive issue number and at least one label are required")
	}
	args := []string{"issue", "edit", strconv.Itoa(number), "--repo", repo}
	for _, label := range labels {
		args = append(args, "--add-label", label)
	}
	return c.write(ctx, repo, fmt.Sprintf("Add labels to issue #%d: %s", number, strings.Join(labels, ", ")), args...)
}

func (c *Client) Assign(ctx context.Context, number int, users []string, explicitlyAllowed bool) (string, error) {
	if !explicitlyAllowed {
		return "", errors.New("assignment changes require --allow-metadata-write")
	}
	repo, err := c.requireEnabled(ctx)
	if err != nil {
		return "", err
	}
	if number <= 0 || len(users) == 0 {
		return "", errors.New("positive issue number and at least one assignee are required")
	}
	args := []string{"issue", "edit", strconv.Itoa(number), "--repo", repo}
	for _, user := range users {
		args = append(args, "--add-assignee", user)
	}
	return c.write(ctx, repo, fmt.Sprintf("Assign issue #%d to %s", number, strings.Join(users, ", ")), args...)
}

func IssueToGoal(issue Issue) Goal {
	goal := Goal{Title: strings.TrimSpace(redactText(issue.Title)), Body: redactText(issue.Body)}
	inAcceptanceSection := false
	for _, line := range strings.Split(issue.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(strings.Trim(trimmed, "#*:_ "))
		if strings.HasPrefix(trimmed, "#") {
			inAcceptanceSection = strings.Contains(lower, "acceptance criteria") || lower == "acceptance" || lower == "criteria"
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "- [ ] ") || strings.HasPrefix(strings.ToLower(trimmed), "- [x] ") || strings.HasPrefix(strings.ToLower(trimmed), "* [ ] ") || strings.HasPrefix(strings.ToLower(trimmed), "* [x] ") {
			goal.AcceptanceCriteria = append(goal.AcceptanceCriteria, strings.TrimSpace(redactText(trimmed[6:])))
			continue
		}
		if inAcceptanceSection && (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ")) {
			goal.AcceptanceCriteria = append(goal.AcceptanceCriteria, strings.TrimSpace(redactText(trimmed[2:])))
		}
	}
	return goal
}

func redactIssues(issues []Issue) []Issue {
	for i := range issues {
		issues[i].Body = redactText(issues[i].Body)
		for j := range issues[i].Comments {
			issues[i].Comments[j].Body = redactText(issues[i].Comments[j].Body)
		}
	}
	return issues
}
func redactText(value string) string {
	value = bearerSecret.ReplaceAllString(value, "[REDACTED]")
	value = githubToken.ReplaceAllString(value, "[REDACTED]")
	return assignedSecret.ReplaceAllString(value, "[REDACTED]")
}
func formatArgv(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = strconv.Quote(arg)
	}
	return strings.Join(quoted, " ")
}
func safeError(err error) error { return errors.New(redactText(err.Error())) }
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
