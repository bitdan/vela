package gitcli

import (
	"context"
	"errors"
	"strings"

	"vela/internal/domain"
)

// manageRef performs non-force ref management. Source identity and checkout
// were captured by the confirmation and are revalidated under the write lock.
func (g *Executor) manageRef(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	source, err := g.resolveSource(ctx, repo, op.Source)
	if err != nil {
		return err
	}
	if op.Kind == domain.CreateTag {
		if op.Target == "" || op.Target == "HEAD" || strings.HasPrefix(op.Target, "-") {
			return errors.New("enter a valid tag name")
		}
		if err := g.runOperation(ctx, repo.Root, "check-ref-format", "refs/tags/"+op.Target); err != nil {
			return err
		}
		// The first tag UI deliberately creates only lightweight, local tags.
		// Disable implicit signing from user configuration for this explicit mode.
		return g.runOperation(ctx, repo.Root, "-c", "tag.gpgSign=false", "tag", "--no-sign", "--", op.Target, source)
	}
	if !strings.HasPrefix(op.Source, "refs/heads/") {
		return errors.New("select a local branch")
	}
	branch := strings.TrimPrefix(op.Source, "refs/heads/")
	if err := g.checkBranchName(ctx, repo.Root, branch); err != nil {
		return err
	}
	if branch != repo.CurrentBranch {
		if err := g.ensureBranchNotCheckedOut(ctx, repo.Root, op.Source); err != nil {
			return err
		}
	}
	if op.Kind == domain.RenameBranch {
		if err := g.checkBranchName(ctx, repo.Root, op.Branch); err != nil {
			return err
		}
		if branch == op.Branch {
			return errors.New("enter a different branch name")
		}
		return g.runOperation(ctx, repo.Root, "branch", "--move", "--", branch, op.Branch)
	}
	if branch == repo.CurrentBranch {
		return errors.New("cannot delete the current branch")
	}
	merged, err := g.isAncestor(ctx, repo.Root, source, repo.Head)
	if err != nil {
		return err
	}
	if !merged {
		return errors.New("the branch is not fully merged into current HEAD; force deletion is not supported")
	}
	return g.runOperation(ctx, repo.Root, "branch", "--delete", "--", branch)
}
