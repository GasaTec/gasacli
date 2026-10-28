// Package sessions reads local coding-agent transcripts without modifying them.
package sessions

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxFileBytes    = 8 << 20
	maxLineBytes    = 1 << 20
	maxMessages     = 500
	maxSessions     = 200
	maxMessageBytes = 32 << 10
	maxToolCalls    = 50
)

type ToolCall struct {
	Kind    string   `json:"kind"`
	Summary string   `json:"summary"`
	Input   string   `json:"input,omitempty"`
	Output  string   `json:"output,omitempty"`
	Files   []string `json:"files,omitempty"`
}

type Message struct {
	Role      string     `json:"role"`
	Text      string     `json:"text,omitempty"`
	ToolCalls []ToolCall `json:"toolCalls,omitempty"`
	Thinking  string     `json:"thinking,omitempty"`
	TS        time.Time  `json:"ts,omitempty"`
}

type Session struct {
	Agent         string    `json:"agent"`
	ID            string    `json:"id"`
	ProviderID    string    `json:"-"`
	Title         string    `json:"title"`
	CWD           string    `json:"cwd"`
	Branch        string    `json:"branch,omitempty"`
	WorktreeID    string    `json:"worktreeId,omitempty"`
	Model         string    `json:"model,omitempty"`
	StartedAt     time.Time `json:"startedAt,omitempty"`
	UpdatedAt     time.Time `json:"updatedAt,omitempty"`
	Status        string    `json:"status"`
	Messages      []Message `json:"messages,omitempty"`
	MessageCount  int       `json:"messageCount,omitempty"`
	MessageOffset int       `json:"messageOffset,omitempty"`
	SourcePath    string    `json:"-"`
}

type Worktree struct {
	Path   string
	Branch string
}

// Roots overrides the conventional agent data roots, primarily for tests.
type Roots struct{ Claude, Codex string }

// Sender resumes an existing provider conversation with one bounded message.
type Sender interface {
	Send(ctx context.Context, request SendRequest) (<-chan Event, error)
	Capabilities() Capabilities
}

type SendRequest struct {
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
}

type Event struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Err  string `json:"error,omitempty"`
}

type Capabilities struct {
	Send   bool     `json:"send"`
	Agents []string `json:"agents,omitempty"`
}

func DefaultRoots() Roots {
	home, _ := os.UserHomeDir()
	return Roots{Claude: filepath.Join(home, ".claude", "projects"), Codex: filepath.Join(home, ".codex", "sessions")}
}

// Scan reads the supported local transcript formats. Unknown formats and
// missing roots are skipped without making the dashboard unavailable.
func Scan(roots Roots) []Session {
	return scan(roots, false)
}

// ScanMetadata reads the same session index without retaining transcript
// messages. LoadTranscript can load an individual session when it is opened.
func ScanMetadata(roots Roots) []Session {
	return scan(roots, true)
}

