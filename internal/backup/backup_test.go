package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GasaTec/gasacli/internal/config"
)

func TestCreateReadAndRestoreConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	if err := config.Ensure(); err != nil {
		t.Fatal(err)
	}
	original := []byte("web:\n  mode: manual\n  host: 127.0.0.1\n  port: 4040\n  fallback_port: true\n")
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), original, 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if err := Create(context.Background(), home, archive, Options{Scopes: []string{"config"}, Version: "test"}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	m, files, err := Read(archive, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := Preview(home, m, files); len(got) == 0 || !strings.HasPrefix(got[0], "new ") {
		t.Fatalf("preview=%v", got)
	}
	if _, err = Restore(context.Background(), home, archive, "", "test"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil || string(got) != string(original) {
		t.Fatalf("restored bytes=%q err=%v", got, err)
	}
	entries, err := os.ReadDir(filepath.Join(home, "backups"))
	if err != nil || len(entries) == 0 {
		t.Fatalf("pre-restore snapshot entries=%v err=%v", entries, err)
	}
}

func TestEncryptedArchiveRejectsWrongPassphraseAndTamper(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("safe"), 0600); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "encrypted.zip")
	if err := Create(context.Background(), home, archive, Options{Scopes: []string{"config"}, Passphrase: "correct horse", Version: "test"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(archive, "wrong"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	b, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-1] ^= 0xff
	if err = os.WriteFile(archive, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Read(archive, "correct horse"); err == nil {
		t.Fatal("tampered archive accepted")
	}
}

func TestDatabaseScopeUsesConsistentSnapshot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("GASACLI_HOME", home)
	if err := config.Ensure(); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "database.zip")
	if err := Create(context.Background(), home, archive, Options{Scopes: []string{"database"}, Version: "test"}); err != nil {
		t.Fatal(err)
	}
	_, files, err := Read(archive, "")
	if err != nil {
		t.Fatal(err)
	}
	data, ok := files["gasacli.db"]
	if !ok || len(data) == 0 || string(data[:min(len(data), 16)]) != "SQLite format 3\x00" {
		t.Fatalf("database snapshot invalid, present=%t size=%d", ok, len(data))
	}
}

func TestReadRejectsTruncatedAndTraversal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "short.zip")
	if err := os.WriteFile(p, []byte("PK\003\004"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Read(p, ""); err == nil {
		t.Fatal("truncated archive accepted")
	}
	if safeName("../outside") || safeName("C:/outside") || safeName("agents/../../outside") {
		t.Fatal("traversal path accepted")
	}
	archive := filepath.Join(t.TempDir(), "traversal.zip")
	writeTestArchive(t, archive, Manifest{FormatVersion: FormatVersion, CreatedAt: time.Now(), Scopes: []string{"config"}, Files: []Entry{{Path: "../outside", Size: 0}}}, map[string]string{"files/../outside": ""})
	if _, _, err := Read(archive, ""); err == nil {
		t.Fatal("archive traversal entry accepted")
	}
}

func TestReadRejectsHashTampering(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "tampered.zip")
	good := []byte("good")
	sum := sha256.Sum256(good)
	m := Manifest{FormatVersion: FormatVersion, CreatedAt: time.Now(), Scopes: []string{"config"}, Files: []Entry{{Path: "config.yaml", SHA256: hex.EncodeToString(sum[:]), Size: 4}}}
	writeTestArchive(t, archive, m, map[string]string{"files/config.yaml": "evil"})
	if _, _, err := Read(archive, ""); err == nil {
		t.Fatal("tampered content accepted")
	}
}

func writeTestArchive(t *testing.T, path string, m Manifest, entries map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	mf, err := w.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = mf.Write(manifest); err != nil {
		t.Fatal(err)
	}
	for name, data := range entries {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
