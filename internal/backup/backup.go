// Package backup creates and restores bounded, integrity-checked GasaCLI archives.
package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GasaTec/gasacli/internal/lifecycle"
	_ "modernc.org/sqlite"
)

const FormatVersion = 1
const maxArchiveBytes = 512 << 20
const maxFileBytes = 64 << 20

type Options struct {
	Scopes     []string
	Passphrase string
	Version    string
}
type Entry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Manifest struct {
	FormatVersion int       `json:"format_version"`
	GasaCLI       string    `json:"gasacli_version"`
	CreatedAt     time.Time `json:"created_at"`
	Scopes        []string  `json:"scopes"`
	Files         []Entry   `json:"files"`
	Encrypted     bool      `json:"encrypted,omitempty"`
	Salt          string    `json:"salt,omitempty"`
	Nonce         string    `json:"nonce,omitempty"`
}

func Create(ctx context.Context, home, output string, opts Options) error {
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return err
	}
	if len(opts.Scopes) == 0 {
		opts.Scopes = []string{"config", "prompts", "skills", "agents", "segundo-cerebro", "database"}
	}
	if err := validateScopes(opts.Scopes); err != nil {
		return err
	}
	files := map[string][]byte{}
	for _, scope := range opts.Scopes {
		if scope == "database" {
			data, err := snapshotDB(ctx, filepath.Join(home, "gasacli.db"))
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return err
			}
			files["gasacli.db"] = data
			continue
		}
		if scope == "config" {
			for _, name := range []string{"config.yaml", "state.json", "permissions.yaml"} {
				addRegularFile(home, name, files)
			}
			continue
		}
		if scope == "segundo-cerebro" {
			addRegularFile(home, "segundo-cerebro/index.json", files)
			continue
		}
		if err := walkScope(home, scope, files); err != nil {
			return err
		}
	}
	var total int64
	for _, data := range files {
		total += int64(len(data))
		if total > maxArchiveBytes {
			return fmt.Errorf("backup contents exceed size limit")
		}
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	manifest := Manifest{FormatVersion: FormatVersion, GasaCLI: opts.Version, CreatedAt: time.Now().UTC(), Scopes: append([]string(nil), opts.Scopes...)}
	for _, name := range keys {
		b := files[name]
		h := sha256.Sum256(b)
		manifest.Files = append(manifest.Files, Entry{Path: name, SHA256: hex.EncodeToString(h[:]), Size: int64(len(b))})
	}
	if opts.Passphrase != "" {
		salt := make([]byte, 16)
		nonce := make([]byte, 12)
		if _, err := rand.Read(salt); err != nil {
			return err
		}
		if _, err := rand.Read(nonce); err != nil {
			return err
		}
		manifest.Encrypted = true
		manifest.Salt = base64.StdEncoding.EncodeToString(salt)
		manifest.Nonce = base64.StdEncoding.EncodeToString(nonce)
		mb, err := json.Marshal(manifest)
		if err != nil {
			return err
		}
		key := derive(opts.Passphrase, salt)
		block, _ := aes.NewCipher(key)
		g, _ := cipher.NewGCM(block)
		payload, err := encodeZip(mb, files)
		if err != nil {
			return err
		}
		sealed := g.Seal(nil, nonce, payload, []byte("GASA-BACKUP-1"))
		wrapped := append([]byte("GASA-BACKUP-1"), salt...)
		wrapped = append(wrapped, nonce...)
		wrapped = append(wrapped, sealed...)
		return atomicWrite(output, wrapped)
	}
	mb, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return writeZip(output, mb, files)
}

func encodeZip(manifest []byte, files map[string][]byte) ([]byte, error) {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, e := w.Create("manifest.json")
	if e != nil {
		return nil, e
	}
	if _, e = f.Write(manifest); e != nil {
		return nil, e
	}
	for n, d := range files {
		f, e = w.Create("files/" + n)
		if e != nil {
			return nil, e
		}
		if _, e = f.Write(d); e != nil {
			return nil, e
		}
	}
	if e := w.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}