func scan(roots Roots, metadataOnly bool) []Session {
	if roots.Claude == "" && roots.Codex == "" {
		roots = DefaultRoots()
	}
	rootCount := 0
	if roots.Claude != "" {
		rootCount++
	}
	if roots.Codex != "" {
		rootCount++
	}
	perRootLimit := maxSessions
	if rootCount > 1 {
		perRootLimit = maxSessions / rootCount
	}
	var out []Session
	for _, root := range []struct{ agent, path string }{{"Claude Code", roots.Claude}, {"Codex", roots.Codex}} {
		if len(out) >= maxSessions {
			break
		}
		if root.path == "" {
			continue
		}
		rootSessions := 0
		_ = filepath.WalkDir(root.path, func(path string, entry os.DirEntry, err error) error {
			if len(out) >= maxSessions || rootSessions >= perRootLimit {
				return filepath.SkipAll
			}
			if err != nil {
				return nil
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			if filepath.Ext(path) != ".jsonl" || (root.agent == "Codex" && !strings.HasPrefix(filepath.Base(path), "rollout-")) {
				return nil
			}
			var session Session
			var ok bool
			if metadataOnly {
				session, ok = readMetadata(root.agent, path)
			} else {
				session, ok = readTranscript(root.agent, path)
			}
			if ok {
				out = append(out, session)
				rootSessions++
			}
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

// LoadTranscript reads one previously indexed transcript, including its
// bounded message history.
func LoadTranscript(agent, path string) (Session, bool) {
	return readTranscript(agent, path)
}

func readTranscript(agent, path string) (Session, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return Session{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return Session{}, false
	}
	defer file.Close()
	if info.Size() > maxFileBytes {
		_, _ = file.Seek(info.Size()-maxFileBytes, io.SeekStart)
		br := bufio.NewReader(file)
		_, _ = br.ReadString('\n')
		return parseLines(agent, path, info, br)
	}
	return parseLines(agent, path, info, bufio.NewReader(file))
}

// readMetadata inspects only the beginning and end of a transcript. Session
// discovery may find hundreds of large files; the full content is parsed only
// when a user opens an individual session.
func readMetadata(agent, path string) (Session, bool) {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return Session{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return Session{}, false
	}
	defer file.Close()
	const headBytes = 64 << 10
	const tailBytes = 96 << 10
	s := Session{Agent: agent, ID: stableID(agent, path), SourcePath: path, Status: "done", UpdatedAt: info.ModTime(), StartedAt: info.ModTime()}
	parseRange := func(offset, length int64) {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return
		}
		reader := bufio.NewReader(io.LimitReader(file, length))
		if offset > 0 {
			_, _ = reader.ReadString('\n')
		}
		for {
			line, oversized, readErr := readLine(reader)
			if readErr != nil && readErr != io.EOF {
				return
			}
			if !oversized && len(line) > 0 {
				var record map[string]any
				if json.Unmarshal(line, &record) == nil {
					if agent == "Claude Code" {
						parseClaude(&s, record)
					} else {
						parseCodex(&s, record)
					}
				}
			}
			if readErr == io.EOF {
				return
			}
		}
	}
	headLen := min(info.Size(), headBytes)
	parseRange(0, headLen)
	if info.Size() > headBytes {
		tailLen := min(info.Size(), tailBytes)
		parseRange(info.Size()-tailLen, tailLen)
	}
	if s.CWD == "" {
		return Session{}, false
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = info.ModTime()
	}
	s.Status = classifyStatus(info.ModTime(), time.Now(), false)
	if s.Title == "" {
		s.Title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	s.Title, s.Model, s.CWD, s.Branch = redact(s.Title), redact(s.Model), redact(s.CWD), redact(s.Branch)
	s.Messages = nil
	s.MessageCount = 0
	s.MessageOffset = 0
	return s, true
}

func parseLines(agent, path string, info os.FileInfo, reader *bufio.Reader) (Session, bool) {
	fileTitle := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	s := Session{Agent: agent, ID: stableID(agent, path), SourcePath: path, Status: "done", UpdatedAt: info.ModTime(), StartedAt: info.ModTime()}
	for {
		line, oversized, err := readLine(reader)
		if err != nil && err != io.EOF {
			break
		}
		if len(line) == 0 && err == io.EOF {
			break
		}
		if oversized {
			continue
		}
		var record map[string]any
		if json.Unmarshal(line, &record) == nil {
			if agent == "Claude Code" {
				parseClaude(&s, record)
			} else {
				parseCodex(&s, record)
			}
		}
		if err == io.EOF {
			break
		}
	}
	if s.CWD == "" || len(s.Messages) == 0 {
		return Session{}, false
	}
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = info.ModTime()
	}
	s.Status = classifyStatus(info.ModTime(), time.Now(), false)
	if len(s.Messages) > maxMessages {
		s.Messages = s.Messages[len(s.Messages)-maxMessages:]
	}
	s.MessageOffset = s.MessageCount - len(s.Messages)
	if s.Title == "" {
		s.Title = fileTitle
	}
	s.Title, s.Model, s.CWD, s.Branch = redact(s.Title), redact(s.Model), redact(s.CWD), redact(s.Branch)
	return s, true
}

// readLine retains at most maxLineBytes while draining the rest of an oversized
// record so a malformed giant line cannot hide later valid transcript messages.
func readLine(reader *bufio.Reader) ([]byte, bool, error) {
	var line []byte
	oversized := false
	for {
		part, err := reader.ReadSlice('\n')
		if !oversized {
			if len(line)+len(part) > maxLineBytes {
				oversized = true
				line = nil
			} else {
				line = append(line, part...)
			}
		}
		if err == bufio.ErrBufferFull {
			continue
		}
		return line, oversized, err
	}
}

func parseClaude(s *Session, r map[string]any) {
	if v := str(r["cwd"]); v != "" {
		s.CWD = v
	}
	if v := str(r["sessionId"]); v != "" {
		s.ID = "claude-" + safeID(v)
		s.ProviderID = v
	}
	if v := str(r["timestamp"]); v != "" {
		if t, e := time.Parse(time.RFC3339Nano, v); e == nil {
			if s.StartedAt.IsZero() || t.Before(s.StartedAt) {
				s.StartedAt = t
			}
			s.UpdatedAt = t
		}
	}
	kind := str(r["type"])
	msg, _ := r["message"].(map[string]any)
	if kind != "user" && kind != "assistant" {
		return
	}
	role := kind
	if msgRole := str(msg["role"]); msgRole != "" {
		role = msgRole
	}
	text, tools := content(msg["content"])
	if text == "" && len(tools) == 0 {
		text = str(r["text"])
	}
	if text != "" || len(tools) > 0 {
		appendMessage(s, Message{Role: role, Text: redact(text), ToolCalls: tools, TS: s.UpdatedAt})
	}
	if s.Title == "" && role == "user" && text != "" {
		s.Title = short(text, 80)
	}
	if model := str(msg["model"]); model != "" {
		s.Model = model
	}
}

func parseCodex(s *Session, r map[string]any) {
	if timestamp := str(r["timestamp"]); timestamp != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			if parsed.Before(s.StartedAt) {
				s.StartedAt = parsed
			}
			s.UpdatedAt = parsed
		}
	}
	typ := str(r["type"])
	payload, _ := r["payload"].(map[string]any)
	if typ == "session_meta" {
		if v := str(payload["cwd"]); v != "" {
			s.CWD = v
		}
		if v := str(payload["id"]); v != "" {
			s.ID = "codex-" + safeID(v)
			s.ProviderID = v
		}
		if v := str(payload["model"]); v != "" {
			s.Model = v
		}
		return
	}
	if typ == "turn_context" {
		if v := str(payload["cwd"]); v != "" {
			s.CWD = v
		}
		if v := str(payload["model"]); v != "" {
			s.Model = v
		}
		return
	}
	if typ != "response_item" {
		if typ == "event_msg" {
			kind := str(payload["type"])
			role, text := "", str(payload["message"])
			if kind == "user_message" {
				role = "user"
			}
			if kind == "agent_message" {
				role = "assistant"
			}
			if role != "" && text != "" {
				appendMessage(s, Message{Role: role, Text: redact(text), TS: s.UpdatedAt})
				if s.Title == "" && role == "user" {
					s.Title = short(text, 80)
				}
			}
		}
		return
	}
	itemType := str(payload["type"])
	role := str(payload["role"])
	text := str(payload["text"])
	if text == "" {
		text, _ = content(payload["content"])
	}
	if role == "" {
		role = itemType
	}
	if itemType == "message" && text != "" {
		text, tools := parseCodexToolBlocks(text)
		if len(tools) > 0 {
			appendMessage(s, Message{Role: "tool", ToolCalls: tools, TS: s.UpdatedAt})
		}
		if strings.TrimSpace(text) != "" {
			appendMessage(s, Message{Role: role, Text: redact(text), TS: s.UpdatedAt})
		}
	}
	if itemType == "function_call" || itemType == "tool_call" {
		appendMessage(s, Message{Role: "tool", ToolCalls: []ToolCall{{Kind: str(payload["name"]), Summary: str(payload["name"]), Input: redact(str(payload["arguments"]))}}, TS: s.UpdatedAt})
	}
	if text != "" && role == "user" && s.Title == "" {
		s.Title = short(text, 80)
	}
}

func parseCodexToolBlocks(text string) (string, []ToolCall) {
	matches := codexToolBlock.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return text, nil
	}
	var tools []ToolCall
	for _, match := range matches {
		kind, summary, body := match[1], strings.TrimSpace(match[2]), strings.TrimSpace(match[3])
		if summary == "" {
			summary = "tool"
		}
		call := ToolCall{Kind: summary, Summary: summary}
		if kind == "tool_call" {
			call.Input = redact(body)
		} else {
			call.Output = redact(body)
		}
		tools = append(tools, call)
	}
	return strings.TrimSpace(codexToolBlock.ReplaceAllString(text, "")), tools
}

func content(v any) (string, []ToolCall) {
	if text, ok := v.(string); ok {
		return text, nil
	}
	list, ok := v.([]any)
	if !ok {
		return "", nil
	}
	var chunks []string
	var tools []ToolCall
	for _, item := range list {
		m, _ := item.(map[string]any)
		switch str(m["type"]) {
		case "text", "output_text", "input_text":
			chunks = append(chunks, str(m["text"]))
		case "tool_use":
			tools = append(tools, ToolCall{Kind: redact(str(m["name"])), Summary: redact(str(m["name"])), Input: redact(jsonText(m["input"]))})
		case "tool_result":
			tools = append(tools, ToolCall{Kind: "tool", Summary: "Tool result", Output: redact(jsonText(m["content"]))})
		}
	}
	return strings.Join(chunks, "\n"), tools
}
func jsonText(v any) string  { b, _ := json.Marshal(v); return string(b) }
func str(v any) string       { s, _ := v.(string); return s }
func safeID(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:12]) }
func stableID(agent, path string) string {
	return strings.ToLower(strings.ReplaceAll(agent, " ", "-")) + "-" + safeID(filepath.Clean(path))
}
func short(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\n", " "), "\r", " "))
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n]) + "…"
	}
	return s
}

