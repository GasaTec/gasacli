package lifecycle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestValidateHomeAcceptsMarkedTemporaryDirectories(t *testing.T) {
	for _, name := range []string{"anything", "gasacli"} {
		t.Run(name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), name)
			if err := os.MkdirAll(home, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("web: {}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".gasacli"), []byte(`{"app":"gasacli","schema":1}`), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := ValidateHome(home)
			if err != nil {
				t.Fatal(err)
			}
			// ValidateHome returns the canonical spelling (Windows 8.3 short names, macOS /var ->
			// /private/var), so compare the location, not the spelling.
			want, err := filepath.EvalSymlinks(home)
			if err != nil {
				t.Fatal(err)
			}
			if gotResolved, err := filepath.EvalSymlinks(got); err != nil || gotResolved != want {
				t.Fatalf("ValidateHome() = %q (resolves to %q, err %v), want location %q", got, gotResolved, err, want)
			}
		})
	}
}

func TestWindowsPathEdgeCasesCompleteWithinFiveSeconds(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path normalization only")
	}
	done := make(chan struct{})
	go func() {
		for _, path := range []string{`C:\Users\x\NUL `, `C:\Users\x\NUL.`, `C:\Users\x\a.`, `C:\Users\x\a `, `C:\Users\x\...`, `C:\Users\x\` + strings.Repeat(`a\`, 300) + `tail`, `C:\Users\x\COM¹`, `C:\Users\x\LPT³`} {
			_ = validateWindowsSyntax(path)
			_, _ = ValidateHome(path)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("path validation did not finish within 5 seconds")
	}
}

func TestValidateHomeRequiresValidMarkerExceptLegacyDefault(t *testing.T) {
	base := t.TempDir()
	for name, marker := range map[string][]byte{"missing": nil, "empty": {}, "fake": []byte(`{"app":"other"}`), "bad-json": []byte("{"), "valid": []byte(`{"app":"gasacli","schema":1}`)} {
		t.Run(name, func(t *testing.T) {
			home := filepath.Join(base, name)
			if err := os.Mkdir(home, 0700); err != nil {
				t.Fatal(err)
			}
			if marker != nil {
				if err := os.WriteFile(filepath.Join(home, ".gasacli"), marker, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := ValidateHome(home)
			if name == "valid" && err != nil {
				t.Fatal(err)
			}
			if name != "valid" && err == nil {
				t.Fatal("accepted home without valid marker")
			}
		})
	}
	linkHome := filepath.Join(base, "symlink")
	if err := os.Mkdir(linkHome, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "missing", ".gasacli"), filepath.Join(linkHome, ".gasacli")); err == nil {
		if _, err := ValidateHome(linkHome); err == nil {
			t.Fatal("accepted linked marker")
		}
	}
}

func TestValidateHomeRejectsUnsafePaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, path string
		wantErr    bool
	}{
		{"home", home, true},
		{"home dot", filepath.Join(home, "."), true},
		{"home trailing separator", home + string(filepath.Separator), true},
		{"unmarked directory", filepath.Join(t.TempDir(), "empty"), true},
	}
	if runtime.GOOS == "windows" {
		tests = append(tests, struct {
			name, path string
			wantErr    bool
		}{"drive root", filepath.VolumeName(home) + string(filepath.Separator), true})
		for _, key := range []string{"LOCALAPPDATA", "APPDATA", "ProgramFiles", "ProgramFiles(x86)", "SystemRoot"} {
			if p := os.Getenv(key); p != "" {
				tests = append(tests, struct {
					name, path string
					wantErr    bool
				}{key, p, true})
			}
		}
		if root := os.Getenv("SystemRoot"); root != "" {
			tests = append(tests, struct {
				name, path string
				wantErr    bool
			}{"System32", filepath.Join(root, "System32"), true})
		}
		tests = append(tests,
			struct {
				name, path string
				wantErr    bool
			}{"device prefix", `\\?\C:\Users\x`, true},
			struct {
				name, path string
				wantErr    bool
			}{"UNC", `\\server\share\x`, true},
			struct {
				name, path string
				wantErr    bool
			}{"drive relative", `C:foo`, true},
			struct {
				name, path string
				wantErr    bool
			}{"fullwidth drive", `Ｃ:\Users\x`, true},
			struct {
				name, path string
				wantErr    bool
			}{"ads", `C:\Users\x:stream`, true},
			struct {
				name, path string
				wantErr    bool
			}{"environment literal", `%USERPROFILE%`, true},
			struct {
				name, path string
				wantErr    bool
			}{"reserved device", `C:\Users\x\NUL`, true},
			struct {
				name, path string
				wantErr    bool
			}{"dot dot reaches root", `C:\Users\..`, true},
		)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ValidateHome(tc.path); (err != nil) != tc.wantErr {
				t.Fatalf("ValidateHome(%q) error = %v", tc.path, err)
			}
		})
	}
}

func TestValidateHomeRejectsRepositoryAndOrcaLookalikes(t *testing.T) {
	for _, name := range []string{"repository", "orca"} {
		t.Run(name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), name)
			if err := os.MkdirAll(filepath.Join(home, "logs"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("web: {}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			if name == "repository" {
				if err := os.Mkdir(filepath.Join(home, ".git"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			_, err := ValidateHome(home)
			if err == nil || !strings.Contains(err.Error(), "missing .gasacli marker") {
				t.Fatalf("lookalike accepted or unclear error: %v", err)
			}
		})
	}
}

func TestRejectProtectedPathIncludesDescendants(t *testing.T) {
	if runtime.GOOS == "windows" {
		for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)", "ProgramW6432", "ProgramData", "SystemRoot", "windir"} {
			t.Setenv(key, `C:\SafeRoot`)
		}
		for _, path := range []string{`C:\SafeRoot\DeskIn`, `C:\SafeRoot\Microsoft OneDrive\setup`, `C:\SafeRoot\WindowsApps\pkg`} {
			if err := rejectProtectedPath(path); err == nil {
				t.Errorf("accepted protected descendant %q", path)
			}
		}
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{"/usr/example", "/bin/example", "/Users/Shared/example"} {
			if err := rejectProtectedPath(path); err == nil {
				t.Errorf("accepted protected descendant %q", path)
			}
		}
	}
}

func TestValidateHomeRejectsSymlinkComponent(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "target")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(home, link); err != nil {
		t.Skipf("symlink unavailable in this environment: %v", err)
	}
	defer os.Remove(link)
	if _, err := ValidateHome(link); err == nil {
		t.Fatal("ValidateHome accepted symlink component")
	}
}
