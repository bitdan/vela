package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"vela/internal/application"
	"vela/internal/domain"
)

func (g *Executor) PreviewSquash(ctx context.Context, repo domain.Repository, selection []string) (domain.SquashPreview, error) {
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 30*time.Second)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	return g.squashPreview(ctx, repo, selection)
}

func (g *Executor) squashPreview(ctx context.Context, repo domain.Repository, selection []string) (domain.SquashPreview, error) {
	plan, err := g.commitRangePreview(ctx, repo, selection, 2)
	if err == nil {
		err = application.ValidateSquashMessage(plan.Message)
	}
	return plan, err
}

func (g *Executor) commitRangePreview(ctx context.Context, repo domain.Repository, selection []string, minimum int) (domain.CommitRewritePreview, error) {
	var plan domain.CommitRewritePreview
	if err := application.ValidateCommitRangeSelection(selection, minimum); err != nil {
		return plan, err
	}
	if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" || repo.Bisect.Active() {
		return plan, errors.New("check out a local branch outside Bisect")
	}
	_, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return plan, err
	}
	if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
		return plan, err
	}
	gitDir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return plan, err
	}
	common, err := g.read(ctx, repo.Root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return plan, err
	}
	if gitDir != repo.GitDir || filepath.Clean(strings.TrimSpace(common)) != repo.CommonDir {
		return plan, errors.New("repository metadata changed; refresh before Squash")
	}
	if err := g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
		return plan, err
	}
	if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
		return plan, err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return plan, err
	}
	shallow, err := g.read(ctx, repo.Root, "rev-parse", "--is-shallow-repository")
	if err != nil {
		return plan, err
	}
	if strings.TrimSpace(shallow) != "false" {
		return plan, errors.New("Squash requires complete history outside a shallow repository")
	}
	chain, err := g.read(ctx, repo.Root, "rev-list", "--first-parent", "--parents", "--max-count=1001", repo.Head)
	if err != nil {
		return plan, err
	}
	rows := strings.FieldsFunc(strings.TrimSpace(chain), func(r rune) bool { return r == '\n' })
	newest, oldest := len(rows), -1
	chosen := make(map[string]bool, len(selection))
	for _, id := range selection {
		chosen[id] = true
	}
	for i, row := range rows {
		parts := strings.Fields(row)
		if len(parts) == 0 {
			return plan, errors.New("invalid commit chain")
		}
		if chosen[parts[0]] {
			newest = min(newest, i)
			oldest = max(oldest, i)
		}
	}
	if oldest < 0 || oldest >= 1000 || oldest-newest+1 != len(selection) {
		return plan, errors.New("select a contiguous range on this branch (at most 1000 rewritten commits)")
	}
	for i := 0; i <= oldest; i++ {
		parts := strings.Fields(rows[i])
		if len(parts) > 2 {
			return plan, errors.New("Squash range and replayed descendants must not include merge commits")
		}
		if i >= newest {
			if !chosen[parts[0]] {
				return plan, errors.New("selection omits an intermediate commit")
			}
			plan.Selected = append(plan.Selected, parts[0])
		} else {
			plan.Replay = append(plan.Replay, parts[0])
		}
		if i == oldest && len(parts) == 2 {
			plan.Parent = parts[1]
		}
	}
	oldID := plan.Selected[len(plan.Selected)-1]
	newID := plan.Selected[0]
	published, err := g.read(ctx, repo.Root, "for-each-ref", "--contains="+oldID, "--format=%(refname)", "refs/remotes")
	if err != nil {
		return plan, err
	}
	if strings.TrimSpace(published) != "" {
		return plan, errors.New("a selected commit or rewritten descendant is published in a remote-tracking branch")
	}
	refs, err := g.read(ctx, repo.Root, "for-each-ref", "--format=%(refname)%00%(objectname)%00", "refs/remotes")
	if err != nil {
		return plan, err
	}
	tree := func(id string) (string, error) {
		v, e := g.read(ctx, repo.Root, "rev-parse", "--verify", "--end-of-options", id+"^{tree}")
		v = strings.TrimSpace(v)
		if e == nil && !validObjectID(v) {
			e = errors.New("invalid tree identity")
		}
		return v, e
	}
	plan.HeadTree, err = tree(repo.Head)
	if err != nil {
		return plan, err
	}
	plan.SelectedTree, err = tree(newID)
	if err != nil {
		return plan, err
	}
	author, err := g.read(ctx, repo.Root, "show", "-s", "--no-show-signature", "--format=%an%x00%ae%x00%aI", oldID, "--")
	if err != nil {
		return plan, err
	}
	fields := strings.Split(strings.TrimSuffix(author, "\n"), "\x00")
	if len(fields) != 3 {
		return plan, errors.New("invalid original author identity")
	}
	for _, s := range fields {
		if !utf8.ValidString(s) || strings.ContainsAny(s, "\r\n") {
			return plan, errors.New("invalid original author identity")
		}
	}
	plan.Author, plan.AuthorEmail, plan.AuthorDate = fields[0], fields[1], fields[2]
	plan.Message, err = g.read(ctx, repo.Root, "log", "--first-parent", "--reverse", "--no-show-signature", "--format=%B", "--max-count="+strconv.Itoa(len(plan.Selected)), newID, "--")
	if err != nil {
		return plan, err
	}
	plan.Message = strings.TrimSpace(plan.Message)
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return plan, err
	}
	plan.Version = fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s\x00%s\x00%s", repo.Root, repo.Head, repo.IndexVersion, repo.WorktreeVersion, strings.Join(plan.Selected, "\x00"), strings.Join(plan.Replay, "\x00"), refs))))
	return plan, ctx.Err()
}

