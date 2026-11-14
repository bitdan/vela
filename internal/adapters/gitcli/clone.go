package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vela/internal/domain"
)

// Clone requires a new directory. On failure it never recursively deletes a
// destination: Git may have left a useful partial repository or user content.
func (g *Executor) Clone(ctx context.Context, source, target string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !domain.ValidRemoteLocation(source) {
		return "", errors.New("enter a supported credential-free remote URL or absolute local source")
	}
	if !filepath.IsAbs(target) || strings.ContainsAny(target, "\x00\r\n") {
		return "", errors.New("choose a new absolute destination directory")
	}
	target = filepath.Clean(target)
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return "", err
		}
		return "", errors.New("clone destination already exists; choose a new directory")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return "", fmt.Errorf("resolve destination parent: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("destination parent is not a directory")
	}
	for dir := parent; ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return "", ErrExistingRepository
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	probe, err := g.run(ctx, parent, "rev-parse", "--git-dir")
	if err == nil {
		return "", ErrExistingRepository
	}
	if probe.exitCode != 128 || ctx.Err() != nil {
		return "", err
	}
	target = filepath.Join(parent, filepath.Base(target))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.Mkdir(target, 0755); err != nil {
		return "", fmt.Errorf("create new clone destination: %w", err)
	}
	out, err := g.run(ctx, parent, "-c", "submodule.recurse=false", "-c", "protocol.ext.allow=never", "clone", "--no-local", "--no-hardlinks", "--no-recurse-submodules", "--template=", "--origin=origin", "--", source, target)
	if err != nil {
		return target, fmt.Errorf("clone did not complete; inspect destination %q before retrying: %w", target, operationError(ctx, out, err))
	}
	return target, nil
}
