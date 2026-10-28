package skills

import (
	"context"
	"github.com/GasaTec/gasacli/internal/storage"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreviewInstallAndRemoveRequireConsent(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "sample")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: sample\nversion: 1.2\ndescription: Test skill\n---\n# Instructions\nNever execute this file.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Manager{Home: filepath.Join(root, "home")}
	preview, err := m.PreviewLocal(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Files) != 2 || preview.Hash == "" {
		t.Fatalf("preview=%+v", preview)
	}
	if err := m.InstallLocal(preview, false); err == nil {
		t.Fatal("install without confirmation succeeded")
	}
	if err := m.InstallLocal(preview, true); err != nil {
		t.Fatal(err)
	}
	manifest, err := parseManifest(filepath.Join(preview.Destination, "SKILL.md"), "gasacli")
	if err != nil || manifest.Name != "sample" {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	if err := os.WriteFile(filepath.Join(source, "notes.txt"), []byte("updated"), 0600); err != nil {
		t.Fatal(err)
	}
	updates, err := m.CheckUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || updates[0] != "sample" {
		t.Fatalf("updates=%v", updates)
	}
	if err := m.Remove("sample", false); err == nil {
		t.Fatal("remove without confirmation succeeded")
	}
	if err := m.Remove("sample", true); err != nil {
		t.Fatal(err)
	}
}

func TestPreviewRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "sample")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("---\nname: sample\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "elsewhere"), filepath.Join(source, "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, err := (Manager{Home: filepath.Join(root, "home")}).PreviewLocal(source)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("PreviewLocal error=%v", err)
	}
}

func TestDiscoverDeduplicatesSameSkillByNameAndHash(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	projectClaude, projectAgents := filepath.Join(project, ".claude", "skills"), filepath.Join(project, ".agents", "skills")
	content := []byte("---\nname: architecture\nversion: 1\ndescription: Same skill\n---\n")
	for _, dir := range []string{filepath.Join(projectClaude, "architecture"), filepath.Join(projectAgents, "architecture")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("USERPROFILE", root)
	t.Setenv("HOME", root)
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	items, err := (Manager{Home: filepath.Join(root, "gasacli")}).Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "architecture" || items[0].Source != "project-claude,project-agents" {
		t.Fatalf("deduplicated skills=%+v", items)
	}
}

func TestParseGitSourceRequiresPinnedHTTPSCommit(t *testing.T) {
	valid := "https://example.com/repo.git#" + strings.Repeat("a", 40)
	remote, commit, ok := ParseGitSource(valid)
	if !ok || remote != "https://example.com/repo.git" || commit != strings.Repeat("a", 40) {
		t.Fatalf("ParseGitSource=%q %q %v", remote, commit, ok)
	}
	for _, source := range []string{"https://example.com/repo.git#main", "https://user:pass@example.com/repo.git#" + strings.Repeat("a", 40), "file:///tmp/repo#" + strings.Repeat("a", 40)} {
		if _, _, ok := ParseGitSource(source); ok {
			t.Errorf("accepted unpinned or unsafe URL %q", source)
		}
	}
}

func TestRecordActivationIsIdempotent(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for n := 0; n < 2; n++ {
		if err := RecordActivation(context.Background(), db, "run-1", "sample"); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := db.DB().QueryRow(`SELECT count(*) FROM skill_run_usage`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("activation count=%d", count)
	}
}
