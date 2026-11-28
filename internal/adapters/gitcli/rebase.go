package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vela/internal/domain"
)

func rebaseAt(gitDir string) (domain.RebaseState, error) {
	var state domain.RebaseState
	dir := filepath.Join(gitDir, "rebase-merge")
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
		return state, nil
	} else if err != nil {
		return state, err
	}
	for _, field := range []struct {
		name     string
		dest     *string
		optional bool
	}{
		{"head-name", &state.Branch, false},
		{"orig-head", &state.Original, false},
		{"onto", &state.Onto, false},
		{"stopped-sha", &state.Stopped, true},
		{"msgnum", &state.Step, true},
	} {
		data, err := os.ReadFile(filepath.Join(dir, field.name))
		if field.optional && errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return state, err
		}
		*field.dest = strings.TrimSpace(string(data))
	}
	if !validObjectID(state.Original) || !validObjectID(state.Onto) ||
		(state.Stopped != "" && !validObjectID(state.Stopped)) {
		return state, errors.New("invalid rebase metadata")
	}
	state.Branch = strings.TrimPrefix(state.Branch, "refs/heads/")
	return state, nil
}

func (g *Executor) verifyRebase(ctx context.Context, repo domain.Repository) error {
	if repo.CurrentBranch != "Detached HEAD" {
		return ErrStaleIndex
	}
	gitDir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return err
	}
	state, err := rebaseAt(gitDir)
	if err != nil {
		return err
	}
	if !state.Active() || state != repo.Rebase {
		return ErrStaleIndex
	}
	if err := g.checkBranchName(ctx, repo.Root, state.Branch); err != nil {
		return errors.New("this rebase has no local branch to restore")
	}
	tip, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "--end-of-options", "refs/heads/"+state.Branch)
	if err != nil {
		return err
	}
	if strings.TrimSpace(tip) != state.Original {
		return ErrStaleIndex
	}
	return nil
}

func (g *Executor) rebase(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if op.Kind == domain.StartRebase {
		if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" {
			return errors.New("check out a local branch with a commit before running this operation")
		}
		if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
			return err
		}
		if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
			return err
		}
		onto, err := g.resolveSource(ctx, repo, op.Source)
		if err != nil {
			return err
		}
		merges, err := g.read(ctx, repo.Root, "rev-list", "--merges", "--max-count=1", onto+".."+repo.Head)
		if err != nil {
			return err
		}
		if merges != "" {
			return errors.New("the commits to replay include a merge commit. Use Git to rebase while preserving merge history")
		}
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, onto); err != nil {
			return err
		}
		return g.runRebase(ctx, repo.Root, "--merge", "--no-autostash", "--no-update-refs", "--no-autosquash",
			"--no-rebase-merges", "--no-fork-point", "--no-rerere-autoupdate", "--empty=stop", onto)
	}
	if err := g.verifyRebase(ctx, repo); err != nil {
		return err
	}
	if err := g.protectWorktreePaths(ctx, repo.Root, repo.Rebase.Original, repo.Rebase.Onto); err != nil {
		return err
	}
	switch op.Kind {
	case domain.ContinueRebase:
		if len(conflictsFromIndex(index)) != 0 {
			return errors.New("resolve all conflicted files before continuing")
		}
		gitDir, err := g.gitDirectory(ctx, repo.Root)
		if err != nil {
			return err
		}
		planPath := filepath.Join(gitDir, "rebase-merge", "vela-interactive-rebase.json")
		if _, err := os.Lstat(planPath); err == nil {
			return g.runInteractiveRebaseRecovery(ctx, repo.Root, planPath)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return g.runRebase(ctx, repo.Root, "--continue")
	case domain.SkipRebase:
		return g.runRebase(ctx, repo.Root, "--skip")
	case domain.AbortRebase:
		return g.runRebase(ctx, repo.Root, "--abort")
	}
	return errors.New("unknown rebase operation")
}

func (g *Executor) runRebase(ctx context.Context, root string, args ...string) error {
	out, err := g.runInputWithEnv(ctx, root, "", []string{"GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:"},
		append([]string{"-c", "submodule.recurse=false", "rebase"}, args...)...)
	return operationError(ctx, out, err)
}

// Git can overwrite ignored paths when switching to the new base. Inspect the
// untracked paths without exclusions before the worktree can change.
func (g *Executor) protectWorktreePaths(ctx context.Context, root, original, onto string) error {
	guard, err := g.checkoutPathGuard(ctx, root)
	if err != nil || guard == nil {
		return err
	}
	for _, args := range [][]string{
		{"ls-tree", "-rz", "--name-only", original},
		{"ls-tree", "-rz", "--name-only", onto},
		{"log", "-m", "-z", "--format=", "--name-only", "--no-renames", "--full-history", onto + ".." + original, "--"},
	} {
		affected, err := g.read(ctx, root, args...)
		if err != nil {
			return err
		}
		for _, path := range nulFields(affected) {
			if err := guard(path); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *Executor) checkoutPathGuard(ctx context.Context, root string) (func(string) error, error) {
	untracked, err := g.read(ctx, root, "ls-files", "--others", "-z")
	if err != nil || untracked == "" {
		return nil, err
	}
	paths := make(map[string]bool)
	parents := make(map[string]bool)
	for _, row := range nulFields(untracked) {
		path := rebasePathKey(strings.TrimSuffix(row, "/"))
		paths[path] = true
		for parent := path; strings.Contains(parent, "/"); {
			parent = parent[:strings.LastIndexByte(parent, '/')]
			parents[parent] = true
		}
	}
	return func(path string) error {
		key := rebasePathKey(path)
		if paths[key] || parents[key] {
			return fmt.Errorf("move untracked or ignored files at %q before running this operation", path)
		}
		if err := rejectDirectoryPath(paths, key); err != nil {
			return fmt.Errorf("move the untracked path before running this operation: %w", err)
		}
		return nil
	}, nil
}

func rebasePathKey(path string) string {
	if filepath.Separator == '\\' {
		return strings.ToLower(path)
	}
	return path
}
