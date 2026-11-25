package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"vela/internal/domain"
)

func (g *Executor) ExecuteRepositoryOperation(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation) error {
	ctx = withNetworkPolicy(ctx, op.Network)
	if repo.Root == "" || repo.IndexVersion == "" {
		return errors.New("refresh the local repository before running this operation")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	current, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return err
	}
	if err := op.Safety.CheckOperation(current, op); err != nil {
		return err
	}
	switch op.Kind {
	case domain.RewordCommit, domain.FixupCommits, domain.DropCommit:
		return g.rewriteCommit(ctx, repo, op)
	case domain.SquashCommits:
		return g.squash(ctx, repo, op)
	case domain.CheckoutRevision, domain.RestoreRevisionFiles, domain.CherryPickSelected, domain.RevertSelected:
		return g.historicalOperation(ctx, repo, index, op)
	case domain.SaveShelf, domain.ApplyShelf, domain.RenameShelf, domain.DeleteShelf, domain.RestoreShelf, domain.PurgeShelf:
		return g.shelfOperation(ctx, repo, index, op)
	case domain.StageHunk, domain.UnstageHunk, domain.DiscardHunk:
		return g.applyHunk(ctx, repo, op)
	case domain.RenameBranch, domain.DeleteBranch, domain.CreateTag:
		return g.manageRef(ctx, repo, index, op)
	case domain.AddRemote, domain.EditRemote, domain.RemoveRemote:
		return g.manageRemote(ctx, repo, op)
	case domain.ResetRevision:
		return g.reset(ctx, repo, index, op)
	case domain.SaveStash, domain.ApplyStash, domain.DropStash:
		return g.stash(ctx, repo, index, op)
	case domain.CreateWorktree, domain.RemoveWorktree:
		return g.worktree(ctx, repo, op)
	case domain.StartBisect, domain.BisectGood, domain.BisectBad, domain.BisectSkip, domain.ResetBisect:
		return g.bisect(ctx, repo, index, op)
	case domain.CreateBranch, domain.CheckoutBranch:
		return g.changeBranch(ctx, repo, index, op)
	case domain.FetchRemote, domain.PullBranch, domain.PushBranch, domain.UpdateTrackedBranch:
		if op.Kind == domain.UpdateTrackedBranch && op.UpdateMethod != "" {
			_, err := g.updateTrackedBranch(ctx, repo, op)
			return err
		}
		return g.syncRemote(ctx, repo, current, index, op)
	case domain.StartMerge, domain.ContinueMerge, domain.AbortMerge:
		return g.merge(ctx, repo, index, op)
	case domain.StartRebase, domain.ContinueRebase, domain.SkipRebase, domain.AbortRebase:
		return g.rebase(ctx, repo, index, op)
	case domain.StartCherryPick, domain.StartRevert, domain.ContinueSequence, domain.SkipSequence, domain.AbortSequence:
		return g.sequence(ctx, repo, index, op)
	case domain.AcceptCurrent, domain.AcceptIncoming, domain.MarkResolved, domain.SaveMergeResult:
		if err := g.verifyConflictOperation(ctx, repo); err != nil {
			return err
		}
		return g.resolveMergeFile(ctx, repo, index, op)
	default:
		return errors.New("unknown repository operation")
	}
}

func (g *Executor) changeBranch(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	source, err := g.resolveSource(ctx, repo, op.Source)
	if err != nil {
		return err
	}
	if op.Kind == domain.CreateBranch || strings.HasPrefix(op.Source, "refs/remotes/") {
		if err := g.checkBranchName(ctx, repo.Root, op.Branch); err != nil {
			return err
		}
		if op.Kind == domain.CreateBranch && !op.Checkout {
			return g.runOperation(ctx, repo.Root, "branch", "--no-track", op.Branch, source)
		}
	}
	if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
		return err
	}
	args := []string{"checkout", "--no-guess", "--no-overwrite-ignore", "--no-recurse-submodules"}
	switch {
	case op.Kind == domain.CreateBranch:
		args = append(args, "--no-track", "-b", op.Branch, source)
	case strings.HasPrefix(op.Source, "refs/heads/"):
		name := strings.TrimPrefix(op.Source, "refs/heads/")
		if err := g.checkBranchName(ctx, repo.Root, name); err != nil {
			return err
		}
		args = append(args, name)
	case strings.HasPrefix(op.Source, "refs/remotes/"):
		args = append(args, "-b", op.Branch, "--track", op.Source)
	default:
		return errors.New("select a local or remote-tracking branch to check out")
	}
	return g.runOperation(ctx, repo.Root, append(args, "--")...)
}

