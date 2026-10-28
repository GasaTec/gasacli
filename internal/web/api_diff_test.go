package web

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAPIGitDiffRedactsCapsAndSkipsBinary(t *testing.T) {
	t.Setenv("GASACLI_LANG", "en")
	root := t.TempDir()
	gitTest(t, root, "init")
	gitTest(t, root, "config", "user.name", "GasaCLI Test")
	gitTest(t, root, "config", "user.email", "test@example.invalid")
	textFile := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(textFile, []byte("baseline\n"), 0600); err != nil {
		t.Fatal(err)
	}
	binaryFile := filepath.Join(root, "asset.bin")
	if err := os.WriteFile(binaryFile, []byte{0, 1, 2, 3}, 0600); err != nil {
		t.Fatal(err)
	}
	secretFile := filepath.Join(root, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("safe\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, root, "add", "notes.txt", "asset.bin", "secret.txt")
	gitTest(t, root, "commit", "-m", "baseline")
	large := strings.Repeat("ordinary line\n", 6000)
	if err := os.WriteFile(textFile, []byte(large), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binaryFile, []byte{0, 9, 8, 7}, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretFile, []byte("OPENAI_API_KEY=sk-1234567890abcdefghijklmnop\n"), 0600); err != nil {
		t.Fatal(err)
	}
	getSnapshot := func() snapshot { return snapshot{worktrees: map[string]string{"trusted": root}} }
	request := func(path string) map[string]any {
		t.Helper()
		recorder := httptest.NewRecorder()
		if !apiHandler(getSnapshot, recorder, httptest.NewRequest("GET", path, nil)) {
			t.Fatalf("API handler did not handle %s", path)
		}
		if recorder.Code != 200 {
			t.Fatalf("GET %s returned %d: %s", path, recorder.Code, recorder.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return body
	}
	status := request("/api/git/status?worktree=trusted")
	if _, ok := status["stat"]; ok {
		t.Fatal("status API still returns raw git diff --stat text")
	}
	secretDiff := request("/api/git/diff?worktree=trusted&file=secret.txt")
	secretText, _ := secretDiff["diff"].(string)
	if !strings.Contains(secretText, "[redacted]") || strings.Contains(secretText, "sk-1234567890") {
		t.Fatalf("diff secrets were not redacted: %q", secretText)
	}
	largeDiff := request("/api/git/diff?worktree=trusted&file=notes.txt")
	largeText, _ := largeDiff["diff"].(string)
	if truncated, _ := largeDiff["truncated"].(bool); !truncated || len(largeText) > 64<<10 {
		t.Fatalf("large diff was not capped: truncated=%t size=%d", truncated, len(largeText))
	}
	binary := request("/api/git/diff?worktree=trusted&file=asset.bin")
	if skipped, _ := binary["binary"].(bool); !skipped {
		t.Fatalf("binary file was not skipped: %#v", binary)
	}
}

func gitTest(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}
