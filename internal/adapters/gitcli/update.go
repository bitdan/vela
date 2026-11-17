package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"vela/internal/domain"
)

func (g *Executor) UpdateRepository(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) (domain.UpdateResult, error) {
	ctx, cancel := context.WithTimeout(withNetworkPolicy(ctx, op.Network), 5*time.Minute)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	return g.updateTrackedBranch(ctx, repo, op)
}

// updateTrackedBranch holds the Executor write lock throughout fetch, backup,
// integration and optional restore. External processes still require rechecks.
func (g *Executor) updateTrackedBranch(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) (result domain.UpdateResult, err error) {
	result.Method = op.UpdateMethod
	if repo.Root == "" || repo.IndexVersion == "" || repo.GitDir == "" || repo.CommonDir == "" {
		return result, errors.New("refresh the repository identity before Update")
	}
	if err = g.checkBranchName(ctx, repo.Root, op.Branch); err != nil {
		return result, err
	}
	if err = g.checkBranchName(ctx, repo.Root, op.Target); err != nil {
		return result, err
	}
	if op.Kind != domain.UpdateTrackedBranch || repo.Head == "" {
		return result, errors.New("select a committed local branch to update")
	}
	if op.UpdateMethod != "Fast-forward" && op.UpdateMethod != "Merge" && op.UpdateMethod != "Rebase" {
		return result, errors.New("select a supported Update method")
	}
	current, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return result, err
	}
	if err = g.verifyUpdateMetadata(ctx, repo); err != nil {
		return result, err
	}
	onCurrent := op.Branch == current.CurrentBranch
	if !onCurrent && op.UpdateMethod != "Fast-forward" {
		return result, errors.New("non-current branches support fast-forward Update only")
	}
	localRef := "refs/heads/" + op.Branch
	if err = g.verifyUpstream(ctx, repo, localRef, op); err != nil {
		return result, err
	}
	localTip, err := g.resolveSource(ctx, repo, localRef)
	if err != nil {
		return result, err
	}
	if err = g.verifyRemote(ctx, repo, op.Remote, false); err != nil {
		return result, err
	}
	if onCurrent {
		if err = g.checkCommitState(ctx, repo.Root, index); err != nil {
			return result, err
		}
		if err = g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
			return result, err
		}
		if err = rejectSubmoduleIndex(index); err != nil {
			return result, err
		}
		if err = g.verifyWorktree(ctx, repo); err != nil {
			return result, err
		}
	} else if err = g.ensureBranchNotCheckedOut(ctx, repo.Root, localRef); err != nil {
		return result, err
	}
	dirty := onCurrent && updateHasChanges(repo)
	if dirty && (!op.ConfirmUpdateCleanup || (op.UpdateCleanup != "Stash" && op.UpdateCleanup != "Shelve")) {
		return result, errors.New("confirm Stash or Shelve cleanup before updating tracked changes")
	}
	var paths []string
	if dirty {
		paths, err = updateChangedPaths(repo)
		if err != nil {
			return result, err
		}
		if err = g.validateUpdateBackup(ctx, repo, index, paths); err != nil {
			return result, err
		}
	}
	if err = g.fetchBranches(ctx, repo.Root, op.Remote, op.Target, op.Network); err != nil {
		return result, err
	}
	result.Fetched = true
	// Report fetch success separately from every subsequent stage.
	defer func() {
		if err != nil {
			err = fmt.Errorf("Fetch completed; Update %s stopped: %w", op.Branch, err)
		}
	}()
	if _, _, err = g.verifyCheckout(ctx, repo); err != nil {
		return result, err
	}
	if err = g.verifyUpstream(ctx, repo, localRef, op); err != nil {
		return result, err
	}
	if _, err = g.resolveSource(ctx, repo, localRef); err != nil {
		return result, err
	}
	tip, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "refs/remotes/"+op.Remote+"/"+op.Target+"^{commit}")
	if err != nil {
		return result, err
	}
	result.TargetOID = strings.TrimSpace(tip)
	if !validObjectID(result.TargetOID) {
		return result, errors.New("invalid fetched target")
	}
	if !onCurrent {
		err = g.updateOtherBranch(ctx, repo, op, localRef, localTip, result.TargetOID)
		result.Integrated = err == nil
		return result, err
	}
	if err = g.verifyWorktree(ctx, repo); err != nil {
		return result, err
	}
	if err = g.protectWorktreePaths(ctx, repo.Root, repo.Head, result.TargetOID); err != nil {
		return result, err
	}
	if err = g.validateHistoricalCheckoutPaths(ctx, repo.Root, result.TargetOID); err != nil {
		return result, err
	}
	if op.UpdateMethod == "Rebase" {
		merges, readErr := g.read(ctx, repo.Root, "rev-list", "--merges", "--max-count=1", result.TargetOID+".."+repo.Head)
		if readErr != nil {
			return result, readErr
		}
		if merges != "" {
			return result, errors.New("local commits include merges; use Merge or rebase with Git")
		}
	}
	active := repo
	if dirty {
		if _, _, err = g.verifyCheckout(ctx, repo); err != nil {
			return result, err
		}
		if err = g.verifyUpstream(ctx, repo, localRef, op); err != nil {
			return result, err
		}
		if err = g.validateUpdateBackup(ctx, repo, index, paths); err != nil {
			return result, err
		}
		result.BackupKind, result.BackupName = op.UpdateCleanup, "Vela Update "+time.Now().UTC().Format("20060102-150405")+" "+rand.Text()
		save := domain.RepositoryOperation{Message: result.BackupName, Paths: paths}
		if op.UpdateCleanup == "Stash" {
			save.Kind = domain.SaveStash
			err = g.stash(ctx, repo, index, save)
		} else {
			save.Kind = domain.SaveShelf
			err = g.shelfOperation(ctx, repo, index, save)
		}
		// Publication may have succeeded even if cleanup or cancellation failed.
		inspectCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		findErr := g.findUpdateBackup(inspectCtx, repo, &result)
		stop()
		if err != nil {
			return result, errors.Join(err, findErr)
		}
		if findErr != nil {
			return result, findErr
		}
		active, err = g.ReadRepository(ctx, repo.Root)
		if err != nil {
			return result, err
		}
		if active.Head != repo.Head || active.CurrentBranch != repo.CurrentBranch {
			return result, ErrStaleIndex
		}
		if active.GitDir != repo.GitDir || active.CommonDir != repo.CommonDir {
			return result, ErrStaleIndex
		}
		if err = g.requireCleanTracked(ctx, repo.Root); err != nil {
			return result, err
		}
		result.Cleaned = true
	}
	_, activeIndex, err := g.verifyCheckout(ctx, active)
	if err != nil {
		return result, err
	}
	if err = g.checkCommitState(ctx, repo.Root, activeIndex); err != nil {
		return result, err
	}
	if err = g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
		return result, err
	}
	if err = g.requireCleanTracked(ctx, repo.Root); err != nil {
		return result, err
	}
	if err = g.verifyWorktree(ctx, active); err != nil {
		return result, err
	}
	if err = g.verifyUpstream(ctx, active, localRef, op); err != nil {
		return result, err
	}
	if err = g.protectWorktreePaths(ctx, repo.Root, active.Head, result.TargetOID); err != nil {
		return result, err
	}
	if _, _, err = g.verifyCheckout(ctx, active); err != nil {
		return result, err
	}
	if err = g.verifyWorktree(ctx, active); err != nil {
		return result, err
	}
	if err = g.verifyUpdateMetadata(ctx, repo); err != nil {
		return result, err
	}
	switch op.UpdateMethod {
	case "Rebase":
		err = g.runRebase(ctx, repo.Root, "--merge", "--no-autostash", "--no-update-refs", "--no-autosquash", "--no-rebase-merges", "--no-fork-point", "--no-rerere-autoupdate", "--empty=stop", result.TargetOID)
	case "Merge":
		err = g.runOperation(ctx, repo.Root, "-c", "submodule.recurse=false", "merge", "--ff", "--no-edit", "--no-squash", "--no-autostash", "--no-overwrite-ignore", "--no-rerere-autoupdate", "-m", "Merge "+op.Remote+"/"+op.Target+" into "+op.Branch, result.TargetOID)
	case "Fast-forward":
		err = g.runOperation(ctx, repo.Root, "-c", "submodule.recurse=false", "merge", "--ff-only", "--no-squash", "--no-autostash", "--no-overwrite-ignore", result.TargetOID)
	}
	if err != nil {
		return result, err
	}
	result.Integrated = true
	if !dirty {
		return result, nil
	}
	active, err = g.ReadRepository(ctx, repo.Root)
	if err != nil {
		return result, err
	}
	if active.CurrentBranch != repo.CurrentBranch || active.ResolvingConflicts() {
		return result, errors.New("checkout or integration state changed; restore the retained backup manually")
	}
	if active.GitDir != repo.GitDir || active.CommonDir != repo.CommonDir {
		return result, ErrStaleIndex
	}
	_, activeIndex, err = g.verifyCheckout(ctx, active)
	if err != nil {
		return result, err
	}
	if err = g.verifyUpdateMetadata(ctx, active); err != nil {
		return result, err
	}
	restore := domain.RepositoryOperation{Source: result.BackupOID, Target: result.BackupID, RestoreIndex: true}
	if result.BackupKind == "Stash" {
		restore.Kind = domain.ApplyStash
		err = g.stash(ctx, active, activeIndex, restore)
	} else {
		restore.Kind = domain.ApplyShelf
		err = g.shelfOperation(ctx, active, activeIndex, restore)
	}
	result.Restored = err == nil
	if err != nil {
		err = fmt.Errorf("branch updated; backup retained and restoration may be partial: %w", err)
	}
	return result, err
}

