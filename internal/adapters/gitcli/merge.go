package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) gitDirectory(ctx context.Context, root string) (string, error) {
	dir, err := g.read(ctx, root, "rev-parse", "--absolute-git-dir")
	return strings.TrimSuffix(dir, "\n"), err
}

func (g *Executor) mergeHeads(ctx context.Context, root string) ([]string, error) {
	dir, err := g.gitDirectory(ctx, root)
	if err != nil {
		return nil, err
	}
	return mergeHeadsAt(dir)
}

func mergeHeadsAt(dir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, "MERGE_HEAD"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	heads := strings.Fields(string(data))
	if len(heads) == 0 || slices.ContainsFunc(heads, func(hash string) bool { return !validObjectID(hash) }) {
		return nil, errors.New("invalid MERGE_HEAD")
	}
	return heads, nil
}

func conflictsFromIndex(index string) []domain.MergeConflict {
	var conflicts []domain.MergeConflict
	indices := make(map[string]int)
	for _, row := range nulFields(index) {
		prefix, path, _ := strings.Cut(row, "\t")
		mode, stage, _ := strings.Cut(prefix, " ")
		hash, stage, _ := strings.Cut(stage, " ")
		if stage != "1" && stage != "2" && stage != "3" {
			continue
		}
		i, found := indices[path]
		if !found {
			i = len(conflicts)
			indices[path] = i
			conflicts = append(conflicts, domain.MergeConflict{Path: path})
		}
		conflict := &conflicts[i]
		conflict.Submodule = conflict.Submodule || mode == "160000"
		switch stage {
		case "1":
			conflict.Base = hash
		case "2":
			conflict.Current = hash
		case "3":
			conflict.Incoming = hash
		}
	}
	return conflicts
}

func (g *Executor) merge(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if op.Kind == domain.StartMerge {
		if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" {
			return errors.New("check out a local branch with a commit before merging")
		}
		if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
			return err
		}
		if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
			return err
		}
		source, err := g.resolveSource(ctx, repo, op.Source)
		if err != nil {
			return err
		}
		label := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(op.Source, "refs/heads/"), "refs/remotes/"), "refs/tags/")
		return g.runOperation(ctx, repo.Root, "-c", "submodule.recurse=false", "merge", "--ff", "--no-edit",
			"--no-squash", "--no-autostash", "--no-overwrite-ignore", "--no-rerere-autoupdate",
			"-m", "Merge "+label+" into "+repo.CurrentBranch, source)
	}
	heads, err := g.mergeHeads(ctx, repo.Root)
	if err != nil {
		return err
	}
	if len(heads) == 0 || !slices.Equal(heads, repo.MergeHeads) {
		return errors.New("the merge state changed. Refresh the repository before continuing")
	}
	switch op.Kind {
	case domain.AbortMerge:
		return g.runOperation(ctx, repo.Root, "merge", "--abort")
	case domain.ContinueMerge:
		if len(conflictsFromIndex(index)) != 0 {
			return errors.New("resolve all conflicted files before continuing")
		}
		return g.runOperation(ctx, repo.Root, "-c", "core.fsmonitor=false", "commit", "--no-edit")
	}
	return errors.New("unknown merge operation")
}

func (g *Executor) verifyConflictOperation(ctx context.Context, repo domain.Repository) error {
	if repo.Sequencer.Active() {
		return g.verifySequencer(ctx, repo)
	}
	if repo.Rebase.Active() {
		return g.verifyRebase(ctx, repo)
	}
	heads, err := g.mergeHeads(ctx, repo.Root)
	if err != nil {
		return err
	}
	if len(heads) == 0 || !slices.Equal(heads, repo.MergeHeads) {
		return ErrStaleIndex
	}
	return nil
}

func (g *Executor) resolveMergeFile(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	path := op.Source
	if !validGitPath(path) || strings.ContainsRune(path, 0) {
		return errors.New("select a valid conflicted file")
	}
	conflicts := conflictsFromIndex(index)
	i := slices.IndexFunc(conflicts, func(c domain.MergeConflict) bool { return c.Path == path })
	if i < 0 || conflicts[i].Submodule {
		return errors.New("select an unresolved file outside submodules")
	}
	if err := validateIndexPaths(repo.Root, map[string]bool{path: true}, index, repo.Staged); err != nil {
		return err
	}
	if op.Kind == domain.SaveMergeResult {
		if conflicts[i] != op.Conflict {
			return ErrStaleIndex
		}
		return g.saveMergeResult(ctx, repo.Root, op)
	}
	if op.Kind != domain.MarkResolved {
		side, oid := "--ours", conflicts[i].Current
		if op.Kind == domain.AcceptIncoming {
			side, oid = "--theirs", conflicts[i].Incoming
		}
		if oid == "" {
			return g.runOperation(ctx, repo.Root, "--literal-pathspecs", "rm", "--force", "--", path)
		}
		if err := g.runOperation(ctx, repo.Root, "--literal-pathspecs", "restore", "--worktree", side, "--", path); err != nil {
			return err
		}
	}
	err := g.runOperation(ctx, repo.Root, "-c", "core.fsmonitor=false", "--literal-pathspecs", "add", "--all", "--", path)
	if err != nil && op.Kind != domain.MarkResolved {
		return fmt.Errorf("Updated the working file but could not stage it: %w", err)
	}
	return err
}

func (g *Executor) readMergeDiff(ctx context.Context, root string, file domain.File) (domain.File, error) {
	value, err := g.read(ctx, root, "ls-files", "--unmerged", "-z")
	if err != nil {
		return file, err
	}
	for _, conflict := range conflictsFromIndex(value) {
		if conflict.Path != file.Path {
			continue
		}
		if conflict.Submodule {
			return file, errors.New("submodule conflict previews are unavailable")
		}
		if conflict.Current != "" {
			if file.Old, err = g.readBlob(ctx, root, conflict.Current); err != nil {
				return file, err
			}
		}
		if conflict.Incoming != "" {
			file.New, err = g.readBlob(ctx, root, conflict.Incoming)
		}
		return file, err
	}
	return file, errors.New("the file is no longer conflicted")
}
