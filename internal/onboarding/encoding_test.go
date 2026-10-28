package onboarding

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSourceFilesContainNoMojibakeSequences(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
	bad := []string{string([]rune{0x00c3, 0x00a7}), string([]rune{0x00c3, 0x00e3}), string([]rune{0x00c3, 0x00a1}), string([]rune{0x00c3, 0x00a9}), string([]rune{0x00c3, 0x0192})}
	for _, dir := range []string{"cmd", "internal", "test"} {
		base := filepath.Join(root, dir)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".go" && ext != ".md" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, sequence := range bad {
				if strings.Contains(string(data), sequence) {
					t.Errorf("%s contains mojibake sequence %q", path, sequence)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "onboarding.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sequence := range bad {
		if strings.Contains(string(data), sequence) {
			t.Errorf("docs/onboarding.md contains mojibake sequence %q", sequence)
		}
	}
}
