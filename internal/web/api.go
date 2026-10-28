package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GasaTec/gasacli/internal/sessions"
	"github.com/GasaTec/gasacli/internal/trust"
)

type projectView struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	Path         string             `json:"path,omitempty"`
	Branch       string             `json:"branch,omitempty"`
	Sessions     []sessions.Session `json:"sessions,omitempty"`
	Worktrees    []worktreeView     `json:"worktrees,omitempty"`
	SessionCount int                `json:"sessionCount"`
	Trusted      bool               `json:"trusted"`
	Current      bool               `json:"current,omitempty"`
}

type worktreeView struct {
	ID           string `json:"id"`
	Path         string `json:"path"`
	Branch       string `json:"branch"`
	SessionCount int    `json:"sessionCount"`
}

type snapshot struct {
	projects  []projectView
	byID      map[string]projectView
	worktrees map[string]string
	sessions  map[string]sessions.Session
}

type snapshotCache struct {
	mu             sync.Mutex
	value          snapshot
	refreshed      time.Time
	home           string
	roots          sessions.Roots
	currentProject string
}

const snapshotRefreshInterval = 10 * time.Second

func (c *snapshotCache) get() snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.refreshed) < snapshotRefreshInterval {
		return c.value
	}
	value := buildSnapshot(c.home, c.roots, c.currentProject)
	debug.FreeOSMemory()
	c.value = value
	c.refreshed = time.Now()
	return c.value
}

func buildSnapshot(home string, roots sessions.Roots, current ...string) snapshot {
	s := snapshot{byID: map[string]projectView{}, worktrees: map[string]string{}, sessions: map[string]sessions.Session{}}
	store := trust.NewStore(home)
	grouped := map[string][]sessions.Session{}
	paths := map[string]string{}
	currentProject := ""
	if len(current) > 0 && current[0] != "" {
		currentProject, _ = trust.Canonical(current[0])
	}
	trustedProjects, _ := store.TrustedProjects()
	for _, root := range trustedProjects {
		paths[projectID(root)] = root
	}
	type resolvedSessionPath struct {
		root, worktreeRoot, branch string
		trusted, valid             bool
	}
	resolvedByCWD := make(map[string]resolvedSessionPath)
	for _, session := range sessions.ScanMetadata(roots) {
		cwd := filepath.Clean(session.CWD)
		resolved, ok := resolvedByCWD[cwd]
		if !ok {
			worktreeRoot := sessions.WorktreeRoot(session.CWD)
			root := sessions.ProjectRoot(session.CWD)
			if root == "" {
				resolvedByCWD[cwd] = resolvedSessionPath{}
				continue
			}
			trusted, err := store.IsTrusted(root)
			if err != nil {
				continue
			}
			if trusted {
				root, err = trust.Canonical(root)
				if err != nil {
					continue
				}
				worktreeRoot, err = trust.Canonical(worktreeRoot)
				if err != nil {
					worktreeRoot = root
				}
			}
			resolved = resolvedSessionPath{root: root, worktreeRoot: worktreeRoot, trusted: trusted, valid: true}
			if trusted {
				resolved.branch = sessions.Branch(session.CWD)
			}
			resolvedByCWD[cwd] = resolved
		}
		if !resolved.valid {
			continue
		}
		root, worktreeRoot := resolved.root, resolved.worktreeRoot
		id := projectID(root)
		if resolved.trusted {
			session.Branch = resolved.branch
			session.WorktreeID = projectID(filepath.Clean(worktreeRoot))
			grouped[id] = append(grouped[id], session)
			paths[id] = root
			s.worktrees[session.WorktreeID] = worktreeRoot
		} else {
			session.Messages = nil
			grouped["untrusted"] = append(grouped["untrusted"], session)
		}
	}
	for id := range paths {
		if _, ok := grouped[id]; !ok {
			grouped[id] = nil
		}
	}
	for id, list := range grouped {
		if id == "untrusted" {
			s.projects = append(s.projects, projectView{ID: id, Name: "Not trusted", SessionCount: len(list), Trusted: false})
			continue
		}
		root := paths[id]
		p := projectView{ID: id, Name: filepath.Base(root), Path: root, SessionCount: len(list), Trusted: true, Current: root == currentProject}
		for _, session := range list {
			s.sessions[session.ID] = session
			session.Messages = nil
			session.MessageCount = 0
			session.MessageOffset = 0
			p.Sessions = append(p.Sessions, session)
		}
		if p.Sessions == nil {
			p.Sessions = []sessions.Session{}
		}
		p.Branch = gitOutput(root, "branch", "--show-current")
		for _, worktree := range sessions.ListWorktrees(root) {
			worktreeID := projectID(filepath.Clean(worktree.Path))
			p.Worktrees = append(p.Worktrees, worktreeView{ID: worktreeID, Path: worktree.Path, Branch: worktree.Branch})
			s.worktrees[worktreeID] = worktree.Path
		}
		if len(p.Worktrees) == 0 {
			worktreeID := projectID(filepath.Clean(root))
			p.Worktrees = []worktreeView{{ID: worktreeID, Path: root, Branch: p.Branch}}
			s.worktrees[worktreeID] = root
		}
		for i := range p.Worktrees {
			for _, session := range list {
				if session.WorktreeID == p.Worktrees[i].ID {
					p.Worktrees[i].SessionCount++
				}
			}
		}
		s.projects = append(s.projects, p)
		s.byID[id] = p
	}
	// The untrusted bucket is presented separately and last.
	sortProjects(s.projects)
	for _, p := range s.projects {
		if p.ID == "untrusted" {
			s.projects = append(removeProject(s.projects, "untrusted"), p)
			break
		}
	}
	return s
}

