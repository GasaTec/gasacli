package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/GasaTec/gasacli/internal/storage"
)

func TestIndexSearchIsIncrementalAndDoesNotWriteVault(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/finance.md", "---\ntags: [permissões, segurança]\n---\n# Permissões\nO ADR define as permissões locais. Veja [ADR-0001](adr-0001.md).\n")
	write(".obsidian/workspace.json", `{"workspace":true}`)
	before := snapshot(t, root)
	db, err := storage.Open(filepath.Join(t.TempDir(), "gasacli.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	idx, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.Connect(ctx, root); err != nil {
		t.Fatal(err)
	}
	stats, err := idx.Reindex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 || stats.Chunks != 1 {
		t.Fatalf("first index stats=%+v", stats)
	}
	results, err := idx.Search(ctx, "permissões ADR", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results=%+v", results)
	}
	r := results[0]
	if r.Path != filepath.Join(root, "docs", "finance.md") || r.Heading != "Permissões" || r.StartLine != 5 || r.EndLine < 5 {
		t.Fatalf("bad provenance: %+v", r)
	}
	if len(r.Tags) != 2 || len(r.Links) != 1 {
		t.Fatalf("metadata missing: %+v", r)
	}
	tagResults, err := idx.Search(ctx, "segurança", 10)
	if err != nil || len(tagResults) != 1 {
		t.Fatalf("frontmatter tag search=%+v err=%v", tagResults, err)
	}
	second, err := idx.Reindex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second.Files != 0 || second.Skipped != 1 {
		t.Fatalf("second index stats=%+v", second)
	}
	if after := snapshot(t, root); after != before {
		t.Fatalf("vault bytes changed: before=%s after=%s", before, after)
	}
}

func TestSearchRequiresConnectedVault(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	idx, _ := New(db)
	if _, err := idx.Search(context.Background(), "ADR", 1); err == nil {
		t.Fatal("search without a vault succeeded")
	}
}

func TestIgnoreGlobsRejectTraversal(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	idx, _ := New(db)
	for _, pattern := range []string{"../private/**", "/absolute/**", "["} {
		if err := idx.SetIgnoreGlobs(context.Background(), []string{pattern}); err == nil {
			t.Errorf("accepted unsafe or malformed glob %q", pattern)
		}
	}
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
