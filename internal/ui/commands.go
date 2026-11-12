package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

type commandID string

const (
	selectRepo         commandID = "workspace.select-repository"
	selectBranch       commandID = "mock.inspect-branch"
	selectCommit       commandID = "mock.inspect-commit"
	showSearch         commandID = "workspace.search-everywhere"
	showSearchActions  commandID = "workspace.search-actions"
	goToFile           commandID = "workspace.go-to-file"
	showRecentFiles    commandID = "workspace.recent-files"
	focusProject       commandID = "workspace.focus-project"
	toggleMainMenu     commandID = "workspace.toggle-main-menu"
	toggleBranches     commandID = "workspace.toggle-branches"
	toggleDetails      commandID = "workspace.toggle-details"
	showLoading        commandID = "mock.history-loading"
	showError          commandID = "mock.history-error"
	retryHistory       commandID = "mock.history-retry"
	switchRepo         commandID = "workspace.switch-repository"
	toggleGit          commandID = "workspace.toggle-git"
	toggleCommit       commandID = "workspace.toggle-commit"
	toggleTheme        commandID = "workspace.toggle-theme"
	closeTab           commandID = "editor.close-tab"
	openDiff           commandID = "editor.open-diff"
	openCode           commandID = "editor.open-code"
	fetch              commandID = "git.fetch"
	pull               commandID = "git.pull"
	push               commandID = "git.push"
	stageChanges       commandID = "git.stage-changes"
	unstageChanges     commandID = "git.unstage-changes"
	commitMock         commandID = "mock.commit-preview"
	commitRepository   commandID = "git.commit"
	commitAndPush      commandID = "git.commit-and-push"
	continueMerge      commandID = "git.continue-merge"
	abortMerge         commandID = "git.abort-merge"
	continueRebase     commandID = "git.continue-rebase"
	skipRebase         commandID = "git.skip-rebase"
	abortRebase        commandID = "git.abort-rebase"
	continueSequence   commandID = "git.continue-sequence"
	skipSequence       commandID = "git.skip-sequence"
	abortSequence      commandID = "git.abort-sequence"
	acceptCurrent      commandID = "git.accept-current"
	acceptIncoming     commandID = "git.accept-incoming"
	markResolved       commandID = "git.mark-resolved"
	openMergeEditor    commandID = "merge.open-editor"
	previousConflict   commandID = "merge.previous-conflict"
	nextConflict       commandID = "merge.next-conflict"
	applyCurrentBlock  commandID = "merge.apply-current-block"
	applyIncomingBlock commandID = "merge.apply-incoming-block"
	saveMergeResult    commandID = "merge.save-result"
	closeMergeEditor   commandID = "merge.close-editor"
)

type commandTarget struct {
	index   int
	tabID   string
	preview bool
	source  string
	file    domain.File
}

