package ui

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type repositoryOperationDialog struct {
	open                         bool
	root, label, detail, message string
	op                           domain.RepositoryOperation
	cancel                       context.CancelFunc
	afterCommit                  bool
	mergeSource                  string
	mainline                     string
	mainlines                    []string
}

func operationRef(repo domain.Repository, source string) string {
	if source == "" || source == repo.CurrentBranch {
		return repo.Head
	}
	for _, prefix := range []string{"refs/heads/", "refs/remotes/", ""} {
		if ref := prefix + source; repo.Refs[ref] != "" {
			return ref
		}
	}
	if slices.ContainsFunc(repo.History, func(c domain.Commit) bool { return c.Hash == source }) {
		return source
	}
	return ""
}

func (a *Workspace) repositoryOperationDisabled(id commandID, target commandTarget) string {
	repo := a.Model.Active().Repository
	switch {
	case a.OperateRepository == nil:
		return "Git operations are unavailable"
	case a.repositoryWrite != "" || a.repositoryRead != nil || a.repositoryOperation != nil || a.indexConfirmation != nil || a.commitConfirmation != nil:
		return "Wait for the current Git operation to finish"
	case repo.IndexVersion == "":
		return "Refresh the repository before continuing"
	}
	branch := target.source
	if branch == "" || id == updateProject {
		branch = repo.CurrentBranch
	}
	switch id {
	case mergeBranch, rebaseBranch, cherryPick, revertCommit:
		if repo.ResolvingConflicts() {
			return "Finish the current Git operation first"
		}
		if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" {
			return "Check out a local branch with a commit"
		}
		if id == cherryPick || id == revertCommit {
			if _, ok := a.operationCommit(target); !ok {
				return "Select a commit in the Git Log"
			}
			if slices.ContainsFunc(repo.Working, func(f domain.File) bool { return f.Status != "?" }) || len(repo.Staged) > 0 {
				return "Commit or stash changes before continuing"
			}
			return ""
		}
		if target.source != "" && (operationRef(repo, target.source) == "" || target.source == repo.CurrentBranch) {
			return "Select a different branch or tag"
		}
	case continueMerge, abortMerge, acceptCurrent, acceptIncoming, markResolved:
		if !repo.ResolvingConflicts() || ((id == continueMerge || id == abortMerge) && len(repo.MergeHeads) == 0) {
			return "No matching Git operation is in progress"
		}
		if id == continueMerge && len(repo.Conflicts) > 0 {
			return "Resolve all conflicted files"
		}
		if id == acceptCurrent || id == acceptIncoming || id == markResolved {
			conflict, ok := findMergeConflict(repo, target.file.Path)
			if !ok || conflict.Submodule {
				return "Select an unresolved file outside submodules"
			}
		}
	case continueRebase, skipRebase, abortRebase:
		if !repo.Rebase.Active() {
			return "No rebase is in progress"
		}
		if id == continueRebase && len(repo.Conflicts) > 0 {
			return "Resolve all conflicted files"
		}
	case continueSequence, skipSequence, abortSequence:
		if !repo.Sequencer.Active() {
			return "No cherry-pick or revert is in progress"
		}
		if id != abortSequence && repo.Sequencer.Remaining > 1 {
			return "Continue or skip this multi-commit sequence using Git"
		}
		if id == continueSequence && len(repo.Conflicts) > 0 {
			return "Resolve all conflicted files"
		}
	case newBranch:
		if operationRef(repo, target.source) == "" {
			return "Select a branch, tag or commit"
		}
	case checkoutBranch:
		ref := operationRef(repo, target.source)
		if branch == repo.CurrentBranch || (!strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/remotes/")) {
			return "Select a different local or remote-tracking branch"
		}
	case updateBranch, updateProject:
		upstream := repo.Upstreams[branch]
		if !remoteConfigured(repo, upstream.Remote) || repo.Refs["refs/heads/"+branch] == "" {
			return "Select a local branch with a remote upstream"
		}
	case fetch, pull, push:
		if len(repo.Remotes) == 0 {
			return "No remotes configured"
		}
		if id == pull && repo.CurrentBranch == "Detached HEAD" {
			return "Check out a local branch before pulling"
		}
		if id == push && repo.Refs["refs/heads/"+branch] == "" {
			return "Select a local branch with a commit"
		}
	}
	return ""
}

func remoteConfigured(repo domain.Repository, name string) bool {
	return name != "" && slices.ContainsFunc(repo.Remotes, func(r domain.Remote) bool { return r.Name == name })
}

