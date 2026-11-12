package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vela/internal/domain"
)

func parseWorktrees(value string) ([]domain.Worktree, error) {
	var entries []domain.Worktree
	var entry *domain.Worktree
	for _, field := range strings.Split(value, "\x00") {
		if field == "" {
			entry = nil
			continue
		}
		key, text, _ := strings.Cut(field, " ")
		if key == "worktree" {
			if entry != nil || text == "" {
				return nil, errors.New("invalid worktree record")
			}
			entries = append(entries, domain.Worktree{Path: text})
			entry = &entries[len(entries)-1]
			continue
		}
		if entry == nil {
			return nil, errors.New("worktree record has no path")
		}
		switch key {
		case "HEAD":
			entry.Head = text
		case "branch":
			entry.Branch = text
		case "bare":
			entry.Bare = true
		case "detached":
			entry.Detached = true
		case "locked":
			entry.Locked = true
		case "prunable":
			entry.Prunable = true
		}
	}
	return entries, nil
}

func (g *Executor) readWorktrees(ctx context.Context, root string) ([]domain.Worktree, string, error) {
	value, err := g.read(ctx, root, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, "", err
	}
	entries, err := parseWorktrees(value)
	return entries, fmt.Sprintf("%x", sha256.Sum256([]byte(value))), err
}

func (g *Executor) worktree(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) error {
	entries, version, err := g.readWorktrees(ctx, repo.Root)
	if err != nil {
		return err
	}
	if repo.WorktreesVersion == "" || version != repo.WorktreesVersion {
		return ErrStaleIndex
	}
	if !filepath.IsAbs(op.Path) || strings.ContainsRune(op.Path, 0) {
		return errors.New("enter an absolute worktree directory")
	}
	path := filepath.Clean(op.Path)
	if op.Kind == domain.RemoveWorktree {
		i := slices.IndexFunc(entries, func(w domain.Worktree) bool { return samePath(w.Path, path) })
		if i <= 0 || samePath(path, repo.Root) {
			return errors.New("only another linked worktree can be removed")
		}
		entry := entries[i]
		if entry.Bare || entry.Locked || entry.Prunable {
			return errors.New("locked, bare or missing worktrees cannot be removed here")
		}
		// Git's non-force removal still deletes ignored files, and status can
		// hide assume-unchanged edits. Refuse both before removing any directory.
		untracked, err := g.read(ctx, path, "ls-files", "--others", "-z")
		if err != nil {
			return err
		}
		if untracked != "" {
			return errors.New("move untracked and ignored files out of the worktree before removing it")
		}
		if err := g.rejectHiddenIndexFlags(ctx, path); err != nil {
			return err
		}
		// Never force removal. Git validates tracked and untracked changes itself.
		return g.runOperation(ctx, repo.Root, "worktree", "remove", "--", path)
	}
	if _, err := os.Lstat(path); err == nil {
		return errors.New("choose a new directory that does not exist")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	for {
		if _, err := os.Stat(parent); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return errors.New("worktree parent directory does not exist")
		}
		parent = next
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	suffix, err := filepath.Rel(parent, path)
	if err != nil {
		return err
	}
	resolved = filepath.Join(resolved, suffix)
	for _, entry := range entries {
		root, err := filepath.EvalSymlinks(entry.Path)
		if err != nil {
			root = entry.Path
		}
		if pathWithin(root, resolved) || pathWithin(resolved, root) {
			return errors.New("worktrees must use separate non-nested directories")
		}
	}
	source, err := g.resolveWorktreeSource(ctx, repo, op)
	if err != nil {
		return err
	}
	args := []string{"-c", "submodule.recurse=false", "worktree", "add"}
	switch op.Mode {
	case "Detached":
		args = append(args, "--detach")
	case "Branch":
		if !strings.HasPrefix(op.Source, "refs/heads/") {
			return errors.New("select an existing local branch")
		}
		if err := g.ensureBranchNotCheckedOut(ctx, repo.Root, op.Source); err != nil {
			return err
		}
		source = strings.TrimPrefix(op.Source, "refs/heads/")
	case "New Branch":
		if err := g.checkBranchName(ctx, repo.Root, op.Branch); err != nil {
			return err
		}
		existing, err := g.run(ctx, repo.Root, "show-ref", "--verify", "--quiet", "refs/heads/"+op.Branch)
		if err == nil {
			return errors.New("this local branch already exists; choose Branch mode to use it")
		}
		if ctx.Err() != nil || existing.exitCode != 1 {
			return err
		}
		args = append(args, "--no-track", "-b", op.Branch)
	default:
		return errors.New("choose Branch, New Branch or Detached worktree")
	}
	return g.runOperation(ctx, repo.Root, append(args, "--", path, source)...)
}

// A candidate may be outside the loaded history, but is authorized only by
// the exact live Bisect step captured by the confirmation.
func (g *Executor) resolveWorktreeSource(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) (string, error) {
	if op.BisectVersion == "" {
		return g.resolveSource(ctx, repo, op.Source)
	}
	if op.Mode != "Detached" {
		return "", errors.New("a Bisect candidate worktree must use Detached mode")
	}
	if !validObjectID(op.Source) || op.BisectVersion != repo.Bisect.Version || op.Source != repo.Bisect.Candidate {
		return "", ErrStaleIndex
	}
	dir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return "", err
	}
	state, err := g.readBisect(ctx, repo.Root, dir)
	if err != nil {
		return "", err
	}
	if !state.Active() || !state.NoCheckout || !state.Supported || state.Ambiguous || state.Version != op.BisectVersion || state.Candidate != op.Source {
		return "", ErrStaleIndex
	}
	value, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "--end-of-options", op.Source+"^{commit}")
	return strings.TrimSpace(value), err
}

func samePath(a, b string) bool {
	if ai, err := os.Stat(a); err == nil {
		if bi, err := os.Stat(b); err == nil && os.SameFile(ai, bi) {
			return true
		}
	}
	return rebasePathKey(filepath.Clean(a)) == rebasePathKey(filepath.Clean(b))
}
func pathWithin(root, path string) bool {
	for parent := path; ; parent = filepath.Dir(parent) {
		if samePath(root, parent) {
			return true
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