var commands = map[commandID]string{
	createBisectWorktree: "Create Candidate Worktree…",
	startBisect:          "Start Bisect…", bisectGood: "Mark Good", bisectBad: "Mark Bad", bisectSkip: "Skip Candidate", resetBisect: "End Bisect…", showBisectCandidate: "Show Candidate in Log",
	dropStash: "Drop Stash…", removeWorktree: "Remove Worktree…", openWorktree: "Open Worktree",
	selectBranch: "Inspect Branch", selectCommit: "Inspect Commit",
	showSearch: "Search Everywhere", showSearchActions: "Search Actions", toggleBranches: "Toggle Branches", toggleDetails: "Toggle Commit Details",
	goToFile: "Go to File", showRecentFiles: "Recent Files", focusProject: "Project Navigation",
	toggleMainMenu:      "Main menu",
	expandBranchFolders: "Expand Branch Folders", collapseBranchFolders: "Collapse Branch Folders",
	showLoading: "Show mock loading", showError: "Show mock error", retryHistory: "Retry history",
	switchRepo: "Switch repository", toggleGit: "Toggle Git", toggleCommit: "Toggle Commit",
	toggleTheme: "Switch theme", closeTab: "Close active tab", openDiff: "Show Diff in a New Tab",
	openCode: "Open in Editor", fetch: "Fetch", pull: "Pull", push: "Push", commitMock: "Preview Commit", commitRepository: "Commit",
	commitAndPush: "Commit and Push…", stageChanges: "Stage Changes", unstageChanges: "Unstage Changes",
	continueMerge: "Continue Merge", abortMerge: "Abort Merge", acceptCurrent: "Accept Current", acceptIncoming: "Accept Incoming", markResolved: "Mark Resolved",
	continueRebase: "Continue Rebase", skipRebase: "Skip Commit", abortRebase: "Abort Rebase",
	continueSequence: "Continue Operation", skipSequence: "Skip Current Commit", abortSequence: "Abort Operation",
	openMergeEditor: "Resolve in Merge Editor", previousConflict: "Previous Conflict", nextConflict: "Next Conflict",
	applyCurrentBlock: "Apply Current Block", applyIncomingBlock: "Apply Incoming Block", saveMergeResult: "Apply Changes",
	closeMergeEditor: "Close Merge Editor",
}

func (a *Workspace) disabledFor(id commandID, target commandTarget) string {
	switch id {
	case openMergeEditor, previousConflict, nextConflict, applyCurrentBlock, applyIncomingBlock, saveMergeResult, closeMergeEditor:
		return a.mergeEditorDisabled(id, target)
	}
	real := a.Model.Active().Repository.Root != ""
	if !real {
		switch id {
		case dropStash, removeWorktree, openWorktree, startBisect, bisectGood, bisectBad, bisectSkip, resetBisect, showBisectCandidate, createBisectWorktree:
			return "Open a local Git repository"
		}
	}
	if id == openLocalRepository && (a.ReadRepository == nil || a.PickRepositoryDirectory == nil || a.pickingRepository) {
		return "Directory picker is unavailable"
	}
	if id == fileHistory && (!real || target.file.Path == "") {
		return "Select a file in a local Git repository"
	}
	if real {
		switch id {
		case showBisectCandidate:
			repo := a.Model.Active().Repository
			if !repo.Bisect.Active() || !slices.ContainsFunc(repo.History, func(c domain.Commit) bool { return c.Hash == repo.Bisect.Candidate }) {
				return "Candidate is outside the loaded log. Inspect its OID using Git"
			}
			return ""
		case openWorktree:
			if a.ReadRepository == nil || a.repositoryWrite != "" {
				return "Wait for Git or connect the repository reader"
			}
			if !slices.ContainsFunc(a.Model.Active().Repository.Worktrees, func(w domain.Worktree) bool { return w.Path == target.source && !w.Bare && !w.Prunable }) {
				return "Select an available worktree"
			}
			return ""
		case newBranch, checkoutBranch, fetch, pull, push, updateBranch, updateProject,
			mergeBranch, continueMerge, abortMerge, rebaseBranch, continueRebase, skipRebase, abortRebase, acceptCurrent, acceptIncoming, markResolved,
			cherryPick, revertCommit, continueSequence, skipSequence, abortSequence, resetRevision, stashChanges, unstashChanges, dropStash,
			newWorktree, showWorktrees, removeWorktree, startBisect, bisectGood, bisectBad, bisectSkip, resetBisect, createBisectWorktree:
			return a.repositoryOperationDisabled(id, target)
		case commitMock, previewFileCommit, showIgnored, newScratch:
			return "Available in a later Git phase"
		case stageChanges, unstageChanges:
			return a.indexDisabled(id == stageChanges, target)
		case commitRepository:
			return a.commitDisabled()
		case commitAndPush:
			if a.OperateRepository == nil || len(a.Model.Active().Repository.Remotes) == 0 {
				return "Configure a remote before committing and pushing"
			}
			return a.commitDisabled()
		case refreshChanges, refreshLog, retryHistory:
			if a.repositoryWrite == a.Model.Active().Repository.Root {
				return "Wait for the Git operation to finish"
			}
		case branchDiff, compareCurrentBranch, showChangesToParents:
			return "This comparison is unavailable for local repositories"
		case showLoading, showError:
			return "Available in the sample workspace"
		case previewDiff, openDiff:
			if target.source == "project" {
				if source, _ := a.projectChange(target.file.Path); source == "" {
					return "This file has no local changes"
				}
			}
		case openRevision:
			if target.file.Status == "D" {
				return "The file was deleted at this revision"
			}
		}
	}
	if _, ok := deferredCommands[id]; ok {
		if id == updateBranch && (strings.HasPrefix(target.source, "origin/") || strings.HasPrefix(target.source, "refs/tags/")) {
			return "Select a local tracking branch"
		}
		return "Available in a later Git phase"
	}
	switch id {
	case unshelve, restoreShelf, renameShelf, deleteShelf:
		return a.shelfDisabled(id, target)
	case previewDiff, openDiff, openCode, compareLocal, compareBefore, openRevision, copyPath, openRightSplit, previewFileCommit, locateChange:
		if target.file.Path == "" {
			return "Select a file"
		}
	case stageChanges:
		if target.file.Status == "I" {
			return "Ignored mock files cannot be staged"
		}
	case branchDiff, compareCurrentBranch, missingBranchCommits:
		if _, ok := branchTip(a.Model.Active().Repository.History, target.source); !ok {
			return "No mock revision for this branch"
		}
	case historyToRevision:
		if target.file.Path == "" {
			return "Select a historical file"
		}
	case fetch, pull, push:
		return "Available when Git operations are enabled"
	case commitMock:
		if strings.TrimSpace(a.Model.Active().Message) == "" {
			return "Enter a commit message"
		}
		if len(a.Model.Active().Repository.Staged) == 0 {
			return "No staged changes"
		}
	case closeTab, closeOthers, closeAllTabs, closeUnmodified, closeLeft, closeRight, closeReadOnly,
		pinTab, reopenTab, splitRight, splitDown, moveRight, moveDown:
		return a.tabDisabled(id, target)
	}
	return ""
}

