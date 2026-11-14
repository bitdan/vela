package adapters

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"vela/internal/domain"
)

type PatchFiles struct{}

// SaveNewPatch publishes an owned temporary file with an atomic no-overwrite
// link. It never replaces an existing destination or writes Git metadata.
func (PatchFiles) SaveNewPatch(ctx context.Context, repo domain.Repository, target, text string) (retErr error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(target) || len(text) == 0 || len(text) > 16<<20 || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || !strings.HasPrefix(text, "diff --git ") {
		return errors.New("choose a new absolute destination for a valid Git patch up to 16 MiB")
	}
	ext := strings.ToLower(filepath.Ext(target))
	if ext != ".patch" && ext != ".diff" {
		return errors.New("use a new .patch or .diff file")
	}
	rawTarget := filepath.Clean(target)
	parent, err := filepath.EvalSymlinks(filepath.Dir(rawTarget))
	if err != nil {
		return fmt.Errorf("patch destination parent: %w", err)
	}
	target = filepath.Join(parent, filepath.Base(target))
	for _, candidate := range []string{rawTarget, target} {
		for _, part := range strings.Split(filepath.ToSlash(candidate), "/") {
			if strings.EqualFold(strings.TrimRight(part, " ."), ".git") {
				return errors.New("patch destination must be outside Git metadata")
			}
		}
	}
	var metadataInfo []os.FileInfo
	for _, metadata := range []string{repo.GitDir, repo.CommonDir} {
		if metadata == "" {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(metadata); err == nil {
			metadata = resolved
		}
		if rel, err := filepath.Rel(metadata, target); err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return errors.New("patch destination must be outside repository metadata")
		}
		if info, err := os.Stat(metadata); err == nil {
			metadataInfo = append(metadataInfo, info)
		}
	}
	if len(metadataInfo) > 0 {
		for ancestor := parent; ; ancestor = filepath.Dir(ancestor) {
			if err := ctx.Err(); err != nil {
				return err
			}
			info, err := os.Stat(ancestor)
			if err != nil {
				return fmt.Errorf("validate patch directory: %w", err)
			}
			for _, metadata := range metadataInfo {
				if os.SameFile(info, metadata) {
					return errors.New("patch destination aliases repository metadata")
				}
			}
			if filepath.Dir(ancestor) == ancestor {
				break
			}
		}
	}
	dir, err := os.OpenRoot(parent)
	if err != nil {
		return err
	}
	defer dir.Close()
	name := filepath.Base(target)
	temp := ".vela-patch-" + rand.Text()
	file, err := dir.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if err := dir.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
			detail := "temporary patch cleanup failed"
			if published {
				detail = "patch created; " + detail
			}
			retErr = errors.Join(retErr, fmt.Errorf("%s: %w", detail, err))
		}
	}()
	_, writeErr := file.WriteString(text)
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write patch temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := dir.Link(temp, name); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("destination already exists; choose a new patch filename")
		}
		return fmt.Errorf("create patch without overwriting: %w", err)
	}
	published = true
	return nil
}