func appendMessage(s *Session, message Message) {
	message.Text = boundedText(message.Text)
	message.Thinking = boundedText(message.Thinking)
	if len(message.ToolCalls) > maxToolCalls {
		message.ToolCalls = message.ToolCalls[:maxToolCalls]
	}
	for i := range message.ToolCalls {
		call := &message.ToolCalls[i]
		call.Kind = boundedText(call.Kind)
		call.Summary = boundedText(call.Summary)
		call.Input = boundedText(call.Input)
		call.Output = boundedText(call.Output)
		if len(call.Files) > maxToolCalls {
			call.Files = call.Files[:maxToolCalls]
		}
		for j := range call.Files {
			call.Files[j] = boundedText(call.Files[j])
		}
	}
	if len(s.Messages) >= maxMessages {
		copy(s.Messages, s.Messages[1:])
		s.Messages = s.Messages[:maxMessages-1]
	}
	s.Messages = append(s.Messages, message)
	s.MessageCount++
}

func boundedText(value string) string {
	if len(value) <= maxMessageBytes {
		return value
	}
	limit := maxMessageBytes - len("\n[truncated]")
	for limit > 0 && !utf8.ValidString(value[:limit]) {
		limit--
	}
	return value[:limit] + "\n[truncated]"
}

func classifyStatus(updatedAt, now time.Time, processAlive bool) string {
	if processAlive || (!updatedAt.IsZero() && now.Sub(updatedAt) >= 0 && now.Sub(updatedAt) <= 10*time.Second) {
		return "working"
	}
	return "done"
}