func (a *Workspace) run(id commandID, target commandTarget) {
	if a.disabledFor(id, target) != "" {
		return
	}
	switch id {
	case openMergeEditor:
		a.loadMergeEditor(target.file.Path)
		return
	case previousConflict, nextConflict, applyCurrentBlock, applyIncomingBlock, saveMergeResult, closeMergeEditor:
		a.runMergeEditor(id)
		return
	}
	if a.Model.Active().Repository.Root != "" {
		switch id {
		case showBisectCandidate:
			a.selectBisectCandidate()
			return
		case openWorktree:
			a.loadRepository(target.source, false)
			return
		case newBranch, checkoutBranch, fetch, pull, push, updateBranch, updateProject,
			mergeBranch, continueMerge, abortMerge, rebaseBranch, continueRebase, skipRebase, abortRebase, acceptCurrent, acceptIncoming, markResolved,
			cherryPick, revertCommit, continueSequence, skipSequence, abortSequence, resetRevision, stashChanges, unstashChanges, dropStash,
			newWorktree, showWorktrees, removeWorktree, startBisect, bisectGood, bisectBad, bisectSkip, resetBisect, createBisectWorktree:
			a.openRepositoryOperation(id, target)
			return
		case commitRepository, commitAndPush:
			a.requestCommit(id == commitAndPush)
			return
		case stageChanges, unstageChanges:
			a.requestIndexChange(id == stageChanges, target)
			return
		case previewDiff, openDiff, openCode, openRightSplit, openRevision, compareLocal, compareBefore:
			if a.ReadRepositoryFile != nil {
				a.readFileForCommand(id, target)
			}
			return
		case refreshChanges, refreshLog, retryHistory:
			if a.ReadRepository != nil {
				a.loadRepository(a.Model.Active().Repository.Root, true)
			}
			return
		}
	}
	switch id {
	case newScratch:
		r := a.repo()
		r.projectScratch++
		a.openCode("scratch", domain.File{Path: fmt.Sprintf(".scratch/scratch-%d.txt", r.projectScratch)})
	case unshelve:
		a.repo().shelfApply = &target
	case renameShelf:
		a.repo().shelfRename = &target
		a.repo().shelfName = a.Model.Active().Repository.Shelf[target.index].Title
	case deleteShelf, restoreShelf:
		entry := &a.Model.Active().Repository.Shelf[target.index]
		entry.Deleted = id == deleteShelf
		a.repo().shelfIndex = -1
		a.record(commandLabel(id) + " · " + entry.Title)
	case clearConsole:
		a.repo().console = nil
	case expandHistorical, collapseHistorical:
		a.repo().files.resetOpen(id == expandHistorical)
	case expandBranchFolders, collapseBranchFolders:
		a.repo().branchOutline().resetOpen(id == expandBranchFolders)
	case showChangesToParents:
		a.repo().hideChangesToParents = !a.repo().hideChangesToParents
		a.Model.Active().Historical = ""
		a.repo().fileIndex = -1
	case clearLogFilters:
		r := a.repo()
		r.logExcludeRef = ""
		r.logHideRoot = false
		r.logQuery, r.logAuthor, r.logRef, r.logPath, r.logHistoryPath = "", "", "", "", ""
		r.logDate = logDateRange{}
	case groupDirectory, groupRepository, groupModule:
		a.repo().commitGroup = id
	case showIgnored:
		r := a.repo()
		r.showIgnored = !r.showIgnored
		s := a.Model.Active()
		if !r.showIgnored && s.Working == ".idea/workspace.xml" {
			s.Working = ""
			r.workingChosen.Remove(".idea/workspace.xml")
		}
	case locateChange:
		a.repo().locateSelected = true
	case expandChanges, collapseChanges:
		r := a.repo()
		r.working.resetOpen(id == expandChanges)
		r.staged.resetOpen(id == expandChanges)
	case previewDiff:
		entry := diffTab(target.source, target.file)
		a.repo().preview = &entry
	case closePreview:
		if a.repositoryFileRead != nil {
			a.status = "File preview canceled"
		}
		stopRead(&a.repositoryFileRead)
		r := a.repo()
		r.preview = nil
		r.emptyEditorFocus = len(r.tabs) == 0
	case previewFileCommit:
		a.record("Mock commit preview for " + target.file.Path)
	case refreshChanges:
		a.record("Refreshed mock changes")
	case openRightSplit:
		r := a.repo()
		previous := ""
		if len(r.tabs) != 0 {
			previous = r.tabs[r.active].id
		}
		if !a.activate(target.tabID) {
			a.run(openCode, target)
		} else {
			r.preview = nil
		}
		a.splitTab(r.tabs[r.active].id, false, false)
		a.activate(previous)
	case compareLocal, compareBefore:
		file := target.file
		file.Old = file.New
		if id == compareBefore {
			file.Old = target.file.Old
		}
		file.New = a.localFile(file).New
		a.run(previewDiff, commandTarget{source: "local vs " + target.source, file: file})
	case branchDiff:
		revision, _ := branchTip(a.Model.Active().Repository.History, target.source)
		if len(revision.Files) > 0 {
			a.run(compareLocal, commandTarget{source: displayBranch(target.source), file: revision.Files[0]})
		}
	case compareCurrentBranch:
		repo := a.Model.Active().Repository
		left, _ := branchTip(repo.History, target.source)
		right, ok := branchTip(repo.History, repo.CurrentBranch)
		if !ok || len(left.Files) == 0 {
			return
		}
		file := left.Files[0]
		file.Old = file.New
		file.New = ""
		if other, ok := matchFile(right.Files, file.Path); ok {
			file.New = other.New
		}
		a.run(previewDiff, commandTarget{source: displayBranch(target.source) + " vs " + repo.CurrentBranch, file: file})
	case missingBranchCommits, historyToRevision, fileHistory:
		a.run(clearLogFilters, commandTarget{})
		r := a.repo()
		if id != fileHistory {
			r.logRef = target.source
		}
		if id == missingBranchCommits {
			r.logExcludeRef = a.Model.Active().Repository.CurrentBranch
		} else if a.Model.Active().Repository.Root != "" {
			r.logHistoryPath = target.file.Path
		} else {
			r.logPath = target.file.Path
		}
		a.run(showLog, commandTarget{})
	case openRevision:
		a.openCode("revision:"+target.source, target.file)
	case copyPath, copyHash:
		value := target.file.Path
		if id == copyHash {
			value = target.source
		}
		a.clipboard = &value
	case goParent, goChild:
		a.run(clearLogFilters, commandTarget{})
		r := a.repo()
		a.run(selectCommit, target)
		r.logScroll = true
	case openShelf, openProject, showCommit:
		a.repo().toolPage = commandLabel(id)
		a.Layout.HideCommit = false
	case toggleConsole, showLog:
		a.repo().consoleOpen = id == toggleConsole
		a.Layout.HideGit = false
	case showSettings:
		a.openSettings()
	case refreshLog:
		a.Model.Active().HistoryState = application.HistoryReady
		a.record("Refreshed mock history")
	case showSearch:
		a.openSearch(searchAll)
	case showSearchActions:
		a.openSearch(searchActions)
	case toggleMainMenu:
		a.mainMenuExpanded = !a.mainMenuExpanded
	case manageRemotes:
		a.openRemotes()
	case initializeRepository:
		a.openRepositoryInit()
	case openLocalRepository:
		if a.ReadRepository != nil && a.PickRepositoryDirectory != nil {
			a.chooseRepository()
		}
	case openRemoteRepository:
		a.browserURL = target.source
	case copyRemoteRepository:
		a.clipboard = &target.source
	case goToFile:
		a.openSearch(searchFiles)
	case showRecentFiles:
		a.openSearch(searchRecent)
	case focusProject:
		a.run(openProject, commandTarget{})
		a.repo().projectFocus = true
	case toggleBranches:
		a.Layout.HideBranches = !a.Layout.HideBranches
	case toggleDetails:
		a.Layout.HideDetails = !a.Layout.HideDetails
	case showLoading:
		a.Model.Active().HistoryState = application.HistoryLoading
	case showError:
		a.Model.Active().HistoryState = application.HistoryError
	case retryHistory:
		a.Model.Active().HistoryState = application.HistoryReady
	case switchRepo:
		a.run(selectRepo, commandTarget{index: (a.Model.Index + 1) % len(a.Model.Sessions)})
	case selectRepo:
		a.indexConfirmation = nil
		a.commitConfirmation = nil
		if a.repositoryOperation != nil && a.repositoryOperation.cancel == nil {
			a.repositoryOperation = nil
		}
		stopRead(&a.repositoryRead)
		stopRead(&a.repositoryFileRead)
		a.Model.Switch(target.index)
		a.status = "Switched to " + a.Model.Active().Repository.Name
	case selectBranch:
		a.Model.Active().Branch = target.source
		a.status = "Selected branch " + target.source
	case selectCommit:
		s, r := a.Model.Active(), a.repo()
		if s.Commit != target.index {
			stopRead(&a.repositoryFileRead)
			s.Commit = target.index
			s.Historical, r.fileIndex = "", -1
			r.files.List = ui.ListState{}
		}
	case toggleGit:
		a.Layout.HideGit = !a.Layout.HideGit
	case toggleCommit:
		a.Layout.HideCommit = !a.Layout.HideCommit
	case toggleTheme:
		a.Layout.Dark = !a.darkTheme()
		a.Layout.Appearance.FollowSystem = false
	case closeTab, closeOthers, closeAllTabs, closeUnmodified, closeLeft, closeRight, closeReadOnly:
		a.closeTabs(id, target)
	case pinTab:
		if target.preview {
			target.tabID = a.promotePreview()
		}
		r := a.repo()
		entry := &r.tabs[r.tabIndex(target.tabID)]
		entry.pinned = !entry.pinned
	case reopenTab:
		a.reopenClosedTab()
	case splitRight, splitDown, moveRight, moveDown:
		if target.preview {
			target.tabID = a.promotePreview()
		}
		a.splitTab(target.tabID, id == splitDown || id == moveDown, id == moveRight || id == moveDown)
	case closeSplit:
		a.repo().split = nil
	case openBranches:
		a.Layout.HideBranches = false
		a.run(showLog, commandTarget{})
	case openDiff:
		a.openDiff(target.source, target.file)
	case openCode:
		if target.source != "working" && target.source != "staged" && target.source != "project" {
			if local, ok := matchFile(projectFiles(a.Model.Active().Repository), target.file.Path); ok {
				a.openCode("project", local)
				return
			}
		}
		a.openCode(target.source, target.file)
	case stageChanges, unstageChanges:
		a.moveChanges(id == stageChanges, target.file.Path, target.source != "editor")
	case commitMock:
		a.previewCommit()
	}
}

