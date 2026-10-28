package orchestration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/GasaTec/gasacli/internal/lifecycle"
)

const maxReviewSnapshotBytes = int64(64 << 20)

func snapshot(project string) (string, error) {
	root, err := filepath.Abs(project)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || lifecycle.IsReparsePoint(info) {
		return "", errors.New("review snapshot requires a regular project directory")
	}
	hash := sha256.New()
	var total int64
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if total+info.Size() > maxReviewSnapshotBytes {
			return errors.New("project exceeds the review snapshot size limit")
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		_, writeErr := io.WriteString(hash, strings.ReplaceAll(relative, "\\", "/")+"\x00")
		if writeErr == nil {
			var count int64
			count, writeErr = io.Copy(hash, io.LimitReader(file, maxReviewSnapshotBytes-total+1))
			total += count
		}
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		if total > maxReviewSnapshotBytes {
			return errors.New("project exceeds the review snapshot size limit")
		}
		_, err = io.WriteString(hash, "\x00")
		return err
	})
	if err != nil {
		return "", fmt.Errorf("hash reviewed project snapshot: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