func updateHasChanges(repo domain.Repository) bool {
	return len(repo.Staged) > 0 || slices.ContainsFunc(repo.Working, func(f domain.File) bool { return f.Status != "?" })
}

func updateChangedPaths(repo domain.Repository) ([]string, error) {
	var paths []string
	for _, file := range slices.Concat(repo.Staged, repo.Working) {
		if file.Status == "?" {
			continue
		}
		paths = append(paths, file.Path)
		if file.OldPath != "" {
			paths = append(paths, file.OldPath)
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	paths, err := historyPaths(paths)
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if err := projectPath(path, false); err != nil {
			return nil, err
		}
	}
	return paths, nil
}

func (g *Executor) updateOtherBranch(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, ref, original, target string) error {
	if err := g.verifyUpstream(ctx, repo, ref, op); err != nil {
		return err
	}
	if _, err := g.resolveSource(ctx, repo, ref); err != nil {
		return err
	}
	if err := g.ensureBranchNotCheckedOut(ctx, repo.Root, ref); err != nil {
		return err
	}
	ancestor, err := g.isAncestor(ctx, repo.Root, target, original)
	if err != nil {
		return err
	}
	if ancestor {
		return nil
	}
	ancestor, err = g.isAncestor(ctx, repo.Root, original, target)
	if err != nil {
		return err
	}
	if !ancestor {
		return errors.New("the local and remote branches have diverged; check out this branch to Merge/Rebase")
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	if err := g.ensureBranchNotCheckedOut(ctx, repo.Root, ref); err != nil {
		return err
	}
	if err := g.verifyUpdateMetadata(ctx, repo); err != nil {
		return err
	}
	return g.runOperation(ctx, repo.Root, "update-ref", "-m", "vela: update tracked branch", ref, target, original)
}

func (g *Executor) verifyUpdateMetadata(ctx context.Context, repo domain.Repository) error {
	gitDir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return err
	}
	common, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	if gitDir != repo.GitDir || filepath.Clean(strings.TrimSuffix(common, "\n")) != repo.CommonDir {
		return ErrStaleIndex
	}
	return nil
}

func (g *Executor) validateUpdateBackup(ctx context.Context, repo domain.Repository, index string, paths []string) error {
	if err := rejectHistoryIndexAliases(paths, index); err != nil {
		return err
	}
	files := historyFiles(paths)
	if err := g.validateShelfPaths(ctx, repo, files, index); err != nil {
		return err
	}
	return g.validateShelfTreeModes(ctx, repo.Root, repo.Head, files)
}

func (g *Executor) findUpdateBackup(ctx context.Context, repo domain.Repository, result *domain.UpdateResult) error {
	if result.BackupKind == "Stash" {
		entries, _, err := g.readStashes(ctx, repo.Root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Subject, result.BackupName) {
				result.BackupOID, result.BackupID = entry.OID, entry.Selector
				return nil
			}
		}
	} else {
		entries, err := g.readShelves(ctx, repo)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if !entry.Deleted && entry.Title == result.BackupName {
				result.BackupOID, result.BackupID = entry.OID, entry.ID
				return nil
			}
		}
	}
	return errors.New("backup identity could not be confirmed; inspect Stash/Shelf before retrying")
}