func (a *Workspace) previewCommit() {
	session := a.Model.Active()
	prefs := a.Layout.VersionControl.Commit
	if warning := prefs.Inspect(session.Message); warning != "" {
		a.record("Mock commit inspection: " + warning)
		return
	}
	if prefs.CheckTODO {
		for _, file := range session.Repository.Staged {
			if strings.Contains(file.New, "TODO") && !strings.Contains(file.Old, "TODO") {
				a.record("Mock commit inspection: TODO in " + file.Path)
				return
			}
		}
	}
	a.record("Mock commit preview ready • staged changes preserved")
	if prefs.ClearMessage {
		session.Message = ""
	}
}

func (a *Workspace) button(c *ui.Context, id commandID, label string) {
	reason := a.disabledFor(id, commandTarget{})
	tip := commandLabel(id)
	if reason != "" {
		tip = reason
	}
	accessLabel := commandLabel(id)
	if id == continueSequence || id == abortSequence {
		accessLabel = label
		tip = label
		if reason != "" {
			tip = reason
		}
	}
	ui.Button(c.Key(string(id)), label).Height(controlHeight).Label(accessLabel).Disabled(reason != "").Tooltip(tip).OnClick(func() {
		a.run(id, commandTarget{})
	})
}

func (a *Workspace) menu(m *ui.Menu, id commandID, target commandTarget) {
	a.menuNamed(m, id, target, commandLabel(id), "")
}

func (a *Workspace) record(message string) {
	a.status = message
	a.repo().addConsole(message)
}

func (r *repoView) addConsole(message string) {
	r.console = append(r.console, message)
	if len(r.console) > 50 {
		r.console = r.console[len(r.console)-50:]
	}
}

func (a *Workspace) localFile(f domain.File) domain.File {
	for _, file := range projectFiles(a.Model.Active().Repository) {
		if file.Path == f.Path {
			return file
		}
	}
	return f
}

func (a *Workspace) moveChanges(stage bool, path string, allowSelection bool) {
	r, s := a.repo(), a.Model.Active()
	chosen, index, selected := &r.stagedChosen, &r.stagedIndex, &s.Staged
	if stage {
		chosen, index, selected = &r.workingChosen, &r.workingIndex, &s.Working
	}
	useSelection := allowSelection && chosen.Has(path)
	count := s.MoveChanges(stage, func(candidate string) bool {
		if useSelection {
			return chosen.Has(candidate)
		}
		return candidate == path
	})
	chosen.Clear()
	*index, *selected = -1, ""
	verb := "unstaged"
	if stage {
		verb = "staged"
	}
	a.record(fmt.Sprintf("Mock %s %d file(s)", verb, count))
}