func writeZip(path string, manifest []byte, files map[string][]byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	zw := zip.NewWriter(tmp)
	f, err := zw.Create("manifest.json")
	if err == nil {
		_, err = f.Write(manifest)
	}
	for n, d := range files {
		if err != nil {
			break
		}
		f, err = zw.Create("files/" + n)
		if err == nil {
			_, err = f.Write(d)
		}
	}
	if closeErr := zw.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func Read(path, passphrase string) (Manifest, map[string][]byte, error) {
	st, err := os.Stat(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	if st.Size() > maxArchiveBytes {
		return Manifest{}, nil, fmt.Errorf("backup archive exceeds size limit")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	const header = "GASA-BACKUP-1"
	if strings.HasPrefix(string(raw), header) {
		if len(raw) < len(header)+16+12 || passphrase == "" {
			return Manifest{}, nil, fmt.Errorf("backup requires a passphrase")
		}
		salt, nonce := raw[len(header):len(header)+16], raw[len(header)+16:len(header)+28]
		key := derive(passphrase, salt)
		block, _ := aes.NewCipher(key)
		g, _ := cipher.NewGCM(block)
		payload, err := g.Open(nil, nonce, raw[len(header)+28:], []byte(header))
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("invalid passphrase or encrypted backup: %w", err)
		}
		m, files, err := readZip(payload)
		if err == nil && !m.Encrypted {
			return m, nil, fmt.Errorf("encrypted archive manifest mismatch")
		}
		return m, files, err
	}
	m, files, err := readZip(raw)
	if err == nil && m.Encrypted {
		return m, nil, fmt.Errorf("unencrypted archive manifest mismatch")
	}
	return m, files, err
}
func readZip(raw []byte) (Manifest, map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return Manifest{}, nil, err
	}
	var m Manifest
	files := map[string][]byte{}
	seen := map[string]bool{}
	var total int64
	for _, f := range zr.File {
		if seen[f.Name] {
			return m, nil, fmt.Errorf("duplicate archive entry")
		}
		seen[f.Name] = true
		if f.Mode()&os.ModeSymlink != 0 {
			return m, nil, fmt.Errorf("symlink entries are not allowed")
		}
		if f.Name == "manifest.json" {
			if f.UncompressedSize64 > 1<<20 {
				return m, nil, fmt.Errorf("backup manifest exceeds size limit")
			}
			r, e := f.Open()
			if e != nil {
				return m, nil, e
			}
			e = json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&m)
			r.Close()
			if e != nil {
				return m, nil, e
			}
			continue
		}
		if !strings.HasPrefix(f.Name, "files/") {
			return m, nil, fmt.Errorf("unexpected archive path")
		}
		name := strings.TrimPrefix(f.Name, "files/")
		if !safeName(name) || f.UncompressedSize64 > maxFileBytes {
			return m, nil, fmt.Errorf("unsafe or oversized archive file")
		}
		r, e := f.Open()
		if e != nil {
			return m, nil, e
		}
		data, e := io.ReadAll(io.LimitReader(r, maxFileBytes+1))
		r.Close()
		if e != nil {
			return m, nil, e
		}
		if len(data) > maxFileBytes {
			return m, nil, fmt.Errorf("archive file exceeds size limit")
		}
		total += int64(len(data))
		if total > maxArchiveBytes {
			return m, nil, fmt.Errorf("backup contents exceed size limit")
		}
		files[name] = data
	}
	if m.FormatVersion != FormatVersion {
		return m, nil, fmt.Errorf("unsupported backup format version %d", m.FormatVersion)
	}
	if err := validateScopes(m.Scopes); err != nil {
		return m, nil, err
	}
	if err := verify(m, files); err != nil {
		return m, nil, err
	}
	return m, files, nil
}

func Preview(home string, m Manifest, files map[string][]byte) []string {
	var out []string
	for name := range files {
		if _, e := os.Lstat(filepath.Join(home, filepath.FromSlash(name))); errors.Is(e, os.ErrNotExist) {
			out = append(out, "new "+name)
		} else {
			out = append(out, "overwrite "+name)
		}
	}
	sort.Strings(out)
	return out
}
func Restore(ctx context.Context, home, archive, passphrase, version string) ([]string, error) {
	m, files, err := Read(archive, passphrase)
	if err != nil {
		return nil, err
	}
	if version != "" && m.GasaCLI != "" && m.GasaCLI != version {
		return nil, fmt.Errorf("backup version %s is incompatible with %s", m.GasaCLI, version)
	}
	if err = os.MkdirAll(filepath.Join(home, "backups"), 0700); err != nil {
		return nil, err
	}
	pre := filepath.Join(home, "backups", fmt.Sprintf("pre-restore-%s.zip", time.Now().UTC().Format("20060102T150405.000000000Z")))
	if err = Create(ctx, home, pre, Options{Version: version}); err != nil {
		return nil, fmt.Errorf("create pre-restore snapshot: %w", err)
	}
	preview := Preview(home, m, files)
	previous := make(map[string][]byte, len(files))
	missing := make(map[string]bool, len(files))
	for name := range files {
		target := filepath.Join(home, filepath.FromSlash(name))
		data, readErr := os.ReadFile(target)
		if errors.Is(readErr, os.ErrNotExist) {
			missing[name] = true
		} else if readErr != nil {
			return preview, readErr
		} else {
			previous[name] = data
		}
	}
	for name, data := range files {
		if err = atomicWriteWithin(home, filepath.Join(home, filepath.FromSlash(name)), data); err != nil {
			var rollbackErr error
			for restored := range files {
				target := filepath.Join(home, filepath.FromSlash(restored))
				if missing[restored] {
					if removeErr := os.Remove(target); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
						rollbackErr = errors.Join(rollbackErr, removeErr)
					}
				} else if old, ok := previous[restored]; ok {
					if restoreErr := atomicWriteWithin(home, target, old); restoreErr != nil {
						rollbackErr = errors.Join(rollbackErr, restoreErr)
					}
				}
			}
			return preview, errors.Join(fmt.Errorf("restore %s: %w (pre-restore snapshot: %s)", name, err, pre), rollbackErr)
		}
	}
	return preview, nil
}

