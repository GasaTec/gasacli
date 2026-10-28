// Package prompts provides versioned embedded role prompts and safe local overrides.
package prompts

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed defaults/*.yaml
var bundled embed.FS

type Prompt struct {
	Version  int    `yaml:"version"`
	Role     string `yaml:"role"`
	Language string `yaml:"language"`
	Body     string `yaml:"body"`
}

type Upgrade struct {
	Path      string
	Candidate string
}

// Entry describes the effective prompt file for one built-in role.
type Entry struct {
	Role   string
	Source string
	Path   string
}

var roles = []string{"orchestrator", "tech-lead", "developer", "tester"}
var languages = []string{"pt-BR", "en"}

// List reports each built-in role and the highest-priority override that exists.
func List(home, project, language string) []Entry {
	entries := make([]Entry, 0, len(roles))
	for _, role := range roles {
		entry := Entry{Role: role, Source: "default", Path: "embedded: " + role + "-" + language}
		userPath := filepath.Join(home, "prompts", role+"-"+language+".yaml")
		if _, err := os.Stat(userPath); err == nil {
			entry.Source, entry.Path = "user", userPath
		}
		if project != "" {
			projectPath := filepath.Join(project, "gasacli", "prompts", role+"-"+language+".yaml")
			if _, err := os.Stat(projectPath); err == nil {
				entry.Source, entry.Path = "project", projectPath
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

func Validate(data []byte) (Prompt, error) {
	var p Prompt
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&p); err != nil {
		return Prompt{}, err
	}
	if p.Version < 1 || !contains(roles, p.Role) || !contains(languages, p.Language) || strings.TrimSpace(p.Body) == "" {
		return Prompt{}, errors.New("prompt schema requires version, supported role, language, and body")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return Prompt{}, err
		}
		return Prompt{}, errors.New("prompt must contain one YAML document")
	}
	return p, nil
}

func Default(role, language string) ([]byte, error) {
	if !contains(roles, role) || !contains(languages, language) {
		return nil, fmt.Errorf("prompts: unsupported role or language")
	}
	return bundled.ReadFile(filepath.ToSlash(filepath.Join("defaults", role+"-"+language+".yaml")))
}

// Ensure initializes global and project overrides without replacing edited files.
// Changed bundled defaults are written beside the user's version for review.
func Ensure(home, project string) ([]Upgrade, error) {
	var upgrades []Upgrade
	for _, root := range []string{filepath.Join(home, "prompts"), filepath.Join(project, "gasacli", "prompts")} {
		if root == filepath.Join(project, "gasacli", "prompts") && project == "" {
			continue
		}
		if err := os.MkdirAll(root, 0700); err != nil {
			return upgrades, err
		}
		for _, role := range roles {
			for _, language := range languages {
				name := role + "-" + language + ".yaml"
				target := filepath.Join(root, name)
				defaults, err := Default(role, language)
				if err != nil {
					return upgrades, err
				}
				current, err := os.ReadFile(target)
				if errors.Is(err, os.ErrNotExist) {
					if err := atomicWrite(target, defaults); err != nil {
						return upgrades, err
					}
					continue
				}
				if err != nil {
					return upgrades, err
				}
				if _, err := Validate(current); err != nil {
					return upgrades, fmt.Errorf("prompts: invalid override %s: %w", target, err)
				}
				if !bytes.Equal(current, defaults) {
					candidate := target + ".new"
					if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
						if err := atomicWrite(candidate, defaults); err != nil {
							return upgrades, err
						}
					}
					diffPath := target + ".diff"
					diff := []byte("--- " + name + " (current)\n+++ " + name + " (bundled)\n" + simpleDiff(current, defaults))
					if err := atomicWrite(diffPath, diff); err != nil {
						return upgrades, err
					}
					upgrades = append(upgrades, Upgrade{Path: target, Candidate: candidate})
				}
			}
		}
	}
	return upgrades, nil
}

func Effective(home, project, role, language string) (Prompt, error) {
	if _, err := Default(role, language); err != nil {
		return Prompt{}, err
	}
	paths := []string{filepath.Join(home, "prompts", role+"-"+language+".yaml")}
	if project != "" {
		paths = append([]string{filepath.Join(project, "gasacli", "prompts", role+"-"+language+".yaml")}, paths...)
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return Validate(data)
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Prompt{}, err
		}
	}
	data, err := Default(role, language)
	if err != nil {
		return Prompt{}, err
	}
	return Validate(data)
}

func Reset(home, project, role, language string) error {
	data, err := Default(role, language)
	if err != nil {
		return err
	}
	root := filepath.Join(home, "prompts")
	if project != "" {
		root = filepath.Join(project, "gasacli", "prompts")
	}
	return atomicWrite(filepath.Join(root, role+"-"+language+".yaml"), data)
}

func Path(home, project, role, language string) string {
	if project != "" {
		path := filepath.Join(project, "gasacli", "prompts", role+"-"+language+".yaml")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	path := filepath.Join(home, "prompts", role+"-"+language+".yaml")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return "embedded: " + role + "-" + language
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".prompt-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(data)
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(temp.Name(), path); err != nil {
		return err
	}
	return nil
}

func simpleDiff(old, next []byte) string {
	left, right := strings.Split(strings.TrimSuffix(string(old), "\n"), "\n"), strings.Split(strings.TrimSuffix(string(next), "\n"), "\n")
	var out strings.Builder
	for _, line := range left {
		fmt.Fprintln(&out, "-"+line)
	}
	for _, line := range right {
		fmt.Fprintln(&out, "+"+line)
	}
	return out.String()
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}
