package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"slices"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) readStashes(ctx context.Context, root string) ([]domain.StashEntry, string, error) {
	value, err := g.read(ctx, root, "stash", "list", "-z", "--format=%H%x00%gd%x00%gs")
	if err != nil {
		return nil, "", err
	}
	fields := nulFields(value)
	if len(fields)%3 != 0 {
		return nil, "", errors.New("invalid stash list record")
	}
	var entries []domain.StashEntry
	for i := 0; i < len(fields); i += 3 {
		entries = append(entries, domain.StashEntry{OID: fields[i], Selector: fields[i+1], Subject: fields[i+2]})
	}
	return entries, fmt.Sprintf("%x", sha256.Sum256([]byte(value))), nil
}

func (g *Executor) stash(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	if err := rejectSubmoduleIndex(index); err != nil {
		return err
	}
	entries, version, err := g.readStashes(ctx, repo.Root)
	if err != nil {
		return err
	}
	if repo.StashVersion == "" || version != repo.StashVersion {
		return ErrStaleIndex
	}
	if op.Kind == domain.DropStash {
		if !slices.ContainsFunc(entries, func(e domain.StashEntry) bool { return e.OID == op.Source && e.Selector == op.Target }) {
			return ErrStaleIndex
		}
		return g.runOperation(ctx, repo.Root, "stash", "drop", "--quiet", op.Target)
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	if err := g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
		return err
	}
	if repo.Head == "" {
		return errors.New("create a commit before using Stash")
	}
	if op.Kind == domain.SaveStash {
		if strings.ContainsRune(op.Message, 0) {
			return errors.New("stash message contains a NUL byte")
		}
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, repo.Head); err != nil {
			return err
		}
		if len(repo.Staged) == 0 && !slices.ContainsFunc(repo.Working, func(f domain.File) bool { return f.Status != "?" || op.IncludeUntracked }) {
			return errors.New("no changes selected for Stash")
		}
		args := []string{"-c", "submodule.recurse=false", "stash", "push", "--message=" + op.Message}
		if op.IncludeUntracked {
			args = append(args, "--include-untracked")
		}
		return g.runOperation(ctx, repo.Root, append(args, "--")...)
	}
	if !slices.ContainsFunc(entries, func(e domain.StashEntry) bool { return e.OID == op.Source && e.Selector == op.Target }) {
		return ErrStaleIndex
	}
	if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
		return err
	}
	if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, op.Source); err != nil {
		return err
	}
	// The optional third parent contains untracked files. Guard those paths too.
	parents, err := g.read(ctx, repo.Root, "show", "-s", "--format=%P", op.Source, "--")
	if err != nil {
		return err
	}
	if p := strings.Fields(parents); len(p) > 2 {
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, p[2]); err != nil {
			return err
		}
	}
	args := []string{"-c", "submodule.recurse=false", "stash", "apply"}
	if op.RestoreIndex {
		args = append(args, "--index")
	}
	return g.runOperation(ctx, repo.Root, append(args, op.Source)...)
}
