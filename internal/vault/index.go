// Package vault indexes Markdown files without writing to the connected folder.
package vault

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/storage"
)

const (
	maxFileBytes  = 2 << 20
	maxFiles      = 10000
	maxTotalBytes = 256 << 20
	maxChunkBytes = 16 << 10
)

func init() {
	if err := storage.RegisterMigration(storage.Migration{Version: 70, Name: "brain_fts", SQL: `
CREATE TABLE brain_config (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE brain_files (path TEXT PRIMARY KEY, hash TEXT NOT NULL, mtime_ns INTEGER NOT NULL, size INTEGER NOT NULL);
CREATE VIRTUAL TABLE brain_chunks USING fts5(path UNINDEXED, heading, start_line UNINDEXED, end_line UNINDEXED, mtime_ns UNINDEXED, tags, links, body, tokenize='unicode61');
`}); err != nil {
		panic(err)
	}
}

type Index struct{ store *storage.Store }

func New(store *storage.Store) (*Index, error) {
	if store == nil {
		return nil, errors.New("vault: storage is required")
	}
	return &Index{store}, nil
}

type Stats struct {
	Files, Chunks, Skipped int
	Bytes                  int64
	Duration               time.Duration
}
type Snippet struct {
	Path, Heading, Text string
	StartLine, EndLine  int
	Modified            time.Time
	Tags, Links         []string
}

func (i *Index) Connect(ctx context.Context, root string) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("vault path: %w", err)
	}
	if !info.IsDir() {
		return errors.New("vault path is not a directory")
	}
	_, err = i.store.DB().ExecContext(ctx, `INSERT INTO brain_config(key,value) VALUES('root',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, abs)
	return err
}

func (i *Index) Root(ctx context.Context) (string, error) {
	var root string
	err := i.store.DB().QueryRowContext(ctx, `SELECT value FROM brain_config WHERE key='root'`).Scan(&root)
	return root, err
}
func (i *Index) Disconnect(ctx context.Context) error {
	_, err := i.store.DB().ExecContext(ctx, `DELETE FROM brain_config WHERE key='root'`)
	if err != nil {
		return err
	}
	_, err = i.store.DB().ExecContext(ctx, `DELETE FROM brain_files; DELETE FROM brain_chunks`)
	return err
}
func (i *Index) SetIgnoreGlobs(ctx context.Context, globs []string) error {
	if len(globs) > 100 {
		return errors.New("vault: at most 100 ignore globs are allowed")
	}
	seen := map[string]bool{}
	for _, pattern := range globs {
		pattern = strings.TrimSpace(pattern)
		pattern = filepath.ToSlash(pattern)
		if pattern == "" {
			continue
		}
		if filepath.IsAbs(pattern) || strings.HasPrefix(pattern, "/") || pattern == ".." || strings.HasPrefix(pattern, "../") || strings.Contains(pattern, "/../") || strings.IndexByte(pattern, 0) >= 0 {
			return fmt.Errorf("vault: invalid ignore glob %q", pattern)
		}
		if _, err := filepath.Match(pattern, "probe.md"); err != nil {
			return fmt.Errorf("vault: invalid ignore glob %q: %w", pattern, err)
		}
		if seen[pattern] {
			return fmt.Errorf("vault: duplicate ignore glob %q", pattern)
		}
		seen[pattern] = true
	}
	_, err := i.store.DB().ExecContext(ctx, `INSERT INTO brain_config(key,value) VALUES('ignore_globs',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, strings.Join(globs, "\n"))
	return err
}

