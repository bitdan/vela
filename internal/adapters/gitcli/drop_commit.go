package gitcli

import (
	"context"
	"errors"
	"strings"
	"vela/internal/application"
	"vela/internal/domain"
)

func (g *Executor) dropSelectedCommit(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, plan domain.CommitRewritePreview) error {
	if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, plan.Parent); err != nil {
		return err
	}
	latest, err := g.commitRewritePreview(ctx, repo, op.Kind, op.RewriteSelection)
	if err != nil {
		return err
	}
	if latest.Version != plan.Version {
		return errors.New("Drop preview changed; no commit was removed")
	}
	application.ReportTaskProgress(ctx, "Dropping selected commit")
	err = g.runRebase(ctx, repo.Root, "--merge", "--no-autostash", "--no-update-refs", "--no-autosquash", "--no-rebase-merges", "--no-fork-point", "--no-rerere-autoupdate", "--reapply-cherry-picks", "--keep-empty", "--empty=stop", "--onto", plan.Parent, plan.Selected[0], repo.CurrentBranch)
	if err != nil {
		return err
	}
	if len(plan.Replay) == 0 {
		head, e := g.read(ctx, repo.Root, "rev-parse", "--verify", "HEAD")
		if e != nil {
			return e
		}
		if strings.TrimSpace(head) != plan.Parent {
			return errors.New("Drop completed with an unexpected HEAD; inspect refreshed state")
		}
	}
	return nil
}