var (
	secretTokenPattern   = regexp.MustCompile(`(?i)(?:\bsk-[a-z0-9_-]{8,}|\b(?:gh[pousr]|github_pat)_[a-z0-9_]{20,}|\bxox[baprs]-[a-z0-9-]{10,}|\bAKIA[0-9A-Z]{16}\b)`)
	authorizationPattern = regexp.MustCompile(`(?i)(\bauthorization\s*:\s*(?:bearer\s+)?|\bbearer\s+)([^\s,;]+)`)
	envSecretPattern     = regexp.MustCompile(`(?i)^\s*([A-Z0-9_]*(?:SECRET|TOKEN|PASSWORD|PASSWD|API_KEY|PRIVATE_KEY|ACCESS_KEY)[A-Z0-9_]*)\s*=\s*(.+?)\s*$`)
	codexToolBlock       = regexp.MustCompile(`(?s)\[external_agent_(tool_call|tool_result)(?::\s*([^\]]+))?\](.*?)\[/external_agent_(?:tool_call|tool_result)\]`)
)

func redact(s string) string {
	lines := strings.Split(s, "\n")
	privateKey := false
	for i, line := range lines {
		l := strings.ToLower(line)
		if privateKey {
			lines[i] = "[redacted]"
			if strings.Contains(l, "-----end ") {
				privateKey = false
			}
			continue
		}
		if strings.Contains(l, "-----begin ") {
			privateKey = true
			lines[i] = "[redacted]"
			continue
		}
		if match := envSecretPattern.FindStringSubmatch(line); match != nil {
			lines[i] = match[1] + "=[redacted]"
			continue
		}
		line = authorizationPattern.ReplaceAllString(line, "$1[redacted]")
		lines[i] = secretTokenPattern.ReplaceAllString(line, "[redacted]")
	}
	return strings.Join(lines, "\n")
}