func (a *Workspace) openRepositoryOperation(id commandID, target commandTarget) {
	repo := a.Model.Active().Repository
	s := &repositoryOperationDialog{open: true, root: repo.Root}
	branch := target.source
	if branch == "" || id == updateProject {
		branch = repo.CurrentBranch
	}
	upstream := repo.Upstreams[branch]
	remote := upstream.Remote
	if !remoteConfigured(repo, remote) && len(repo.Remotes) > 0 {
		remote = repo.Remotes[0].Name
		if remoteConfigured(repo, "origin") {
			remote = "origin"
		}
	}
	remoteBranch := upstream.Branch
	if remoteBranch == "" || remote != upstream.Remote {
		remoteBranch = branch
	}
	switch id {
	case cherryPick, revertCommit, continueSequence, skipSequence, abortSequence:
		commit, _ := a.operationCommit(target)
		s.configureSequence(repo, id, commit)
	case mergeBranch, continueMerge, abortMerge, acceptCurrent, acceptIncoming, markResolved:
		s.configureMerge(repo, id, target)
	case rebaseBranch, continueRebase, skipRebase, abortRebase:
		s.configureRebase(repo, id, target)
	case newBranch:
		s.label, s.detail = "Create Branch", "Start point: "+branch
		s.op = domain.RepositoryOperation{Kind: domain.CreateBranch, Source: operationRef(repo, target.source)}
	case checkoutBranch:
		s.label, s.detail = "Checkout", "Switch the working tree to "+displayBranch(branch)+"."
		s.op = domain.RepositoryOperation{Kind: domain.CheckoutBranch, Source: operationRef(repo, target.source)}
		if strings.HasPrefix(s.op.Source, "refs/remotes/") {
			branch = displayBranch(branch)
			remoteName := ""
			for _, item := range repo.Remotes {
				if strings.HasPrefix(branch, item.Name+"/") && len(item.Name) > len(remoteName) {
					remoteName = item.Name
				}
			}
			s.op.Branch = strings.TrimPrefix(branch, remoteName+"/")
			s.detail = "Create a local tracking branch from " + branch + " and check it out."
		}
	case fetch:
		s.label, s.detail = "Fetch", "Refresh remote-tracking branches."
		s.op = domain.RepositoryOperation{Kind: domain.FetchRemote, Remote: remote}
	case pull:
		s.label, s.detail = "Pull", "Fast-forward the current branch "+repo.CurrentBranch+"."
		s.op = domain.RepositoryOperation{Kind: domain.PullBranch, Branch: repo.CurrentBranch, Remote: remote, Target: remoteBranch}
	case push:
		s.label, s.detail = "Push", "Publish commits from local branch "+branch+"."
		s.op = domain.RepositoryOperation{Kind: domain.PushBranch, Branch: branch, Remote: remote, Target: remoteBranch, SetUpstream: upstream.Remote == ""}
	case updateBranch, updateProject:
		s.label, s.detail = "Update Branch", "Fetch "+upstream.Remote+"/"+upstream.Branch+" and fast-forward "+branch+"."
		s.op = domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: branch, Remote: upstream.Remote, Target: upstream.Branch}
	}
	a.repositoryOperation = s
}

