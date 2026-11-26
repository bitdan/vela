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
	shelfHunkDocs                map[string]domain.PatchDocument
	shelfHunkChosen              map[string][]bool
	shelfHunkPath                string
	shelfHunkRead                *readRequest
	shelfHunkList                ui.ListState
	shelfFiles                   []domain.File
	shelfChosen                  map[string]bool
	shelfList                    ui.ListState
	shelfSelected                int
	updateFinished               bool
	open                         bool
	root, label, detail, message string
	op                           domain.RepositoryOperation
	cancel                       context.CancelFunc
	afterCommit                  bool
	mergeSource                  string
	mainline                     string
	mainlines                    []string
	acknowledged                 bool
	snapshot                     domain.Repository
	choice                       string
	choices                      []string
	management                   bool
	historyChanges               *historyChangeSelection
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
	if reason := a.protectedCommandReason(id, target); reason != "" {
		return reason
	}
	if id == resetRevision && a.updateDirtyBuffer(repo.Root) {
		return "Save or discard unsaved source documents before resetting"
	}
	switch id {
	case continueMerge, abortMerge, continueRebase, skipRebase, abortRebase:
		if a.updateDirtyBuffer(repo.Root) {
			return "Save or discard unsaved source documents before continuing or aborting"
		}
	}
	switch {
	case a.OperateRepository == nil:
		return "Git operations are unavailable"
	case a.repositoryWrite != "" || a.repositoryRead != nil || a.repositoryOperation != nil || a.indexConfirmation != nil || a.commitConfirmation != nil:
		return "Wait for the current Git operation to finish"
	case repo.IndexVersion == "":
		return "Refresh the repository before continuing"
	}
	branch := target.source
	if isBisectCommand(id) {
		return bisectDisabled(repo, id)
	}
	if repo.Bisect.Active() {
		switch id {
		case resetRevision, stashChanges, unstashChanges, dropStash, newBranch, checkoutBranch, checkoutRebase, checkoutUpdate, pull, updateBranch, updateProject, mergeBranch, rebaseBranch, cherryPick, revertCommit, renameBranch, deleteBranch, newTag:
			return "End Bisect before running this operation"
		}
	}
	if branch == "" || id == updateProject {
		branch = repo.CurrentBranch
	}
	switch id {
	case renameBranch, deleteBranch, newTag:
		if repo.ResolvingConflicts() {
			return "Finish the current Git operation first"
		}
		if id == newTag {
			if operationRef(repo, target.source) == "" {
				return "Select a branch, tag or commit"
			}
		} else {
			ref := localBranchRef(repo, target.source)
			if ref == "" {
				return "Select a local branch"
			}
			if id == deleteBranch && strings.TrimPrefix(ref, "refs/heads/") == repo.CurrentBranch {
				return "Cannot delete the current branch"
			}
			for _, w := range repo.Worktrees {
				if w.Branch == ref && w.Path != repo.Root {
					return "The branch is checked out in another worktree"
				}
			}
		}
	case addRemote, editRemote, removeRemote:
		if id != addRemote && !remoteConfigured(repo, target.source) {
			return "Select a configured remote"
		}
	case newWorktree, showWorktrees, removeWorktree:
		if id == newWorktree && repo.Head == "" {
			return "Create a commit before adding a worktree"
		}
	case stashChanges, unstashChanges, dropStash:
		if repo.ResolvingConflicts() || len(repo.Conflicts) > 0 {
			return "Finish the current Git operation first"
		}
		if repo.Head == "" {
			return "Create a commit before using Stash"
		}
		if id != stashChanges && len(repo.Stashes) == 0 {
			return "No stashes"
		}
	case resetRevision:
		if len(repo.MergeHeads) > 0 || repo.Rebase.Active() || repo.Sequencer.Active() {
			return "Finish the current Git operation first"
		}
		if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" {
			return "Check out a local branch with a commit"
		}
		if _, ok := a.operationCommit(target); !ok {
			return "Select a commit in the Git Log"
		}
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
				return "Commit or shelve changes before continuing"
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
	case checkoutRebase, checkoutUpdate:
		ref := localBranchRef(repo, target.source)
		if ref == "" || ref == "refs/heads/"+repo.CurrentBranch || repo.Head == "" || repo.CurrentBranch == "Detached HEAD" {
			return "Select a different local branch and check out a base branch"
		}
		if repo.ResolvingConflicts() || len(repo.Staged) > 0 || slices.ContainsFunc(repo.Working, func(f domain.File) bool { return f.Status != "?" }) || a.updateDirtyBuffer(repo.Root) {
			return "Finish the current operation and save or shelve changes before switching branches"
		}
		if id == checkoutUpdate {
			upstream := repo.Upstreams[strings.TrimPrefix(ref, "refs/heads/")]
			if upstream.Branch == "" || !remoteConfigured(repo, upstream.Remote) {
				return "Select a local branch with a remote upstream"
			}
		}
		for _, worktree := range repo.Worktrees {
			if worktree.Branch == ref {
				return "The selected branch is checked out in a worktree"
			}
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
	s := &repositoryOperationDialog{open: true, root: repo.Root, snapshot: repo}
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
	case shelve, unshelve, renameShelf, deleteShelf, restoreShelf, purgeShelf:
		a.configureShelf(s, id, target)
	case renameBranch, deleteBranch, newTag:
		s.configureRef(repo, id, target)
	case addRemote, editRemote, removeRemote:
		s.configureRemote(id, target)
	case startBisect, bisectGood, bisectBad, bisectSkip, resetBisect, createBisectWorktree:
		s.configureBisect(repo, id)
	case newWorktree, showWorktrees, removeWorktree:
		s.configureWorktree(repo, id, target)
	case stashChanges, unstashChanges, dropStash:
		s.configureStash(repo, id, target)
	case resetRevision:
		commit, _ := a.operationCommit(target)
		s.label, s.detail = "Reset", "Reset branch "+repo.CurrentBranch+" to "+commit.Hash+" · "+commit.Subject
		s.op = domain.RepositoryOperation{Kind: domain.ResetRevision, Source: commit.Hash, Mode: "Mixed"}
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
	case checkoutRebase:
		ref := localBranchRef(repo, target.source)
		s.label = "Checkout and Rebase"
		s.detail = "Check out " + strings.TrimPrefix(ref, "refs/heads/") + " and rebase it onto " + repo.CurrentBranch + " (" + repo.Head + "). The base branch stays unchanged."
		s.op = domain.RepositoryOperation{Kind: domain.CheckoutRebase, Source: ref}
	case checkoutUpdate:
		name := strings.TrimPrefix(localBranchRef(repo, target.source), "refs/heads/")
		upstream := repo.Upstreams[name]
		s.label = "Checkout and Update"
		s.detail = "Check out " + name + ", then fetch " + upstream.Remote + "/" + upstream.Branch + " and fast-forward. If Update stops after checkout, " + name + " stays checked out."
		s.op = domain.RepositoryOperation{Kind: domain.CheckoutUpdate, Branch: name, Remote: upstream.Remote, Target: upstream.Branch}
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
		if a.UpdateRepository != nil {
			a.configureUpdate(s)
		}
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
				a.stashFields(c, s, busy)
				a.updateFields(c, p, s, busy)
				a.shelfFields(c, p, s, busy)
				a.worktreeFields(c, s, busy)
				bisectFields(c, s, busy)
				refFields(c, s, busy)
				if op.Kind == domain.ResetRevision {
					ui.Text(c, op.Source)
					ui.Text(c, repo.CurrentBranch)
					ui.Select(c, &op.Mode, []string{"Soft", "Mixed", "Hard"}).Label("Reset mode").Disabled(busy)
					ui.Text(c, resetImpact(op.Mode)).FillWidth()
					if op.Mode == "Hard" {
						ui.Checkbox(c, &s.acknowledged, "I understand Hard reset discards tracked changes").Disabled(busy)
					}
				}
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
					if ui.Select(c, &s.mainline, s.mainlines).Label("Choose mainline parent").Disabled(busy).Changed() && s.historyChanges != nil {
						a.pending = func() { a.loadHistoryComparison(s) }
					}
				}
				a.historyOperationFields(c, s)
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
				needsBranch := op.Kind == domain.CreateBranch || op.Kind == domain.RenameBranch || (op.Kind == domain.CheckoutBranch && strings.HasPrefix(op.Source, "refs/remotes/"))
				needsTarget := op.Kind == domain.PullBranch || op.Kind == domain.PushBranch
				disabled := busy || a.repositoryWrite != "" || a.repositoryRead != nil || repo.Root != s.root || repo.IndexVersion == "" ||
					a.updateDisabled(s) != "" ||
					refFieldsDisabled(s) ||
					historyComparisonDisabled(s) ||
					shelfFieldsDisabled(s) ||
					a.shelfApplyReason(s) != "" ||
					worktreeCommandDisabled(s) ||
					bisectNeedsRange(*op) ||
					((op.Kind == domain.ApplyStash || op.Kind == domain.DropStash) && len(s.choices) == 0) ||
					(op.Kind == domain.ResetRevision && op.Mode == "Hard" && !s.acknowledged) ||
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
			stopRead(&s.shelfHunkRead)
			if s.historyChanges != nil {
				stopRead(&s.historyChanges.request)
			}
			a.repositoryOperation = nil
		}
	}
}