func (g *Executor) syncRemote(ctx context.Context, repo, current domain.Repository, index string, op domain.RepositoryOperation) error {
	if err := g.verifyRemote(ctx, repo, op.Remote, op.Kind == domain.PushBranch); err != nil {
		return err
	}
	if op.Kind == domain.FetchRemote {
		return g.fetchBranches(ctx, repo.Root, op.Remote, "*", op.Network)
	}
	if err := g.checkBranchName(ctx, repo.Root, op.Target); err != nil {
		return err
	}
	if op.Kind == domain.PushBranch {
		if err := g.checkBranchName(ctx, repo.Root, op.Branch); err != nil {
			return err
		}
		if _, err := g.resolveSource(ctx, repo, "refs/heads/"+op.Branch); err != nil {
			return err
		}
		args := []string{"-c", "remote." + op.Remote + ".mirror=false", "push", "--porcelain",
			"--no-force", "--no-follow-tags", "--recurse-submodules=no"}
		if op.SetUpstream {
			args = append(args, "--set-upstream")
		}
		return g.runOperation(ctx, repo.Root, append(args, "--", op.Remote, "refs/heads/"+op.Branch+":refs/heads/"+op.Target)...)
	}
	return g.integrateRemote(ctx, repo, current, index, op)
}

func (g *Executor) integrateRemote(ctx context.Context, repo, current domain.Repository, index string, op domain.RepositoryOperation) error {
	if err := g.checkBranchName(ctx, repo.Root, op.Branch); err != nil {
		return err
	}
	localRef := "refs/heads/" + op.Branch
	onCurrent := op.Branch == current.CurrentBranch
	if op.Kind == domain.PullBranch && !onCurrent {
		return errors.New("Pull must target the currently checked-out branch")
	}
	var localTip string
	// A first pull can populate an unborn local branch.
	if !(onCurrent && current.Head == "" && op.Kind == domain.PullBranch) {
		var err error
		localTip, err = g.resolveSource(ctx, repo, localRef)
		if err != nil {
			return err
		}
	}
	if op.Kind == domain.UpdateTrackedBranch {
		if err := g.verifyUpstream(ctx, repo, localRef, op); err != nil {
			return err
		}
	}
	if onCurrent {
		if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
			return err
		}
		if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
			return err
		}
	} else if err := g.ensureBranchNotCheckedOut(ctx, repo.Root, localRef); err != nil {
		return err
	}
	if err := g.fetchBranches(ctx, repo.Root, op.Remote, op.Target, op.Network); err != nil {
		return err
	}
	if err := g.fastForward(ctx, repo, op, localRef, localTip, onCurrent); err != nil {
		return fmt.Errorf("Fetched %s/%s. Could not update %s: %w", op.Remote, op.Target, op.Branch, err)
	}
	return nil
}

func (g *Executor) fastForward(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, localRef, localTip string, onCurrent bool) error {
	_, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return err
	}
	if op.Kind == domain.UpdateTrackedBranch {
		if err := g.verifyUpstream(ctx, repo, localRef, op); err != nil {
			return err
		}
	}
	target, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "refs/remotes/"+op.Remote+"/"+op.Target+"^{commit}")
	if err != nil {
		return err
	}
	target = strings.TrimSpace(target)
	if onCurrent {
		if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
			return err
		}
		if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
			return err
		}
		return g.runOperation(ctx, repo.Root, "-c", "submodule.recurse=false", "merge", "--ff-only", "--no-squash", "--no-autostash", "--no-overwrite-ignore", target)
	}
	if err := g.ensureBranchNotCheckedOut(ctx, repo.Root, localRef); err != nil {
		return err
	}
	ancestor, err := g.isAncestor(ctx, repo.Root, target, localTip)
	if err != nil {
		return err
	}
	if ancestor {
		_, err := g.resolveSource(ctx, repo, localRef)
		return err
	}
	ancestor, err = g.isAncestor(ctx, repo.Root, localTip, target)
	if err != nil {
		return err
	}
	if !ancestor {
		return errors.New("the local and remote branches have diverged")
	}
	return g.runOperation(ctx, repo.Root, "update-ref", "-m", "vela: fast-forward tracked branch", localRef, target, localTip)
}

func (g *Executor) checkBranchName(ctx context.Context, root, branch string) error {
	if branch == "" || branch == "HEAD" || strings.HasPrefix(branch, "-") {
		return errors.New("enter a valid branch name")
	}
	out, err := g.run(ctx, root, "check-ref-format", "refs/heads/"+branch)
	if out.exitCode == 1 && ctx.Err() == nil {
		return fmt.Errorf("%q is not a valid branch name", branch)
	}
	return err
}

func (g *Executor) resolveSource(ctx context.Context, repo domain.Repository, ref string) (string, error) {
	if validObjectID(ref) {
		if !slices.ContainsFunc(repo.History, func(c domain.Commit) bool { return c.Hash == ref }) && ref != repo.Head {
			return "", errors.New("select a commit in the current repository")
		}
	} else {
		expected := repo.Refs[ref]
		if expected == "" {
			return "", errors.New("select an existing branch, tag or commit")
		}
		actual, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "--end-of-options", ref)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(actual) != expected {
			return "", ErrStaleIndex
		}
		ref = expected
	}
	value, err := g.read(ctx, repo.Root, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	return strings.TrimSpace(value), err
}

