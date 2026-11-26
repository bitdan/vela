package gitcli

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
	"vela/internal/application"
	"vela/internal/domain"
)

func (g *Executor) PreviewCommitRewrite(ctx context.Context, repo domain.Repository, kind domain.RepositoryOperationKind, ids []string) (domain.CommitRewritePreview, error) {
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 30*time.Second)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	return g.commitRewritePreview(ctx, repo, kind, ids)
}
func (g *Executor) commitRewritePreview(ctx context.Context, repo domain.Repository, kind domain.RepositoryOperationKind, ids []string) (domain.CommitRewritePreview, error) {
	minimum, prefix := 1, "reword:"
	switch kind {
	case domain.RewordCommit:
		if len(ids) != 1 {
			return domain.CommitRewritePreview{}, errors.New("select exactly one commit for message editing")
		}
	case domain.FixupCommits:
		minimum, prefix = 2, "fixup:"
	case domain.DropCommit:
		if len(ids) != 1 {
			return domain.CommitRewritePreview{}, errors.New("select exactly one commit to drop")
		}
		prefix = "drop:"
	default:
		return domain.CommitRewritePreview{}, errors.New("unsupported commit rewrite operation")
	}
	plan, err := g.commitRangePreview(ctx, repo, ids, minimum)
	if err == nil && kind == domain.DropCommit {
		if plan.Parent == "" {
			return plan, errors.New("dropping a root commit is not supported")
		}
		plan.Affected, err = g.ReadRevisionChanges(ctx, repo.Root, plan.Selected[0], 0)
	}
	if err == nil && kind == domain.FixupCommits {
		raw, e := g.read(ctx, repo.Root, "cat-file", "commit", plan.Selected[len(plan.Selected)-1])
		err = e
		if err == nil {
			_, message, ok := strings.Cut(raw, "\n\n")
			if !ok || len(message) > 64<<10 || !utf8.ValidString(message) || strings.ContainsRune(message, 0) {
				err = errors.New("original message is not bounded UTF-8")
			} else {
				plan.Message = message
			}
		}
	}
	if err == nil {
		plan.Version = prefix + plan.Version
	}
	return plan, err
}
func (g *Executor) rewriteCommit(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) error {
	ctx = canonicalHistory(ctx)
	if !op.ConfirmRewrite {
		return errors.New("confirm the commit and rewritten descendants")
	}
	if err := application.ValidateSquashMessage(op.Message); op.Kind == domain.RewordCommit && err != nil {
		return err
	}
	plan, err := g.commitRewritePreview(ctx, repo, op.Kind, op.RewriteSelection)
	if err != nil {
		return err
	}
	if op.RewriteVersion == "" || op.RewriteVersion != plan.Version {
		return errors.New("history rewrite preview changed; preview again")
	}
	if op.Kind == domain.DropCommit {
		return g.dropSelectedCommit(ctx, repo, op, plan)
	}
	application.ReportTaskProgress(ctx, "Creating rewritten commit")
	args := []string{"commit-tree", plan.SelectedTree}
	if plan.Parent != "" {
		args = append(args, "-p", plan.Parent)
	}
	message := op.Message + "\n"
	if op.Kind == domain.FixupCommits {
		message = plan.Message
	}
	out, err := g.runInputWithEnv(ctx, repo.Root, message, []string{"GIT_AUTHOR_NAME=" + plan.Author, "GIT_AUTHOR_EMAIL=" + plan.AuthorEmail, "GIT_AUTHOR_DATE=" + plan.AuthorDate}, args...)
	if err != nil {
		return operationError(ctx, out, err)
	}
	target := strings.TrimSpace(out.stdout)
	if !validObjectID(target) {
		return errors.New("invalid rewritten commit identity")
	}
	latest, err := g.commitRewritePreview(ctx, repo, op.Kind, op.RewriteSelection)
	if err != nil {
		return err
	}
	if latest.Version != plan.Version {
		return errors.New("history changed before publication")
	}
	if len(plan.Replay) == 0 {
		application.ReportTaskProgress(ctx, "Publishing rewritten HEAD")
		err = g.runOperation(ctx, repo.Root, "update-ref", "-m", "vela: edit commit message", "refs/heads/"+repo.CurrentBranch, target, repo.Head)
	} else {
		if err = g.protectWorktreePaths(ctx, repo.Root, repo.Head, target); err != nil {
			return err
		}
		application.ReportTaskProgress(ctx, "Replaying later commits")
		err = g.runRebase(ctx, repo.Root, "--merge", "--no-autostash", "--no-update-refs", "--no-autosquash", "--no-rebase-merges", "--no-fork-point", "--no-rerere-autoupdate", "--reapply-cherry-picks", "--keep-empty", "--empty=stop", "--onto", target, "--", plan.Selected[0], repo.CurrentBranch)
	}
	if err != nil {
		return err
	}
	tree, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(tree) != plan.HeadTree {
		return errors.New("rewritten history has a different final tree; inspect refreshed state")
	}
	return nil
}
