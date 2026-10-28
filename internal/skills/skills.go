// Package skills discovers and manages local Markdown agent skills.
package skills

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/lifecycle"
)

type Skill struct{ Name, Description, Version, Source, Hash, Path string }
type Manager struct{ Home string }

type installReceipt struct {
	Source string `json:"source"`
	Hash   string `json:"hash"`
}

const (
	maxSkillFileBytes  = 2 << 20
	maxSkillTotalBytes = 10 << 20
)

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)

// Discover reads only known skill locations and never runs their contents.
func (m Manager) Discover() ([]Skill, error) {
	paths := []struct{ path, source string }{}
	add := func(p, s string) {
		if p != "" {
			paths = append(paths, struct{ path, source string }{p, s})
		}
	}
	user, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	add(filepath.Join(user, ".claude", "skills"), "claude")
	add(filepath.Join(user, ".claude", "plugins"), "claude-plugin")
	add(filepath.Join(user, ".codex", "skills"), "codex")
	add(filepath.Join(user, ".agents", "skills"), "agents")
	project, _ := os.Getwd()
	add(filepath.Join(project, ".claude", "skills"), "project-claude")
	add(filepath.Join(project, ".agents", "skills"), "project-agents")
	add(filepath.Join(m.Home, "skills"), "gasacli")
	seen := map[string]bool{}
	var out []Skill
	for _, base := range paths {
		count := 0
		e := filepath.WalkDir(base.path, func(path string, entry fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if path == base.path {
				return nil
			}
			count++
			if count > 10000 {
				return errors.New("skill discovery exceeded 10,000 entries")
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				rel, _ := filepath.Rel(base.path, path)
				if strings.Count(filepath.ToSlash(rel), "/") >= 5 {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() != "SKILL.md" {
				return nil
			}
			skill, err := parseManifest(path, base.source)
			if err != nil {
				return err
			}
			key := strings.ToLower(skill.Name + "\x00" + skill.Path)
			if !seen[key] {
				seen[key] = true
				out = append(out, skill)
			}
			return nil
		})
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return nil, e
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Source < out[j].Source
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func parseManifest(path, source string) (Skill, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Skill{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) {
		return Skill{}, fmt.Errorf("refusing non-regular skill manifest %s", path)
	}
	if info.Size() > 256<<10 {
		return Skill{}, fmt.Errorf("skill manifest too large: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Skill{}, err
	}
	name, desc, version := filepath.Base(filepath.Dir(path)), "", ""
	text := strings.TrimPrefix(strings.ReplaceAll(string(data), "\r\n", "\n"), "\ufeff")
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			for _, line := range strings.Split(text[4:4+end], "\n") {
				key, value, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				value = strings.Trim(strings.TrimSpace(value), "\"'")
				switch strings.TrimSpace(key) {
				case "name":
					name = value
				case "description":
					desc = value
				case "version":
					version = value
				}
			}
		}
	}
	if !nameRE.MatchString(name) {
		return Skill{}, fmt.Errorf("invalid skill name in %s", path)
	}
	sum := sha256.Sum256(data)
	abs, _ := filepath.Abs(path)
	return Skill{Name: name, Description: desc, Version: version, Source: source, Hash: hex.EncodeToString(sum[:]), Path: abs}, nil
}

type Preview struct {
	Name, Source, Destination, Hash string
	Files                           []string
}

// PreviewLocal validates a local SKILL.md tree and lists every file that would be copied.
func (m Manager) PreviewLocal(source string) (Preview, error) {
	src, err := filepath.Abs(source)
	if err != nil {
		return Preview{}, err
	}
	rootInfo, err := os.Lstat(src)
	if err != nil {
		return Preview{}, err
	}
	if !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(rootInfo) {
		return Preview{}, errors.New("skill source must be a regular directory")
	}
	manifest, err := parseManifest(filepath.Join(src, "SKILL.md"), "local")
	if err != nil {
		return Preview{}, err
	}
	dst := filepath.Join(m.Home, "skills", manifest.Name)
	if _, err = os.Lstat(dst); err == nil {
		return Preview{}, fmt.Errorf("skill %q is already installed", manifest.Name)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Preview{}, err
	}
	files := []string{}
	var totalBytes int64
	err = filepath.WalkDir(src, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if path == src {
			return nil
		}
		rel, e := filepath.Rel(src, path)
		if e != nil {
			return e
		}
		if rel == ".git" || strings.HasPrefix(filepath.ToSlash(rel), ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink in skill: %s", path)
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular skill file: %s", path)
		}
		if !info.IsDir() {
			if info.Size() > maxSkillFileBytes {
				return fmt.Errorf("skill file exceeds %d bytes: %s", maxSkillFileBytes, path)
			}
			totalBytes += info.Size()
			if totalBytes > maxSkillTotalBytes {
				return fmt.Errorf("skill exceeds %d total bytes", maxSkillTotalBytes)
			}
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return Preview{}, err
	}
	sort.Strings(files)
	if len(files) > 500 {
		return Preview{}, errors.New("skill contains more than 500 files")
	}
	hash, err := hashTree(src, files)
	if err != nil {
		return Preview{}, err
	}
	return Preview{Name: manifest.Name, Source: src, Destination: dst, Hash: hash, Files: files}, nil
}

// InstallLocal requires a separate explicit confirmation after PreviewLocal.
func (m Manager) InstallLocal(preview Preview, confirmed bool) error {
	if !confirmed {
		return errors.New("installation requires explicit confirmation after reviewing the file list")
	}
	again, err := m.PreviewLocal(preview.Source)
	if err != nil {
		return err
	}
	if again.Hash != preview.Hash || strings.Join(again.Files, "\n") != strings.Join(preview.Files, "\n") {
		return errors.New("skill changed after preview; review a new preview")
	}
	if err = os.MkdirAll(filepath.Dir(preview.Destination), 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(preview.Destination), ".skill-install-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, rel := range preview.Files {
		src := filepath.Join(preview.Source, filepath.FromSlash(rel))
		dst := filepath.Join(stage, filepath.FromSlash(rel))
		info, e := os.Stat(src)
		if e != nil {
			return e
		}
		if info.IsDir() {
			if e = os.MkdirAll(dst, 0700); e != nil {
				return e
			}
			continue
		}
		if e = os.MkdirAll(filepath.Dir(dst), 0700); e != nil {
			return e
		}
		data, e := os.ReadFile(src)
		if e != nil {
			return e
		}
		if e = os.WriteFile(dst, data, 0600); e != nil {
			return e
		}
	}
	if err = os.Rename(stage, preview.Destination); err != nil {
		return err
	}
	registry := filepath.Join(m.Home, "skill-registry")
	if err = os.MkdirAll(registry, 0700); err != nil {
		_ = os.RemoveAll(preview.Destination)
		return err
	}
	data, err := json.Marshal(installReceipt{Source: preview.Source, Hash: preview.Hash})
	if err != nil {
		_ = os.RemoveAll(preview.Destination)
		return err
	}
	if err = os.WriteFile(filepath.Join(registry, preview.Name+".json"), append(data, '\n'), 0600); err != nil {
		_ = os.RemoveAll(preview.Destination)
		return err
	}
	return nil
}

func (m Manager) Remove(name string, confirmed bool) error {
	if !nameRE.MatchString(name) {
		return errors.New("invalid skill name")
	}
	if !confirmed {
		return errors.New("removal requires explicit confirmation")
	}
	path := filepath.Join(m.Home, "skills", name)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) {
		return errors.New("refusing unsafe skill directory")
	}
	manifest, err := parseManifest(filepath.Join(path, "SKILL.md"), "gasacli")
	if err != nil {
		return err
	}
	if manifest.Name != name {
		return errors.New("skill name does not match managed directory")
	}
	var receipt installReceipt
	if data, readErr := os.ReadFile(filepath.Join(m.Home, "skill-registry", name+".json")); readErr == nil {
		_ = json.Unmarshal(data, &receipt)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(m.Home, "skill-registry", name+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	staging := filepath.Join(m.Home, "skill-staging")
	if receipt.Source != "" {
		rel, e := filepath.Rel(staging, receipt.Source)
		if e == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			if err := os.RemoveAll(receipt.Source); err != nil {
				return err
			}
		}
	}
	return nil
}

// CheckUpdates compares installed hashes with their current source directories.
func (m Manager) CheckUpdates() ([]string, error) {
	skills, err := m.Discover()
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, s := range skills {
		if s.Source != "gasacli" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.Home, "skill-registry", s.Name+".json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var receipt installReceipt
		if err = json.Unmarshal(data, &receipt); err != nil {
			return nil, err
		}
		files := []string{}
		err = filepath.WalkDir(receipt.Source, func(path string, d fs.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if path == receipt.Source {
				return nil
			}
			rel, e := filepath.Rel(receipt.Source, path)
			if e != nil {
				return e
			}
			if rel == ".git" || strings.HasPrefix(filepath.ToSlash(rel), ".git/") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing symlink in skill source: %s", path)
			}
			if d.IsDir() {
				return nil
			}
			files = append(files, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		current, err := hashTree(receipt.Source, files)
		if err != nil {
			return nil, err
		}
		if current != receipt.Hash {
			changed = append(changed, s.Name)
		}
	}
	return changed, nil
}

func hashTree(root string, files []string) (string, error) {
	h := sha256.New()
	for _, rel := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func (m Manager) PreviewSourceHash(path string) (string, error) {
	sum, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(sum)
	return hex.EncodeToString(hash[:]), nil
}

var commitRE = regexp.MustCompile(`^[0-9a-fA-F]{40,64}$`)

func ParseGitSource(source string) (remote, commit string, ok bool) {
	if !strings.HasPrefix(source, "https://") {
		return "", "", false
	}
	parts := strings.SplitN(source, "#", 2)
	if len(parts) != 2 || !commitRE.MatchString(parts[1]) {
		return "", "", false
	}
	parsed, err := url.Parse(parts[0])
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Opaque != "" {
		return "", "", false
	}
	return parts[0], strings.ToLower(parts[1]), true
}

// ResolveGitSource checks out only a pinned HTTPS commit and never executes repository files.
func (m Manager) ResolveGitSource(ctx context.Context, source string, fetch bool) (string, error) {
	remote, commit, ok := ParseGitSource(source)
	if !ok {
		return "", errors.New("Git skill source must be an HTTPS URL with #<full commit SHA>")
	}
	dest := filepath.Join(m.Home, "skill-staging", commit)
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) {
			return "", errors.New("refusing unsafe cached Git skill source")
		}
		output, verifyErr := exec.CommandContext(ctx, "git", "-C", dest, "rev-parse", "HEAD").Output()
		if verifyErr != nil || strings.TrimSpace(string(output)) != commit {
			return "", errors.New("cached Git skill source does not match the pinned commit")
		}
		if _, manifestErr := parseManifest(filepath.Join(dest, "SKILL.md"), "git"); manifestErr != nil {
			return "", manifestErr
		}
		return dest, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if !fetch {
		return "", errors.New("pinned Git source is not cached; rerun with --fetch after approving network access")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".skill-fetch-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	configFile, err := os.CreateTemp(filepath.Dir(stage), ".gitconfig-empty-")
	if err != nil {
		return "", err
	}
	emptyConfig := configFile.Name()
	if err := configFile.Close(); err != nil {
		return "", err
	}
	defer os.Remove(emptyConfig)
	if err := os.Chmod(emptyConfig, 0600); err != nil {
		return "", err
	}
	if err := runGit(ctx, emptyConfig, "clone", "--quiet", "--no-checkout", "--", remote, stage); err != nil {
		return "", fmt.Errorf("clone pinned skill source: %w", err)
	}
	if err := runGit(ctx, emptyConfig, "-C", stage, "checkout", "--quiet", "--detach", commit); err != nil {
		return "", fmt.Errorf("check out pinned skill commit: %w", err)
	}
	output, err := runGitOutput(ctx, emptyConfig, "-C", stage, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(string(output)) != commit {
		return "", errors.New("Git source did not resolve to the requested pinned commit")
	}
	if _, err := parseManifest(filepath.Join(stage, "SKILL.md"), "git"); err != nil {
		return "", err
	}
	if err := os.Rename(stage, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func gitEnv(emptyConfig string) []string {
	env := os.Environ()
	filtered := env[:0]
	for _, item := range env {
		if strings.HasPrefix(strings.ToUpper(item), "GIT_CONFIG_GLOBAL=") || strings.HasPrefix(strings.ToUpper(item), "GIT_CONFIG_NOSYSTEM=") || strings.HasPrefix(strings.ToUpper(item), "GIT_TERMINAL_PROMPT=") || strings.HasPrefix(strings.ToUpper(item), "GIT_LFS_SKIP_SMUDGE=") {
			continue
		}
		filtered = append(filtered, item)
	}
	return append(filtered, "GIT_CONFIG_GLOBAL="+emptyConfig, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_LFS_SKIP_SMUDGE=1")
}
func runGit(ctx context.Context, emptyConfig string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitEnv(emptyConfig)
	return cmd.Run()
}
func runGitOutput(ctx context.Context, emptyConfig string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = gitEnv(emptyConfig)
	return cmd.Output()
}
