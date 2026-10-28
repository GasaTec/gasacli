package trust

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTrustRelativePersistenceAndRevoke(t *testing.T) {
	home := t.TempDir()
	store := NewStore(home)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Create the project under the working directory: a relative path to it always exists,
	// even when TEMP is on another volume (Windows CI: workspace on D:, TEMP on C:).
	project, err := os.MkdirTemp(cwd, "trust-rel-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(project) })
	rel, err := filepath.Rel(cwd, project)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := store.IsTrusted(rel); err != nil || got {
		t.Fatalf("initial = %v, %v", got, err)
	}
	if err := store.Grant(rel); err != nil {
		t.Fatal(err)
	}
	if got, err := NewStore(home).IsTrusted(project); err != nil || !got {
		t.Fatalf("persisted = %v, %v", got, err)
	}
	if err := store.Revoke(project); err != nil {
		t.Fatal(err)
	}
	if got, err := store.IsTrusted(rel); err != nil || got {
		t.Fatalf("revoked = %v, %v", got, err)
	}
}

func TestTrustedProjects(t *testing.T) {
	home := t.TempDir()
	store := NewStore(home)
	first, second := t.TempDir(), t.TempDir()
	if err := store.Grant(second); err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(first); err != nil {
		t.Fatal(err)
	}
	got, err := store.TrustedProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != first && got[0] != second || got[1] != first && got[1] != second || got[0] >= got[1] {
		t.Fatalf("TrustedProjects() = %#v, want sorted canonical paths", got)
	}
}

func TestTrustedProjectsFailsClosed(t *testing.T) {
	t.Run("empty store", func(t *testing.T) {
		got, err := NewStore(t.TempDir()).TrustedProjects()
		if err != nil || len(got) != 0 {
			t.Fatalf("TrustedProjects() = %#v, %v; want empty, nil", got, err)
		}
	})
	t.Run("missing store", func(t *testing.T) {
		got, err := NewStore(filepath.Join(t.TempDir(), "missing")).TrustedProjects()
		if err != nil || len(got) != 0 {
			t.Fatalf("TrustedProjects() = %#v, %v; want empty, nil", got, err)
		}
	})
	t.Run("corrupt store", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "trust.json"), []byte("{"), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := NewStore(home).TrustedProjects()
		if err == nil || len(got) != 0 {
			t.Fatalf("TrustedProjects() = %#v, %v; want error and no projects", got, err)
		}
	})
}

func TestConcurrentStoresPreserveGrantAndRevoke(t *testing.T) {
	home, projects := prepareConcurrentTrust(t)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			if err := updateConcurrentTrust(home, projects, worker); err != nil {
				t.Error(err)
			}
		}(worker)
	}
	close(start)
	wg.Wait()
	verifyConcurrentTrust(t, home, projects)
}

func prepareConcurrentTrust(t *testing.T) (string, string) {
	t.Helper()
	home, projects := t.TempDir(), t.TempDir()
	store := NewStore(home)
	for i := 0; i < 40; i++ {
		path := filepath.Join(projects, fmt.Sprint(i))
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			if err := store.Grant(path); err != nil {
				t.Fatal(err)
			}
		}
	}
	return home, projects
}

