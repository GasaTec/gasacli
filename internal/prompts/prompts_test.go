package prompts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListReportsEffectiveSourceAndPath(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	entries := List(home, project, "en")
	if len(entries) != 4 || entries[0].Role != "orchestrator" || entries[0].Source != "default" || !strings.HasPrefix(entries[0].Path, "embedded:") {
		t.Fatalf("default entries=%+v", entries)
	}
	user := filepath.Join(home, "prompts", "developer-en.yaml")
	projectOverride := filepath.Join(project, "gasacli", "prompts", "developer-en.yaml")
	for _, path := range []string{user, projectOverride} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("override"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range List(home, project, "en") {
		if entry.Role == "developer" && (entry.Source != "project" || entry.Path != projectOverride) {
			t.Fatalf("project override should win: %+v", entry)
		}
	}
	if _, err := os.Stat(user); err != nil {
		t.Fatal("listing should not write or remove user prompts:", err)
	}
}

func TestBundledPromptsValidateForEveryRoleAndLanguage(t *testing.T) {
	for _, role := range roles {
		for _, language := range languages {
			data, err := Default(role, language)
			if err != nil {
				t.Fatal(err)
			}
			p, err := Validate(data)
			if err != nil {
				t.Fatalf("%s/%s: %v", role, language, err)
			}
			if p.Role != role || p.Language != language {
				t.Fatalf("got %s/%s", p.Role, p.Language)
			}
		}
	}
}

func TestEnsurePreservesEditedPromptAndCreatesUpgradeCandidate(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	if _, err := Ensure(home, project); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "prompts", "developer-en.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = append(content, []byte("\n# user edit\n")...)
	if err = os.WriteFile(path, content, 0600); err != nil {
		t.Fatal(err)
	}
	upgrades, err := Ensure(home, project)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, upgrade := range upgrades {
		if upgrade.Path == path {
			found = true
			if _, err := os.Stat(upgrade.Candidate); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !found {
		t.Fatal("expected upgrade candidate report")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatal("edited prompt was overwritten")
	}
	projectPrompt := Prompt{Version: 1, Role: "developer", Language: "en", Body: "project override"}
	data := []byte("version: 1\nrole: developer\nlanguage: en\nbody: project override\n")
	_ = projectPrompt
	if err = os.MkdirAll(filepath.Join(project, "gasacli", "prompts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(project, "gasacli", "prompts", "developer-en.yaml"), data, 0600); err != nil {
		t.Fatal(err)
	}
	effective, err := Effective(home, project, "developer", "en")
	if err != nil {
		t.Fatal(err)
	}
	if effective.Body != "project override" {
		t.Fatalf("effective body = %q", effective.Body)
	}
}

func TestValidateRejectsUnknownFieldsAndUnsupportedLocale(t *testing.T) {
	if _, err := Validate([]byte("version: 1\nrole: developer\nlanguage: fr\nbody: x\n")); err == nil {
		t.Fatal("expected unsupported locale error")
	}
	if _, err := Validate([]byte("version: 1\nrole: developer\nlanguage: en\nbody: x\nunknown: true\n")); err == nil {
		t.Fatal("expected unknown field error")
	}
}
