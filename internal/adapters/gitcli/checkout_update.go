package gitcli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"vela/internal/application"
	"vela/internal/domain"
)

// checkoutUpdate runs under the repository write lock, including the fetch.
func (g *Executor) checkoutUpdate(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" || repo.CurrentBranch == op.Branch || repo.Bisect.Active() {
		return errors.New("select a different local branch with an upstream")
	}
	if err := g.checkBranchName(ctx, repo.Root, op.Branch); err != nil {
		return err
	}
	if err := g.checkBranchName(ctx, repo.Root, op.Target); err != nil {
		return err
	}
	ref := "refs/heads/" + op.Branch
	if err := g.verifyUpdateMetadata(ctx, repo); err != nil {
		return err
	}
	if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	if err := g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
		return err
	}
	if err := rejectSubmoduleIndex(index); err != nil {
		return err
	}
	if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
		return err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	if err := g.verifyUpstream(ctx, repo, ref, op); err != nil {
		return err
	}
	if err := g.verifyRemote(ctx, repo, op.Remote, false); err != nil {
		return err
	}
	tip, err := g.resolveSource(ctx, repo, ref)
	if err != nil {
		return err
	}
	if symref, err := g.run(ctx, repo.Root, "symbolic-ref", "--quiet", ref); err == nil {
		return ErrStaleIndex
	} else if symref.exitCode != 1 || ctx.Err() != nil {
		return err
	}
	if err := g.validateHistoricalCheckoutPaths(ctx, repo.Root, tip); err != nil {
		return err
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	application.ReportTaskProgress(ctx, "Checking out "+op.Branch)
	if err := g.changeBranch(ctx, repo, index, domain.RepositoryOperation{Kind: domain.CheckoutBranch, Source: ref}); err != nil {
		inspect, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if branch, readErr := g.read(inspect, repo.Root, "symbolic-ref", "--quiet", "HEAD"); readErr == nil && strings.TrimSuffix(branch, "\n") == ref {
			return fmt.Errorf("Checked out %s; Update stopped: %w", op.Branch, err)
		}
		return err
	}
	updated, err := g.ReadRepository(ctx, repo.Root)
	if err == nil && (updated.CurrentBranch != op.Branch || updated.Head != tip || updated.GitDir != repo.GitDir || updated.CommonDir != repo.CommonDir || updated.RemoteVersion != repo.RemoteVersion || updateHasChanges(updated)) {
		err = ErrStaleIndex
	}
	if err == nil {
		op.Kind, op.UpdateMethod, op.UpdateCleanup, op.ConfirmUpdateCleanup = domain.UpdateTrackedBranch, "Fast-forward", "", false
		_, err = g.updateTrackedBranch(ctx, updated, op)
	}
	if err != nil {
		return fmt.Errorf("Checked out %s; Update stopped: %w", op.Branch, err)
	}
	return nil
}