func (a *Workspace) repositoryOperationView(c *ui.Context, p tokens) {
	s := a.repositoryOperation
	repo := a.Model.Active().Repository
	busy := s.cancel != nil
	width, _ := c.Size()
	op := &s.op
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(width-32, 560)).Background(p.panel).Border(1, p.theme.Border).Radius(6).
			Label(s.label + " dialog").Column().Children(func() {
			heading(c, p, s.label, repo.Name)
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.detail).FillWidth().TextColor(p.theme.TextMuted)
				if op.Kind == domain.StartMerge || op.Kind == domain.StartRebase {
					ui.Text(c, "Source branch or tag")
					options := []string{chooseMergeSource}
					for _, ref := range repo.Branches {
						if ref != repo.CurrentBranch && operationRef(repo, ref) != "" {
							options = append(options, ref)
						}
					}
					ui.Select(c, &s.mergeSource, options).Label("Merge source").Disabled(busy)
				}
				if len(s.mainlines) > 0 {
					ui.Text(c, "Mainline parent")
					ui.Select(c, &s.mainline, s.mainlines).Label("Choose mainline parent").Disabled(busy)
				}
				if op.Kind == domain.CreateBranch || (op.Kind == domain.CheckoutBranch && strings.HasPrefix(op.Source, "refs/remotes/")) {
					ui.Text(c, "Local branch")
					ui.TextInput(c, &op.Branch).Label("Local branch name").FillWidth().AutoFocus().Disabled(busy)
				}
				if op.Kind == domain.CreateBranch {
					ui.Checkbox(c, &op.Checkout, "Checkout after creation").Disabled(busy)
				}
				if op.Kind == domain.FetchRemote || op.Kind == domain.PullBranch || op.Kind == domain.PushBranch {
					var names []string
					for _, remote := range repo.Remotes {
						if !slices.Contains(names, remote.Name) {
							names = append(names, remote.Name)
						}
					}
					ui.Text(c, "Remote")
					ui.Select(c, &op.Remote, names).Label("Remote").Disabled(busy)
				}
				if op.Kind == domain.PullBranch || op.Kind == domain.PushBranch {
					ui.Text(c, "Remote branch")
					ui.TextInput(c, &op.Target).Label("Remote branch name").FillWidth().Disabled(busy)
				}
				if op.Kind == domain.PushBranch {
					ui.Checkbox(c, &op.SetUpstream, "Set upstream").Disabled(busy)
				}
				if s.message != "" {
					ui.Scroll(c).FillWidth().MaxHeight(140).Children(func() {
						ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
					})
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					if s.cancel != nil {
						s.cancel()
					} else {
						s.open = false
					}
				})
				needsBranch := op.Kind == domain.CreateBranch || (op.Kind == domain.CheckoutBranch && strings.HasPrefix(op.Source, "refs/remotes/"))
				needsTarget := op.Kind == domain.PullBranch || op.Kind == domain.PushBranch
				disabled := busy || a.repositoryWrite != "" || a.repositoryRead != nil || repo.Root != s.root || repo.IndexVersion == "" ||
					(len(s.mainlines) > 0 && slices.Index(s.mainlines, s.mainline) <= 0) ||
					(needsBranch && strings.TrimSpace(op.Branch) == "") || (needsTarget && strings.TrimSpace(op.Target) == "") ||
					((op.Kind == domain.StartMerge || op.Kind == domain.StartRebase) && (s.mergeSource == chooseMergeSource || s.mergeSource == repo.CurrentBranch ||
						operationRef(repo, s.mergeSource) == ""))
				ui.PrimaryButton(c, s.label).Label("Run " + s.label).Disabled(disabled).OnClick(func() {
					a.applyRepositoryOperation(s)
				})
			})
		})
	})
	if !s.open {
		if s.cancel != nil {
			s.cancel()
			s.open = true
		} else {
			a.repositoryOperation = nil
		}
	}
}

func (a *Workspace) applyRepositoryOperation(s *repositoryOperationDialog) {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.message = cancel, ""
	op := s.op
	if len(s.mainlines) > 0 {
		op.Mainline = slices.Index(s.mainlines, s.mainline)
	}
	snapshot := a.Model.Active().Repository
	if op.Kind == domain.StartMerge || op.Kind == domain.StartRebase {
		op.Source = operationRef(snapshot, s.mergeSource)
	}
	op.Branch, op.Target = strings.TrimSpace(op.Branch), strings.TrimSpace(op.Target)
	a.beginRepositoryWrite(s.root, s.label+"…")
	a.OperateRepository(ctx, snapshot, op, func(repo domain.Repository, err, refreshErr error) {
		cancel()
		s.cancel = nil
		message := s.label + " completed"
		paused := op.Kind == domain.StartMerge && refreshErr == nil && len(repo.MergeHeads) > 0
		rebasePaused := (op.Kind == domain.StartRebase || op.Kind == domain.ContinueRebase || op.Kind == domain.SkipRebase) &&
			refreshErr == nil && repo.Rebase.Active() && repo.Rebase != snapshot.Rebase
		sequencePaused := (op.Kind == domain.StartCherryPick || op.Kind == domain.StartRevert || op.Kind == domain.ContinueSequence || op.Kind == domain.SkipSequence) &&
			refreshErr == nil && repo.Sequencer.Active() && repo.Sequencer != snapshot.Sequencer
		unchanged := refreshErr == nil && errors.Is(err, domain.ErrRevertUnchanged)
		if unchanged {
			message = err.Error()
			s.open = false
		} else if sequencePaused {
			message = sequenceTitle(repo.Sequencer) + " paused. Resolve conflicts and continue, or skip an empty commit."
			s.open = false
		} else if rebasePaused {
			message = "Rebase paused. Review the replayed commit before continuing."
			s.open = false
		} else if paused {
			message = "Merge paused. Resolve the conflicted files or abort."
			s.open = false
		} else if err != nil {
			message = s.label + " · " + err.Error()
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				message = "Git did not confirm " + s.label + " · " + err.Error()
			}
		} else {
			s.open = false
		}
		if s.afterCommit {
			message = "Commit succeeded. " + message
		}
		if err != nil && !paused && !rebasePaused && !sequencePaused && !unchanged {
			s.message = message
		}
		a.finishRepositoryWrite(s.root, repo, refreshErr, message)
		if (paused || rebasePaused || sequencePaused) && a.Model.Active().Repository.Root == s.root {
			a.run(showCommit, commandTarget{})
		}
	})
}
