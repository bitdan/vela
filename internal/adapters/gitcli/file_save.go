package gitcli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"vela/internal/domain"
)

var ErrStaleFile = errors.New("file changed on disk since it was opened; edits are kept in the editor")

func projectFileVersion(root, path, text string) (string, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	info, err := dir.Lstat(filepath.FromSlash(path))
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", nil
	}
	return diskFileVersion(info, text), nil
}

func diskFileVersion(info os.FileInfo, text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s:%d:%d\x00%s", info.Mode(), info.Size(), info.ModTime().UnixNano(), text))))
}

// SaveFile replaces only a version-matched regular project file. It never
// stages it, follows symlinks, creates a missing user file or overwrites an
// externally changed version. Index/HEAD and other Git writes share this lock.
func (g *Executor) SaveFile(ctx context.Context, repo domain.Repository, file domain.File, text string) (domain.File, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	return g.saveFile(ctx, repo, file, text)
}

// saveFile runs with the shared write lock held by its caller.
func (g *Executor) saveFile(ctx context.Context, repo domain.Repository, file domain.File, text string) (domain.File, error) {
	if err := ctx.Err(); err != nil {
		return file, err
	}
	if !validGitPath(file.Path) || file.DiskVersion == "" {
		return file, errors.New("open an editable regular project file before saving")
	}
	if _, err := previewText(text); err != nil {
		return file, err
	}
	if _, index, err := g.verifyCheckout(ctx, repo); err != nil {
		return file, err
	} else if err = g.checkCommitState(ctx, repo.Root, index); err != nil {
		return file, err
	}
	actual, err := g.ReadFile(ctx, repo.Root, "project", domain.File{Path: file.Path})
	if err != nil {
		return file, err
	}
	if actual.DiskVersion != file.DiskVersion {
		return file, ErrStaleFile
	}
	dir, err := os.OpenRoot(repo.Root)
	if err != nil {
		return file, err
	}
	defer dir.Close()
	name := filepath.FromSlash(file.Path)
	info, err := dir.Lstat(name)
	if err != nil {
		return file, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0200 == 0 {
		return file, errors.New("the project file is not a writable regular file")
	}
	temp := filepath.Join(filepath.Dir(name), ".vela-save-"+rand.Text())
	out, err := dir.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return file, err
	}
	defer dir.Remove(temp)
	_, writeErr := out.WriteString(text)
	if writeErr == nil {
		writeErr = out.Sync()
	}
	if writeErr == nil {
		writeErr = out.Chmod(info.Mode().Perm())
	}
	if err := errors.Join(writeErr, out.Close()); err != nil {
		return file, fmt.Errorf("write temporary file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return file, err
	}
	latest, err := g.ReadFile(ctx, repo.Root, "project", domain.File{Path: file.Path})
	if err != nil {
		return file, err
	}
	if latest.DiskVersion != file.DiskVersion {
		return file, ErrStaleFile
	}
	if err := ctx.Err(); err != nil {
		return file, err
	}
	if err := dir.Rename(temp, name); err != nil {
		return file, fmt.Errorf("replace project file: %w", err)
	}
	file.New = text
	// Read the new stat without a canceled request: replacement already succeeded.
	info, err = dir.Lstat(name)
	if err != nil {
		return file, fmt.Errorf("file saved but could not inspect new version: %w", err)
	}
	file.DiskVersion = diskFileVersion(info, text)
	return file, nil
}