func (a *Workspace) applyRepositoryOperation(s *repositoryOperationDialog) {
	if (s.op.Kind == domain.CheckoutRebase || s.op.Kind == domain.CheckoutUpdate) && (a.Model.Active().Repository.Root != s.root || a.updateDirtyBuffer(s.root)) {
		s.message = "Return to the original repository and save unsaved source documents before switching branches"
		return
	}
	if s.op.Kind == domain.ResetRevision && a.updateDirtyBuffer(s.root) {
		s.message = "Save or discard unsaved source documents before resetting"
		return
	}
	if reason := a.shelfApplyReason(s); reason != "" {
		s.message = reason
		return
	}
	if err := a.Layout.VersionControl.Git.SafetyPolicy().CheckOperation(s.snapshot, s.op); err != nil {
		s.message = err.Error()
		return
	}
	if s.op.Kind == domain.UpdateTrackedBranch && s.op.UpdateMethod != "" && a.UpdateRepository != nil {
		a.applyBranchUpdate(s)
		return
	}
	switch s.op.Kind {
	case domain.ContinueMerge, domain.AbortMerge, domain.ContinueRebase, domain.SkipRebase, domain.AbortRebase:
		if a.updateDirtyBuffer(s.root) {
			s.message = "Save or discard unsaved source documents before continuing or aborting"
			return
		}
	}
	if historyComparisonDisabled(s) {
		return
	}
	if isHistoryWrite(s.op.Kind) {
		if a.Model.Active().Repository.Root != s.root {
			s.message = "Return to the target repository"
			return
		}
		if reason := a.historyDirtyReason(s.op.Kind, s.op.Paths); reason != "" {
			s.message = reason
			return
		}
	}
	if s.op.Kind == domain.SaveShelf || s.op.Kind == domain.ApplyShelf {
		for _, path := range s.op.Paths {
			if a.hasDirtySource(path) {
				s.message = "Save or discard the unsaved source buffer first"
				return
			}
		}
	}
	if s.op.Kind == domain.RemoveWorktree && s.management {
		entry, _ := s.selectedWorktree()
		a.repositoryOperation = nil
		a.run(removeWorktree, commandTarget{source: entry.Path})
		return
	}
	restoreEditors := func() {}
	if s.op.Kind == domain.SaveShelf || s.op.Kind == domain.ApplyShelf || s.op.Kind == domain.CheckoutRebase || s.op.Kind == domain.CheckoutUpdate {
		restoreEditors = a.lockShelfEditors()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.message = cancel, ""
	op := s.op
	if op.Kind == domain.CheckoutRebase {
		op.ConfirmRewrite = true
	}
	if op.Kind == domain.RemoveWorktree {
		entry, _ := s.selectedWorktree()
		op.Path = entry.Path
	}
	if op.Kind == domain.ApplyStash || op.Kind == domain.DropStash {
		entry, ok := s.selectedStash()
		if !ok {
			cancel()
			s.cancel = nil
			return
		}
		op.Source, op.Target = entry.OID, entry.Selector
	}
	if len(s.mainlines) > 0 {
		op.Mainline = slices.Index(s.mainlines, s.mainline)
	}
	snapshot := s.snapshot
	if op.Kind == domain.StartMerge || op.Kind == domain.StartRebase {
		op.Source = operationRef(snapshot, s.mergeSource)
	}
	op.Branch, op.Target = strings.TrimSpace(op.Branch), strings.TrimSpace(op.Target)
	a.beginRepositoryWrite(s.root, s.label+"…", cancel)
	a.OperateRepository(ctx, snapshot, op, func(repo domain.Repository, err, refreshErr error) {
		cancel()
		restoreEditors()
		s.cancel = nil
		message := s.label + " completed"
		paused := op.Kind == domain.StartMerge && refreshErr == nil && len(repo.MergeHeads) > 0
		rebasePaused := (op.Kind == domain.StartRebase || op.Kind == domain.CheckoutRebase || op.Kind == domain.ContinueRebase || op.Kind == domain.SkipRebase) &&
			refreshErr == nil && repo.Rebase.Active() && repo.Rebase != snapshot.Rebase
		sequencePaused := (op.Kind == domain.StartCherryPick || op.Kind == domain.StartRevert || op.Kind == domain.CherryPickSelected || op.Kind == domain.RevertSelected || op.Kind == domain.ContinueSequence || op.Kind == domain.SkipSequence) &&
			refreshErr == nil && repo.Sequencer.Active() && repo.Sequencer != snapshot.Sequencer
		unchanged := refreshErr == nil && errors.Is(err, domain.ErrRevertUnchanged)
		stashConflicts := (op.Kind == domain.ApplyStash || op.Kind == domain.ApplyShelf) && refreshErr == nil && len(repo.Conflicts) > 0
		if stashConflicts {
			message = "Backup applied with conflicts. Original entry retained. Resolve and stage the unmerged files, or explicitly reset to HEAD to discard the applied changes."
			if op.Kind == domain.ApplyShelf {
				message = "Shelf applied with conflicts. Original entry retained. Resolve and stage the unmerged files, or explicitly reset to HEAD to discard the applied changes."
			}
			if err != nil {
				message += " · " + err.Error()
			}
			s.open = false
		} else if op.Kind >= domain.StartBisect && op.Kind <= domain.ResetBisect && refreshErr == nil && (repo.Bisect.FirstBad != "" || repo.Bisect.Ambiguous) {
			message = bisectSummary(repo.Bisect)
			if err != nil {
				message += " · " + err.Error()
			}
			s.open = false
		} else if unchanged {
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
		switch op.Kind {
		case domain.StartMerge, domain.ContinueMerge, domain.AbortMerge, domain.StartRebase, domain.CheckoutRebase, domain.CheckoutUpdate, domain.ContinueRebase, domain.SkipRebase, domain.AbortRebase:
			a.refreshHistoricalWriteViews(s.root, op)
		}
		if refreshErr == nil && (isHistoryWrite(op.Kind) || op.Kind == domain.ContinueSequence || op.Kind == domain.SkipSequence || op.Kind == domain.AbortSequence) {
			a.refreshHistoricalWriteViews(s.root, op)
		}
		if op.Kind == domain.ApplyShelf {
			a.refreshHistoricalWriteViews(s.root, op)
		} else if refreshErr == nil && op.Kind == domain.SaveShelf {
			for _, path := range op.Paths {
				a.refreshOpenFileViews(s.root, path)
			}
		}
		if err == nil && refreshErr == nil && op.Kind == domain.RemoveWorktree {
			a.invalidateRemovedWorktree(op.Path)
		}
		if (paused || rebasePaused || sequencePaused || stashConflicts) && a.Model.Active().Repository.Root == s.root {
			a.run(showCommit, commandTarget{})
		}
	})
}
