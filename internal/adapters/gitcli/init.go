package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrExistingRepository = errors.New("directory is already inside a Git repository")

// Initialize creates a repository in an existing directory without changing
// its files. The same use case serves the GUI and command-line entry points.
func (g *Executor) Initialize(ctx context.Context, directory, branch string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	g.initialize.Lock()
	defer g.initialize.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(directory) == "" {
		return "", errors.New("choose a directory")
	}
	dir, err := filepath.Abs(directory)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve directory: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("inspect directory: %w", err)
	}
	if !info.IsDir() {
		return "", errors.New("target is not a directory")
	}
	if branch == "" || strings.HasPrefix(branch, "-") {
		return "", errors.New("enter a valid initial branch name")
	}
	if out, err := g.run(ctx, dir, "check-ref-format", "refs/heads/"+branch); err != nil {
		if out.exitCode == 1 && ctx.Err() == nil {
			return "", errors.New("enter a valid initial branch name")
		}
		return "", err
	}
	for ancestor := dir; ; ancestor = filepath.Dir(ancestor) {
		if _, err := os.Lstat(filepath.Join(ancestor, ".git")); err == nil {
			return "", ErrExistingRepository
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("check existing repository: %w", err)
		}
		if filepath.Dir(ancestor) == ancestor {
			break
		}
	}
	probe, err := g.run(ctx, dir, "rev-parse", "--git-dir")
	if err == nil {
		return "", ErrExistingRepository
	}
	if ctx.Err() != nil || probe.exitCode != 128 {
		return "", err
	}
	out, err := g.run(ctx, dir, "init", "--initial-branch="+branch, "--template=", "--", ".")
	if err != nil {
		return "", fmt.Errorf("initialize %q: %w %s", dir, err, strings.TrimSpace(out.stderr))
	}
	return dir, nil
}
