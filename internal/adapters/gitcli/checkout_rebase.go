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
		gitDir, err := g.gitDirectory(ctx, repo.Root)
		if err != nil {
			return err
		}
		common, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			return err
		}
		common = filepath.Clean(strings.TrimSuffix(common, "\n"))
		if repo.GitDir != gitDir || repo.CommonDir != common {
			return ErrStaleIndex
		}
		if _, err := os.Lstat(filepath.Join(common, "info", "grafts")); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			return errors.New("rebase requires history without legacy Git grafts")
		}
		return checkLogBoundary(common, repo.LogSnapshot.Boundary)
	}
	targetTip := func() (string, error) {
		ref, err := g.run(ctx, repo.Root, "symbolic-ref", "--quiet", op.Source)
		if err == nil {
			return "", ErrStaleIndex
		}
		if ref.exitCode != 1 || ctx.Err() != nil {
			return "", err
		}
		return g.resolveSource(ctx, repo, op.Source)
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
	tip, err := targetTip()
	if err != nil {
		return err
	}
	if out, err := g.run(ctx, repo.Root, "merge-base", repo.Head, tip); err != nil {
		if out.exitCode == 1 && ctx.Err() == nil {
			return errors.New("branches must share an ancestor")
		}
		return err
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
	if _, err := targetTip(); err != nil {
		return err
	}
	if err := checkAncestry(); err != nil {
		return err
	}
	return g.runRebase(ctx, repo.Root, "--merge", "--no-autostash", "--no-update-refs", "--no-autosquash",
		"--no-rebase-merges", "--no-fork-point", "--no-rerere-autoupdate", "--reapply-cherry-picks", "--keep-empty", "--empty=stop", repo.Head, "--", name)
}