func sortProjects(projects []projectView) {
	for i := 0; i < len(projects); i++ {
		for j := i + 1; j < len(projects); j++ {
			if projects[j].Name < projects[i].Name {
				projects[i], projects[j] = projects[j], projects[i]
			}
		}
	}
}
func removeProject(projects []projectView, id string) []projectView {
	out := projects[:0]
	for _, p := range projects {
		if p.ID != id {
			out = append(out, p)
		}
	}
	return out
}
func projectID(path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	return hex.EncodeToString(sum[:12])
}
func gitOutput(path string, args ...string) string {
	if path == "" {
		return ""
	}
	all := append([]string{"-C", path}, args...)
	c := exec.Command("git", all...)
	c.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	b, e := c.Output()
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func safeDiffPath(value string) bool {
	if value == "" || strings.ContainsRune(value, 0) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) || filepath.IsAbs(value) || filepath.VolumeName(value) != "" || hasWindowsVolume(value) {
		return false
	}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return false
		}
	}
	return true
}

func hasWindowsVolume(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

type cappedOutput struct {
	value strings.Builder
	limit int
	full  bool
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	remaining := b.limit - b.value.Len()
	if remaining <= 0 {
		b.full = true
		return 0, fmt.Errorf("output limit reached")
	}
	if len(p) > remaining {
		_, _ = b.value.Write(p[:remaining])
		b.full = true
		return remaining, fmt.Errorf("output limit reached")
	}
	return b.value.Write(p)
}

func gitOutputLimited(path string, limit int, args ...string) (string, bool) {
	all := append([]string{"-C", path}, args...)
	cmd := exec.Command("git", all...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	output := &cappedOutput{limit: limit}
	cmd.Stdout = output
	_ = cmd.Run()
	return strings.TrimSpace(output.value.String()), output.full
}
func jsonResponse(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func apiHandler(getSnapshot func() snapshot, w http.ResponseWriter, r *http.Request) bool {
	switch {
	case r.URL.Path == "/api/projects" && r.Method == http.MethodGet:
		jsonResponse(w, getSnapshot().projects)
		return true
	case r.URL.Path == "/api/sessions" && r.Method == http.MethodGet:
		projectID := r.URL.Query().Get("project")
		if projectID == "" {
			http.Error(w, "project required", 400)
			return true
		}
		s := getSnapshot()
		project, ok := s.byID[projectID]
		if !ok {
			http.Error(w, "not found", 404)
			return true
		}
		jsonResponse(w, project.Sessions)
		return true
	case strings.HasPrefix(r.URL.Path, "/api/sessions/") && r.Method == http.MethodGet:
		id := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
		if id == "" || strings.Contains(id, "/") || strings.Contains(id, "\\") {
			http.Error(w, "not found", 404)
			return true
		}
		s := getSnapshot()
		if indexed, ok := s.sessions[id]; ok {
			session := indexed
			if indexed.SourcePath != "" {
				loadedSession, loaded := sessions.LoadTranscript(indexed.Agent, indexed.SourcePath)
				if !loaded || loadedSession.ID != indexed.ID {
					http.Error(w, "not found", 404)
					return true
				}
				session = loadedSession
			}
			const pageSize = 100
			end := session.MessageOffset + len(session.Messages)
			if value := r.URL.Query().Get("before"); value != "" {
				parsed, err := strconv.Atoi(value)
				if err != nil || parsed < 0 {
					http.Error(w, "invalid cursor", 400)
					return true
				}
				end = parsed
			}
			base := session.MessageOffset
			end -= base
			if end < 0 {
				end = 0
			}
			if end > len(session.Messages) {
				end = len(session.Messages)
			}
			start := end - pageSize
			if start < 0 {
				start = 0
			}
			session.MessageOffset = base + start
			session.Messages = session.Messages[start:end]
			jsonResponse(w, session)
			return true
		}
		http.Error(w, "not found", 404)
		return true
	case r.URL.Path == "/api/git/status" && r.Method == http.MethodGet:
		id := r.URL.Query().Get("worktree")
		s := getSnapshot()
		worktree, ok := s.worktrees[id]
		if !ok {
			http.Error(w, "not found", 404)
			return true
		}
		status := sessions.Redact(gitOutput(worktree, "status", "--porcelain=v1"))
		numstat := sessions.Redact(gitOutput(worktree, "diff", "--numstat"))
		jsonResponse(w, map[string]string{"status": status, "numstat": numstat})
		return true
	case r.URL.Path == "/api/git/diff" && r.Method == http.MethodGet:
		id := r.URL.Query().Get("worktree")
		file := r.URL.Query().Get("file")
		s := getSnapshot()
		worktree, ok := s.worktrees[id]
		if !ok || !safeDiffPath(file) {
			http.Error(w, "not found", http.StatusNotFound)
			return true
		}
		numstat := gitOutput(worktree, "diff", "--numstat", "--", file)
		if numstat == "" || strings.HasPrefix(numstat, "-\t") {
			jsonResponse(w, map[string]any{"diff": "", "binary": strings.HasPrefix(numstat, "-\t")})
			return true
		}
		diff, truncated := gitOutputLimited(worktree, 64<<10, "diff", "--no-ext-diff", "--unified=3", "--", file)
		jsonResponse(w, map[string]any{"diff": sessions.Redact(diff), "truncated": truncated})
		return true
	case r.URL.Path == "/api/events" && r.Method == http.MethodGet:
		if _, ok := w.(http.Flusher); !ok {
			http.Error(w, "stream unavailable", 500)
			return true
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher := w.(http.Flusher)
		controller := http.NewResponseController(w)
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			data, err := json.Marshal(getSnapshot().projects)
			if err != nil {
				return true
			}
			_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if _, err := fmt.Fprintf(w, "event: projects\ndata: %s\n\n", data); err != nil {
				return true
			}
			flusher.Flush()
			select {
			case <-r.Context().Done():
				return true
			case <-ticker.C:
			}
		}
	}
	return false
}
