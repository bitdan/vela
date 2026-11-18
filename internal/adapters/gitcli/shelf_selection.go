package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

// projectShelf creates a stash-compatible object whose deltas contain only the
// selected source paths. Both saved index and saved worktree are overlaid onto
// the original base, preserving their distinction. No live index/ref is edited.
func (g *Executor) projectShelf(ctx context.Context, repo domain.Repository, entry domain.Shelf, files []domain.File) (domain.Shelf, error) {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	paths, err := historyPaths(paths)
	if err != nil {
		return domain.Shelf{}, err
	}
	temporary, err := os.MkdirTemp("", "vela-unshelve-")
	if err != nil {
		return domain.Shelf{}, err
	}
	defer os.RemoveAll(temporary)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(temporary, "index")}
	tree := func(tip string) (string, error) {
		if _, err := g.runInputWithEnv(ctx, repo.Root, "", env, "read-tree", entry.Base); err != nil {
			return "", err
		}
		var updates strings.Builder
		for _, path := range paths {
			fmt.Fprintf(&updates, "0 %s\t%s\x00", strings.Repeat("0", len(entry.Base)), path)
		}
		args := append([]string{"--literal-pathspecs", "ls-tree", "-r", "-z", tip, "--"}, paths...)
		out, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, 2<<20, args...)
		if err != nil {
			return "", err
		}
		for _, record := range nulFields(out.stdout) {
			header, path, ok := strings.Cut(record, "\t")
			fields := strings.Fields(header)
			if !ok || len(fields) != 3 || !slices.Contains(paths, path) || (fields[0] != "100644" && fields[0] != "100755") || fields[1] != "blob" || !validObjectID(fields[2]) {
				return "", errors.New("selected shelf contains a directory, link or invalid object")
			}
			fmt.Fprintf(&updates, "%s %s\t%s\x00", fields[0], fields[2], path)
		}
		if _, err := g.runInputWithEnv(ctx, repo.Root, updates.String(), env, "update-index", "-z", "--index-info"); err != nil {
			return "", err
		}
		out, err = g.runInputWithEnv(ctx, repo.Root, "", env, "write-tree")
		oid := strings.TrimSpace(out.stdout)
		if err == nil && !validObjectID(oid) {
			err = errors.New("invalid projected shelf tree")
		}
		return oid, err
	}
	application.ReportTaskProgress(ctx, "Preparing selected shelf index and working versions")
	indexTree, err := tree(entry.Index)
	if err != nil {
		return domain.Shelf{}, err
	}
	workTree, err := tree(entry.OID)
	if err != nil {
		return domain.Shelf{}, err
	}
	// Tree insertion can encounter file/directory conflicts. Check the complete
	// resulting delta, not just queried entries, so an index operation cannot
	// implicitly remove an unchecked ancestor or descendant.
	for _, tip := range []string{indexTree, workTree} {
		if err := g.verifyProjectedShelfPaths(ctx, repo.Root, entry.Base, tip, paths); err != nil {
			return domain.Shelf{}, err
		}
	}
	indexCommit, err := g.shelfCommit(ctx, repo.Root, indexTree, "vela-shelf-index-v1", entry.Base)
	if err != nil {
		return domain.Shelf{}, err
	}
	commit, err := g.shelfCommit(ctx, repo.Root, workTree, shelfSubject+entry.Title, entry.Base, indexCommit)
	if err != nil {
		return domain.Shelf{}, err
	}
	if !validObjectID(commit) {
		return domain.Shelf{}, errors.New("invalid projected shelf commit")
	}
	entry.Index, entry.OID, entry.Files = indexCommit, commit, slices.Clone(files)
	return entry, nil
}

func (g *Executor) applyShelf(ctx context.Context, repo domain.Repository, index string, entry domain.Shelf, op domain.RepositoryOperation) error {
	if err := g.verifyShelfRepository(ctx, repo); err != nil {
		return err
	}
	files, partial, err := application.SelectShelfFiles(entry, op.Paths, op.SelectShelfFiles)
	if err != nil {
		return err
	}
	if partial && op.RemoveApplied {
		return errors.New("partial unshelve retains the complete original shelf; disable moving the applied entry to Recently Deleted")
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	if _, err = historyPaths(paths); err != nil {
		return err
	}
	if err = g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
		return err
	}
	if err = rejectSubmoduleIndex(index); err != nil {
		return err
	}
	if err = g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	if err = g.requireCleanTracked(ctx, repo.Root); err != nil {
		return err
	}
	if err = rejectHistoryIndexAliases(paths, index); err != nil {
		return err
	}
	parents, err := g.read(ctx, repo.Root, "show", "-s", "--format=%P", entry.Index, "--")
	if err != nil {
		return err
	}
	if strings.TrimSpace(parents) != entry.Base {
		return errors.New("invalid shelf index ancestry")
	}
	if err = g.validateShelfPaths(ctx, repo, files, index); err != nil {
		return err
	}
	for _, tip := range []string{entry.Base, entry.Index, entry.OID} {
		if err = g.validateShelfTreeModes(ctx, repo.Root, tip, files); err != nil {
			return err
		}
	}
	applied := entry
	if partial {
		applied, err = g.projectShelf(ctx, repo, entry, files)
		if err != nil {
			return err
		}
	}
	if err = g.protectWorktreePaths(ctx, repo.Root, repo.Head, applied.OID); err != nil {
		return err
	}
	if _, _, err = g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	if err = g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	if err = g.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	if err = g.requireCleanTracked(ctx, repo.Root); err != nil {
		return err
	}
	ref := shelfPrefix(repo.GitDir, false) + entry.ID
	if err = g.verifyShelfRepository(ctx, repo); err != nil {
		return err
	}
	current, err := g.read(ctx, repo.Root, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(current) != entry.OID {
		return errors.New("shelf changed during preparation; refresh before unshelving")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	application.ReportTaskProgress(ctx, fmt.Sprintf("Applying %d of %d shelf paths; original entry retained until completion", len(files), len(entry.Files)))
	args := []string{"-c", "submodule.recurse=false", "stash", "apply"}
	if op.RestoreIndex {
		args = append(args, "--index")
	}
	if err = g.runOperation(ctx, repo.Root, append(args, applied.OID)...); err != nil {
		return fmt.Errorf("original shelf retained; review files for partial application or conflicts: %w", err)
	}
	if op.RemoveApplied {
		if err = g.moveShelfRef(ctx, repo, entry, true); err != nil {
			return fmt.Errorf("shelf applied but entry could not move to Recently Deleted: %w", err)
		}
	}
	return nil
}

func (g *Executor) verifyProjectedShelfPaths(ctx context.Context, root, base, tree string, paths []string) error {
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 2<<20, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--no-renames", "--no-ext-diff", "--no-textconv", base, tree, "--")
	if err != nil {
		return err
	}
	for _, path := range nulFields(out.stdout) {
		if !slices.Contains(paths, path) {
			return fmt.Errorf("selected shelf files would also change unchecked path %q; select the related file/directory paths or restore the complete entry", path)
		}
	}
	return nil
}

func (g *Executor) verifyShelfRepository(ctx context.Context, repo domain.Repository) error {
	dir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return err
	}
	common, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	if dir != repo.GitDir || filepath.Clean(strings.TrimSuffix(common, "\n")) != repo.CommonDir {
		return errors.New("repository metadata changed; reopen the shelf selection")
	}
	return nil
}