func (g *Executor) squash(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) error {
	ctx = canonicalHistory(ctx)
	if !op.ConfirmSquash {
		return errors.New("confirm the rewritten commit range before Squash")
	}
	if err := application.ValidateSquashMessage(op.Message); err != nil {
		return err
	}
	plan, err := g.squashPreview(ctx, repo, op.SquashSelection)
	if err != nil {
		return err
	}
	if op.SquashVersion == "" || op.SquashVersion != plan.Version {
		return errors.New("Squash range or remote refs changed; preview again")
	}
	application.ReportTaskProgress(ctx, "Creating squashed commit")
	args := []string{"commit-tree", plan.SelectedTree}
	if plan.Parent != "" {
		args = append(args, "-p", plan.Parent)
	}
	out, err := g.runInputWithEnv(ctx, repo.Root, op.Message+"\n", []string{"GIT_AUTHOR_NAME=" + plan.Author, "GIT_AUTHOR_EMAIL=" + plan.AuthorEmail, "GIT_AUTHOR_DATE=" + plan.AuthorDate}, args...)
	if err != nil {
		return operationError(ctx, out, err)
	}
	squashed := strings.TrimSpace(out.stdout)
	if !validObjectID(squashed) {
		return errors.New("Git returned an invalid squashed commit identity")
	}
	latest, err := g.squashPreview(ctx, repo, op.SquashSelection)
	if err != nil {
		return err
	}
	if latest.Version != plan.Version || !slices.Equal(latest.Selected, plan.Selected) {
		return errors.New("Squash preview changed before writing; no branch was moved")
	}
	if len(plan.Replay) == 0 {
		application.ReportTaskProgress(ctx, "Publishing squashed HEAD")
		err = g.runOperation(ctx, repo.Root, "update-ref", "-m", "vela: squash commits", "refs/heads/"+repo.CurrentBranch, squashed, repo.Head)
	} else {
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, squashed); err != nil {
			return err
		}
		application.ReportTaskProgress(ctx, "Replaying later commits")
		err = g.runRebase(ctx, repo.Root, "--merge", "--no-autostash", "--no-update-refs", "--no-autosquash", "--no-rebase-merges", "--no-fork-point", "--no-rerere-autoupdate", "--reapply-cherry-picks", "--keep-empty", "--empty=stop", "--onto", squashed, plan.Selected[0], repo.CurrentBranch)
	}
	if err != nil {
		return err
	}
	actual, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(actual) != plan.HeadTree {
		return errors.New("Squash completed with a different final tree; inspect refreshed history and files")
	}
	return nil
}
