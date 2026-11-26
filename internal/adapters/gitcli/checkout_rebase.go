package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) checkoutRebase(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	ctx = canonicalHistory(ctx)
	name, local := strings.CutPrefix(op.Source, "refs/heads/")
	if !op.ConfirmRewrite || !local || name == repo.CurrentBranch || repo.Head == "" || repo.CurrentBranch == "Detached HEAD" || repo.Bisect.Active() {
		return errors.New("confirm a different local branch and a checked-out base branch")
	}
	if err := g.checkBranchName(ctx, repo.Root, name); err != nil {
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
	checkAncestry := func() error {
		if _, err := os.Lstat(filepath.Join(repo.CommonDir, "info", "grafts")); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			return errors.New("rebase requires history without legacy Git grafts")
		}
		return checkLogBoundary(repo.CommonDir, repo.LogSnapshot.Boundary)
	}
	if err := checkAncestry(); err != nil {
		return err
	}
	shallow, err := g.read(ctx, repo.Root, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return err
	}
	if strings.TrimSpace(shallow) != "false" {
		return errors.New("rebase requires complete history")
	}
	tip, err := g.resolveSource(ctx, repo, op.Source)
	if err != nil {
		return err
	}
	if _, err := g.read(ctx, repo.Root, "merge-base", repo.Head, tip); err != nil {
		return errors.New("branches must share an ancestor")
	}
	rangeArg := repo.Head + ".." + tip
	merges, err := g.read(ctx, repo.Root, "rev-list", "--merges", "--max-count=1", rangeArg)
	if err != nil {
		return err
	}
	if merges != "" {
		return errors.New("the commits to replay include a merge commit")
	}
	chain, err := g.read(ctx, repo.Root, "rev-list", "--max-count=1001", rangeArg)
	if err != nil {
		return err
	}
	commits := strings.Fields(chain)
	if len(commits) > 1000 {
		return errors.New("at most 1000 commits can be replayed")
	}
	if len(commits) > 0 {
		refs, err := g.read(ctx, repo.Root, "for-each-ref", "--contains="+commits[len(commits)-1], "--format=%(refname)", "refs/remotes")
		if err != nil {
			return err
		}
		if strings.TrimSpace(refs) != "" {
			return errors.New("a commit to replay is published in a remote-tracking branch")
		}
	}
	if err := g.protectWorktreePaths(ctx, repo.Root, tip, repo.Head); err != nil {
		return err
	}
	if err := g.ensureBranchNotCheckedOut(ctx, repo.Root, op.Source); err != nil {
		return err
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	if _, err := g.resolveSource(ctx, repo, op.Source); err != nil {
		return err
	}
	if err := checkAncestry(); err != nil {
		return err
	}
	return g.runRebase(ctx, repo.Root, "--merge", "--no-autostash", "--no-update-refs", "--no-autosquash",
		"--no-rebase-merges", "--no-fork-point", "--no-rerere-autoupdate", "--reapply-cherry-picks", "--keep-empty", "--empty=stop", repo.Head, "--", name)
}