func (g *Executor) verifyUpstream(ctx context.Context, repo domain.Repository, ref string, op domain.RepositoryOperation) error {
	if repo.Upstreams[op.Branch] != (domain.Upstream{Remote: op.Remote, Branch: op.Target}) {
		return errors.New("select a local branch with a configured remote upstream")
	}
	value, err := g.read(ctx, repo.Root, "for-each-ref", "--format=%(refname)%00%(upstream:remotename)%00%(upstream:remoteref)", ref)
	if err != nil {
		return err
	}
	if value != ref+"\x00"+op.Remote+"\x00refs/heads/"+op.Target+"\n" {
		return ErrStaleIndex
	}
	return nil
}

func (g *Executor) requireCleanTracked(ctx context.Context, root string) error {
	value, err := g.read(ctx, root, "-c", "core.fsmonitor=false", "status", "--porcelain=v1", "-z",
		"--untracked-files=no", "--ignore-submodules=none")
	if err == nil && value != "" {
		return errors.New("commit or stash tracked changes before changing the working tree")
	}
	return err
}

func (g *Executor) ensureBranchNotCheckedOut(ctx context.Context, root, ref string) error {
	value, err := g.read(ctx, root, "worktree", "list", "--porcelain", "-z")
	if err == nil && slices.Contains(nulFields(value), "branch "+ref) {
		return errors.New("the target branch is checked out in a worktree")
	}
	return err
}

func (g *Executor) isAncestor(ctx context.Context, root, older, newer string) (bool, error) {
	out, err := g.run(ctx, root, "merge-base", "--is-ancestor", older, newer)
	if out.exitCode == 1 && ctx.Err() == nil {
		return false, nil
	}
	return err == nil, err
}

func (g *Executor) fetchBranches(ctx context.Context, root, remote, branch string, policies ...domain.NetworkPolicy) error {
	spec := "+refs/heads/" + branch + ":refs/remotes/" + remote + "/" + branch
	args := []string{"fetch", "--atomic", "--no-prune", "--no-prune-tags", "--no-recurse-submodules", "--no-auto-maintenance", "--no-write-fetch-head", "--refmap="}
	tags := "Never" // Preserve callers without a dispatched preference.
	if len(policies) > 0 && policies[0].FetchTags != "" {
		tags = policies[0].FetchTags
	}
	switch tags {
	case "Always":
		args = append(args, "--tags")
	case "Never":
		args = append(args, "--no-tags")
	case "Auto": // Git's automatic tag following and repository tagOpt.
	default:
		return errors.New("invalid fetch tags preference")
	}
	return g.runOperation(ctx, root, append(args, "--", remote, spec)...)
}

func (g *Executor) readRemoteConfig(ctx context.Context, root string) (string, string, error) {
	out, err := g.run(ctx, root, "config", "--null", "--get-regexp", "^remote\\..*\\.(url|pushurl|fetch|mirror)$")
	if err != nil && (out.exitCode != 1 || ctx.Err() != nil) {
		return "", "", err
	}
	return out.stdout, fmt.Sprintf("%x", sha256.Sum256([]byte(out.stdout))), nil
}

func (g *Executor) verifyRemote(ctx context.Context, repo domain.Repository, remote string, push bool) error {
	if remote == "" || strings.HasPrefix(remote, "-") || !slices.ContainsFunc(repo.Remotes, func(r domain.Remote) bool { return r.Name == remote }) {
		return errors.New("select a configured remote")
	}
	if err := g.checkBranchName(ctx, repo.Root, remote); err != nil {
		return err
	}
	_, version, err := g.readRemoteConfig(ctx, repo.Root)
	if err != nil {
		return err
	}
	if version != repo.RemoteVersion {
		return ErrStaleIndex
	}
	if push {
		urls, err := g.read(ctx, repo.Root, "remote", "get-url", "--push", "--all", remote)
		if err != nil {
			return err
		}
		if strings.Count(strings.TrimSuffix(urls, "\n"), "\n") > 0 {
			return errors.New("this remote has multiple push destinations")
		}
	}
	return nil
}

var httpAddress = regexp.MustCompile(`(?i)https?://[^\s'"]+`)

func (g *Executor) runOperation(ctx context.Context, root string, args ...string) error {
	out, err := g.run(ctx, root, args...)
	return operationError(ctx, out, err)
}

func operationError(ctx context.Context, out result, err error) error {
	if err == nil || ctx.Err() != nil {
		return err
	}
	detail := strings.TrimSpace(out.stderr + "\n" + out.stdout)
	detail = httpAddress.ReplaceAllStringFunc(detail, func(address string) string {
		parsed, err := url.Parse(address)
		if err != nil {
			return "[remote URL]"
		}
		parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
		return parsed.String()
	})
	if detail != "" {
		return fmt.Errorf("%w: %s", err, detail)
	}
	return err
}