func snapshotDB(ctx context.Context, path string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	tmpDir, err := os.MkdirTemp("", "gasacli-db-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	dest := filepath.Join(tmpDir, "snapshot.sqlite")
	if _, err = db.ExecContext(ctx, "VACUUM INTO ?", dest); err != nil {
		return nil, fmt.Errorf("snapshot database: %w", err)
	}
	return os.ReadFile(dest)
}
func walkScope(home, scope string, out map[string][]byte) error {
	root := filepath.Join(home, scope)
	return filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(home, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) || !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular backup path %s", rel)
		}
		if info.IsDir() {
			return nil
		}
		if secretName(filepath.Base(path)) {
			return nil
		}
		if info.Size() > maxFileBytes {
			return fmt.Errorf("backup file exceeds size limit: %s", rel)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
}
func addRegularFile(home, name string, out map[string][]byte) {
	p := filepath.Join(home, name)
	i, e := os.Lstat(p)
	if e == nil && i.Mode().IsRegular() && i.Size() <= maxFileBytes {
		if b, e := os.ReadFile(p); e == nil {
			out[name] = b
		}
	}
}
func validateScopes(s []string) error {
	allowed := map[string]bool{"config": true, "prompts": true, "skills": true, "agents": true, "segundo-cerebro": true, "database": true}
	for _, x := range s {
		if !allowed[x] {
			return fmt.Errorf("unsupported backup scope %q", x)
		}
	}
	return nil
}
func secretName(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "token") || strings.Contains(s, "secret") || strings.Contains(s, "credential") || strings.Contains(s, "password") || strings.Contains(s, "session")
}
func safeName(s string) bool {
	return s != "" && !path.IsAbs(s) && !strings.ContainsAny(s, "\\:") && path.Clean(s) == s && !strings.HasPrefix(s, "../") && s != ".."
}
func verify(m Manifest, files map[string][]byte) error {
	if len(m.Files) != len(files) {
		return fmt.Errorf("backup manifest file list does not match archive")
	}
	seen := map[string]bool{}
	for _, e := range m.Files {
		if seen[e.Path] {
			return fmt.Errorf("duplicate backup manifest path")
		}
		seen[e.Path] = true
		if !safeName(e.Path) || !allowedPath(e.Path) {
			return fmt.Errorf("unsafe backup manifest path: %s", e.Path)
		}
		b, ok := files[e.Path]
		if !ok || int64(len(b)) != e.Size {
			return fmt.Errorf("backup file size mismatch: %s", e.Path)
		}
		h := sha256.Sum256(b)
		if hex.EncodeToString(h[:]) != e.SHA256 {
			return fmt.Errorf("backup hash mismatch: %s", e.Path)
		}
	}
	return nil
}
func allowedPath(name string) bool {
	if secretName(filepath.Base(name)) {
		return false
	}
	if name == "config.yaml" || name == "state.json" || name == "permissions.yaml" || name == "gasacli.db" || name == "segundo-cerebro/index.json" {
		return true
	}
	for _, root := range []string{"prompts/", "skills/", "agents/", "segundo-cerebro/"} {
		if strings.HasPrefix(name, root) {
			return true
		}
	}
	return false
}
func atomicWrite(path string, data []byte) error {
	return atomicWriteWithin(filepath.Dir(path), path, data)
}

func atomicWriteWithin(root, path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if err := ensureNoLinks(root, dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := ensureNoLinks(root, dir); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) || !info.Mode().IsRegular()) {
		return fmt.Errorf("refusing to replace non-regular restore target")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.CreateTemp(dir, ".restore-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func ensureNoLinks(root, dir string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, dirAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("restore path escapes allowed root")
	}
	for p := dirAbs; ; p = filepath.Dir(p) {
		i, e := os.Lstat(p)
		if e == nil && (i.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(i)) {
			return fmt.Errorf("refusing symlink in restore path")
		}
		if p == rootAbs {
			break
		}
	}
	return nil
}
func derive(pass string, salt []byte) []byte {
	h := sha256.New
	u := hmacSum(h, []byte(pass), salt, 120000)
	return u
}
func hmacSum(newHash func() hash.Hash, key, salt []byte, iter int) []byte {
	mac := hmac.New(newHash, key)
	mac.Write(append(append([]byte(nil), salt...), 0, 0, 0, 1))
	u := mac.Sum(nil)
	out := append([]byte(nil), u...)
	for i := 1; i < iter; i++ {
		mac = hmac.New(newHash, key)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range out {
			out[j] ^= u[j]
		}
	}
	return out
}