func updateConcurrentTrust(home, projects string, worker int) error {
	store := NewStore(home)
	for i := worker * 20; i < (worker+1)*20; i++ {
		path := filepath.Join(projects, fmt.Sprint(i))
		var err error
		if i%2 == 0 {
			err = store.Revoke(path)
		} else {
			err = store.Grant(path)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func verifyConcurrentTrust(t *testing.T, home, projects string) {
	t.Helper()
	for i := 0; i < 40; i++ {
		got, err := NewStore(home).IsTrusted(filepath.Join(projects, fmt.Sprint(i)))
		if err != nil || got != (i%2 == 1) {
			t.Errorf("project %d trusted = %v, %v", i, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "trust.json.lock")); !os.IsNotExist(err) {
		t.Errorf("lock remains: %v", err)
	}
}

func TestTrustProcessHelper(t *testing.T) {
	if os.Getenv("GASA_TRUST_TEST_CHILD") != "1" {
		return
	}
	home, projects := os.Getenv("GASA_TRUST_TEST_HOME"), os.Getenv("GASA_TRUST_TEST_PROJECTS")
	worker := 0
	if os.Getenv("GASA_TRUST_TEST_WORKER") == "1" {
		worker = 1
	}
	if err := os.WriteFile(filepath.Join(home, fmt.Sprintf("ready-%d", worker)), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := updateConcurrentTrust(home, projects, worker); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentProcessesPreserveGrantAndRevoke(t *testing.T) {
	home, projects := prepareConcurrentTrust(t)
	unlock, err := NewStore(home).lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	var commands []*exec.Cmd
	var outputs []*bytes.Buffer
	for worker := 0; worker < 2; worker++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTrustProcessHelper$")
		cmd.Env = append(os.Environ(), "GASA_TRUST_TEST_CHILD=1", "GASA_TRUST_TEST_HOME="+home,
			"GASA_TRUST_TEST_PROJECTS="+projects, fmt.Sprintf("GASA_TRUST_TEST_WORKER=%d", worker))
		output := new(bytes.Buffer)
		cmd.Stdout, cmd.Stderr = output, output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands, outputs = append(commands, cmd), append(outputs, output)
		t.Cleanup(func() { _ = cmd.Process.Kill() })
	}
	deadline := time.Now().Add(time.Second)
	for worker := 0; worker < 2; worker++ {
		for {
			if _, err := os.Stat(filepath.Join(home, fmt.Sprintf("ready-%d", worker))); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("child process did not become ready")
			}
			time.Sleep(time.Millisecond)
		}
	}
	unlock()
	for i, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Errorf("child %d: %v\n%s", i, err, outputs[i])
		}
	}
	verifyConcurrentTrust(t, home, projects)
}

func TestTrustLockTimeoutAndStaleRecovery(t *testing.T) {
	home, project := t.TempDir(), t.TempDir()
	path := filepath.Join(home, "trust.json.lock")
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(home).Grant(project); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("fresh lock error = %v", err)
	}
	old := time.Now().Add(-31 * time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	var warning bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&warning)
	defer log.SetOutput(previous)
	if err := NewStore(home).Grant(project); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning.String(), "removed stale project trust lock older than 30s") {
		t.Fatalf("missing stale lock warning: %s", &warning)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock remains: %v", err)
	}
}

func TestTrustSymlinkRevalidated(t *testing.T) {
	store := NewStore(t.TempDir())
	first, second := t.TempDir(), t.TempDir()
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := store.Grant(link); err != nil {
		t.Fatal(err)
	}
	if got, err := store.IsTrusted(first); err != nil || !got {
		t.Fatalf("canonical = %v, %v", got, err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if got, err := store.IsTrusted(link); err != nil || got {
		t.Fatalf("retarget = %v, %v", got, err)
	}
	if err := store.Grant(link); err != nil {
		t.Fatal(err)
	}
	if err := store.Revoke(link); err != nil {
		t.Fatal(err)
	}
	if got, err := store.IsTrusted(second); err != nil || got {
		t.Fatalf("revoke target = %v, %v", got, err)
	}
	if got, err := store.IsTrusted(first); err != nil || !got {
		t.Fatalf("original target = %v, %v", got, err)
	}
}

func TestCorruptStoreFailsClosed(t *testing.T) {
	for _, data := range []string{"{broken", "null", "[]", `{"path":"yes"}`, ""} {
		t.Run(data, func(t *testing.T) {
			home := t.TempDir()
			project := t.TempDir()
			path := filepath.Join(home, "trust.json")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			store := NewStore(home)
			if got, err := store.IsTrusted(project); err == nil || got {
				t.Fatalf("corrupt = %v, %v", got, err)
			}
			if err := store.Grant(project); err == nil {
				t.Fatal("grant accepted corruption")
			}
			if err := store.Revoke(project); err == nil {
				t.Fatal("revoke accepted corruption")
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != data {
				t.Fatalf("corrupt store overwritten: %q %v", got, err)
			}
		})
	}
}
