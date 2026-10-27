//go:build windows

package trust

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCanonicalWindowsJunction(t *testing.T) {
	root := t.TempDir()
	target, junction := filepath.Join(root, "target"), filepath.Join(root, "junction")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("cmd", "/c", "mklink", "/J", junction, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create NTFS junction: %v: %s", err, output)
	}
	want, err := canonical(target)
	if err != nil {
		t.Fatal(err)
	}
	got, err := canonical(junction)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("canonical(target) = %q; canonical(junction) = %q; equal = %v", want, got, want == got)
	if got != want {
		t.Fatal("NTFS junction did not resolve to target")
	}
	child := filepath.Join(target, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	resolvedChild, err := Canonical(filepath.Join(junction, "child"))
	if err != nil || resolvedChild != child {
		t.Fatalf("junction ancestor = %q, %v; want %q", resolvedChild, err, child)
	}
	store := NewStore(t.TempDir())
	if err := store.Grant(junction); err != nil {
		t.Fatal(err)
	}
	if trusted, err := store.IsTrusted(target); err != nil || !trusted {
		t.Fatalf("junction grant = %v, %v", trusted, err)
	}
	if err := store.Revoke(target); err != nil {
		t.Fatal(err)
	}
	if trusted, err := store.IsTrusted(junction); err != nil || trusted {
		t.Fatalf("junction revoke = %v, %v", trusted, err)
	}
	exported, err := Canonical(junction)
	if err != nil || exported != want {
		t.Fatalf("Canonical(junction) = %q, %v", exported, err)
	}
}