// Redact removes common credentials from text before it leaves the process.
func Redact(s string) string { return redact(s) }

// ProjectRoot uses git argv invocation to map worktrees without a shell.
func ProjectRoot(cwd string) string {
	if cwd == "" {
		return ""
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	if common := execGit("-C", cwd, "rev-parse", "--git-common-dir"); common != "" {
		if !filepath.IsAbs(common) {
			common = filepath.Join(cwd, common)
		}
		return filepath.Clean(filepath.Dir(common))
	}
	return filepath.Clean(cwd)
}

// WorktreeRoot resolves a transcript working directory to its registered
// worktree path, falling back to the clean working directory outside Git.
func WorktreeRoot(cwd string) string {
	if cwd == "" {
		return ""
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	if top := execGit("-C", cwd, "rev-parse", "--show-toplevel"); top != "" {
		return filepath.Clean(top)
	}
	return filepath.Clean(cwd)
}

// ListWorktrees reads Git's porcelain format using argv, never a shell.
func ListWorktrees(root string) []Worktree {
	output := execGit("-C", root, "worktree", "list", "--porcelain")
	var result []Worktree
	for _, block := range strings.Split(output, "\n\n") {
		var item Worktree
		for _, line := range strings.Split(block, "\n") {
			if strings.HasPrefix(line, "worktree ") {
				item.Path = strings.TrimPrefix(line, "worktree ")
			}
			if strings.HasPrefix(line, "branch refs/heads/") {
				item.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			}
		}
		if item.Path != "" {
			item.Path = filepath.Clean(item.Path)
			if item.Branch == "" {
				item.Branch = "detached"
			}
			result = append(result, item)
		}
	}
	return result
}

func Branch(cwd string) string {
	return execGit("-C", cwd, "rev-parse", "--abbrev-ref", "HEAD")
}

var execGit = func(args ...string) string {
	cmd := execCommand("git", args...)
	return strings.TrimSpace(cmd)
}

var execCommand = func(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