func (i *Index) Reindex(ctx context.Context) (Stats, error) {
	started := time.Now()
	root, err := i.Root(ctx)
	if err != nil {
		return Stats{}, fmt.Errorf("connect a vault first: %w", err)
	}
	var globsText string
	_ = i.store.DB().QueryRowContext(ctx, `SELECT value FROM brain_config WHERE key='ignore_globs'`).Scan(&globsText)
	globs := append([]string{".git/**", ".obsidian/**", "node_modules/**", "vendor/**"}, strings.Split(globsText, "\n")...)
	seen := make(map[string]bool)
	stats := Stats{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		for _, pattern := range globs {
			if pattern == "" {
				continue
			}
			matched, _ := filepath.Match(pattern, rel)
			if matched || strings.HasPrefix(pattern, "**/") && strings.Contains(rel, strings.TrimPrefix(pattern, "**/")) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(rel), ".md") {
			return nil
		}
		if len(seen) >= maxFiles {
			return fmt.Errorf("vault contains more than %d Markdown files", maxFiles)
		}
		seen[rel] = true
		info, e := entry.Info()
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > maxFileBytes {
			return fmt.Errorf("Markdown file exceeds %d bytes: %s", maxFileBytes, rel)
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		stats.Bytes += int64(len(data))
		if stats.Bytes > maxTotalBytes {
			return fmt.Errorf("vault exceeds %d bytes of indexed Markdown", maxTotalBytes)
		}
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		var old string
		qerr := i.store.DB().QueryRowContext(ctx, `SELECT hash FROM brain_files WHERE path=?`, rel).Scan(&old)
		if qerr == nil && old == hash {
			stats.Skipped++
			return nil
		}
		if qerr != nil && !errors.Is(qerr, sql.ErrNoRows) {
			return qerr
		}
		chunks := parseMarkdown(data, info.ModTime())
		if _, e = i.store.DB().ExecContext(ctx, `DELETE FROM brain_chunks WHERE path=?`, rel); e != nil {
			return e
		}
		for _, c := range chunks {
			if _, e = i.store.DB().ExecContext(ctx, `INSERT INTO brain_chunks(path,heading,start_line,end_line,mtime_ns,tags,links,body) VALUES(?,?,?,?,?,?,?,?)`, rel, c.Heading, c.StartLine, c.EndLine, info.ModTime().UnixNano(), strings.Join(c.Tags, " "), strings.Join(c.Links, " "), c.Text); e != nil {
				return e
			}
			stats.Chunks++
		}
		_, e = i.store.DB().ExecContext(ctx, `INSERT INTO brain_files(path,hash,mtime_ns,size) VALUES(?,?,?,?) ON CONFLICT(path) DO UPDATE SET hash=excluded.hash,mtime_ns=excluded.mtime_ns,size=excluded.size`, rel, hash, info.ModTime().UnixNano(), len(data))
		if e != nil {
			return e
		}
		stats.Files++
		return nil
	})
	if err != nil {
		return stats, err
	}
	rows, err := i.store.DB().QueryContext(ctx, `SELECT path FROM brain_files`)
	if err != nil {
		return stats, err
	}
	var stale []string
	for rows.Next() {
		var p string
		if err = rows.Scan(&p); err != nil {
			rows.Close()
			return stats, err
		}
		if !seen[p] {
			stale = append(stale, p)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return stats, err
	}
	for _, p := range stale {
		if _, err = i.store.DB().ExecContext(ctx, `DELETE FROM brain_chunks WHERE path=?`, p); err != nil {
			return stats, err
		}
		if _, err = i.store.DB().ExecContext(ctx, `DELETE FROM brain_files WHERE path=?`, p); err != nil {
			return stats, err
		}
	}
	stats.Duration = time.Since(started)
	return stats, nil
}

var headingRE = regexp.MustCompile(`^(#{1,6})\s+(.+?)\s*#*\s*$`)
var tagRE = regexp.MustCompile(`(?i)(?:^|\s)#([\pL\pN_/-]+)`)
var linkRE = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)|\[\[([^\]]+)\]\]`)

func parseMarkdown(data []byte, modified time.Time) []Snippet {
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	frontTags := []string{}
	lineOffset := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		inTags := false
		for n := 1; n < len(lines); n++ {
			if strings.TrimSpace(lines[n]) == "---" {
				lineOffset = n + 1
				lines = append([]string(nil), lines[n+1:]...)
				break
			}
			field := strings.TrimSpace(lines[n])
			if strings.HasPrefix(field, "tags:") || inTags && strings.HasPrefix(field, "-") {
				value := field
				if strings.HasPrefix(field, "tags:") {
					value = strings.TrimSpace(strings.TrimPrefix(field, "tags:"))
					inTags = value == "" || value == "["
				} else {
					value = strings.TrimSpace(strings.TrimPrefix(field, "-"))
				}
				value = strings.Trim(value, "[]")
				for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '"' || r == '\'' }) {
					tag = strings.TrimPrefix(tag, "#")
					if tag != "" {
						frontTags = append(frontTags, tag)
					}
				}
			} else if inTags && !strings.HasPrefix(lines[n], " ") && !strings.HasPrefix(lines[n], "\t") {
				inTags = false
			}
		}
	}
	title := "(document)"
	start := lineOffset + 1
	var body strings.Builder
	var out []Snippet
	flush := func(end int) {
		text := strings.TrimSpace(body.String())
		if text != "" {
			if len(text) > maxChunkBytes {
				text = text[:maxChunkBytes]
			}
			tags, links := extractMeta(text)
			tags = append(tags, frontTags...)
			sort.Strings(tags)
			out = append(out, Snippet{Heading: title, Text: text, StartLine: start, EndLine: end, Modified: modified, Tags: tags, Links: links})
		}
		body.Reset()
	}
	for n, line := range lines {
		if m := headingRE.FindStringSubmatch(line); m != nil {
			flush(n)
			title = m[2]
			start = lineOffset + n + 2
			continue
		}
		if body.Len()+len(line)+1 > maxChunkBytes {
			flush(n)
			start = lineOffset + n + 1
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	flush(lineOffset + len(lines))
	return out
}
func extractMeta(body string) ([]string, []string) {
	tags, links := []string{}, []string{}
	seen := map[string]bool{}
	for _, m := range tagRE.FindAllStringSubmatch(body, -1) {
		if !seen[m[1]] {
			tags = append(tags, m[1])
			seen[m[1]] = true
		}
	}
	seen = map[string]bool{}
	for _, m := range linkRE.FindAllStringSubmatch(body, -1) {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		if !seen[v] {
			links = append(links, v)
			seen[v] = true
		}
	}
	sort.Strings(tags)
	sort.Strings(links)
	return tags, links
}

func (i *Index) Search(ctx context.Context, query string, limit int) ([]Snippet, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("search query is empty")
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	words := strings.Fields(query)
	quoted := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.Trim(w, `"'()[]{}:;,.!?`)
		if w != "" {
			quoted = append(quoted, `"`+strings.ReplaceAll(w, `"`, `""`)+`"`)
		}
	}
	if len(quoted) == 0 {
		return nil, errors.New("search query has no searchable terms")
	}
	root, err := i.Root(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect a vault first: %w", err)
	}
	rows, err := i.store.DB().QueryContext(ctx, `SELECT path,heading,start_line,end_line,mtime_ns,tags,links,body FROM brain_chunks WHERE brain_chunks MATCH ? ORDER BY bm25(brain_chunks) LIMIT ?`, strings.Join(quoted, " AND "), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snippet
	for rows.Next() {
		var s Snippet
		var ns int64
		var tags, links string
		if err := rows.Scan(&s.Path, &s.Heading, &s.StartLine, &s.EndLine, &ns, &tags, &links, &s.Text); err != nil {
			return nil, err
		}
		s.Modified = time.Unix(0, ns)
		s.Path = filepath.Join(root, filepath.FromSlash(s.Path))
		s.Tags = strings.Fields(tags)
		s.Links = strings.Fields(links)
		out = append(out, s)
	}
	return out, rows.Err()
}
func (i *Index) Counts(ctx context.Context) (files, chunks int, root string, err error) {
	err = i.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM brain_files`).Scan(&files)
	if err == nil {
		err = i.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM brain_chunks`).Scan(&chunks)
	}
	if err == nil {
		root, err = i.Root(ctx)
	}
	return
}
