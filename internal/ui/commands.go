package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

type commandID string

const (
	cancelGitTask      commandID = "workspace.cancel-git-task"
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
	restoreLayout      commandID = "workspace.restore-default-layout"
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
	shelfID, shelfOID string
	remote            domain.Remote
	index             int
	tabID             string
	preview           bool
	source            string
	file              domain.File
	editorState       *editor.State
}

var commands = map[commandID]string{
	importPatch: "Import Patch…", previewImportedPatch: "Preview Imported Patch", applyImportedPatch: "Apply Imported Patch",
	replaceInProject: "Replace in Files…", applyProjectReplace: "Replace Previewed Matches",
	closeBranchComparison: "Close Branch Comparison", reloadBranchComparison: "Reload Branch Comparison", loadMoreComparisonCommits: "Load More Missing Commits", cancelBranchComparison: "Cancel Branch Comparison", openComparisonEntry: "Open Branch Comparison Diff", browseComparisonCommit: "Browse Missing Commit",
	openDemo: "Open Demo Workspace", leaveDemo: "Leave Demo Workspace",
	checkoutRevision: "Checkout Revision", showRepositoryRevision: "Show Repository at Revision",
	getRevision: "Get from Revision", cherryPickChanges: "Cherry-Pick Selected Changes", revertChanges: "Revert Selected Changes",
	openRevisionEntry: "Open Historical Entry", restoreRevisionEntry: "Restore Historical File", revisionParent: "Parent Historical Directory", reloadRevisionTree: "Reload Historical Directory", closeRevisionTree: "Close Historical Repository",
	purgeShelf: "Delete Shelf Permanently…", previewShelfIndex: "Show Shelved Index Diff",
	newChangelist: "New Changelist…", editChangelist: "Edit Changelist…", moveChangelist: "Move to Another Changelist…", setActiveChangelist: "Set Active Changelist", removeChangelist: "Remove Changelist…", applyChangelist: "Apply Changelist", cancelChangelist: "Cancel Changelist", filterChangelist: "Filter Working Changelist",
	findInProject: "Find in Files…", startProjectSearch: "Find Project Text", cancelProjectSearch: "Cancel Project Search", openProjectSearchResult: "Open Search Result", closeProjectSearch: "Close Find in Files",
	toggleBookmark: "Toggle Line Bookmark", nextBookmark: "Next Bookmark", previousBookmark: "Previous Bookmark", clearBookmarks: "Clear Bookmarks", goBookmark: "Go to Bookmark",
	fileType: "Override File Type", applyFileType: "Apply File Type", cancelFileType: "Cancel File Type",
	loadMoreCommits: "Load More Commits", cancelLogLoading: "Cancel Log Loading",
	saveCloseSource: "Save and Close Source", discardCloseSource: "Discard and Close Source", confirmReloadSource: "Reload Source File",
	savePatchFile:       "Save Patch File",
	loadMoreFileHistory: "Load More File History", closeFileHistory: "Close File History", openFileHistoryRevision: "Open Historical Version", openFileHistoryDiff: "Show Historical Diff",
	stageHunk: "Stage Hunk…", unstageHunk: "Unstage Hunk…", discardHunk: "Discard Hunk…",
	restoreLayout:        "Restore Default Layout",
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
	if a.patchImport != nil && id != previewImportedPatch && id != applyImportedPatch && id != cancelGitTask && id != selectRepo {
		return "Finish or close the patch import first"
	}
	if id == importPatch || id == previewImportedPatch || id == applyImportedPatch {
		return a.patchImportDisabled(id)
	}
	if a.projectReplaceWriting != "" && a.projectReplaceWriting == a.Model.Active().Repository.Root && id != cancelGitTask && id != selectRepo {
		return "Wait for the project replacement to finish"
	}
	if a.projectFileWriting != "" && a.projectFileWriting == a.Model.Active().Repository.Root && id != cancelGitTask && id != selectRepo {
		return "Wait for the project file operation to finish"
	}
	if isProjectFileCommand(id) {
		return a.projectFileDisabled(id, target)
	}
	if isBranchComparisonCommand(id) && (a.Model.Active().Repository.Root != "" || !isBranchComparisonEntry(id)) {
		return a.branchComparisonDisabled(id, target)
	}
	if id == openDemo || id == leaveDemo {
		if a.repositoryWrite != "" || a.repositoryOperation != nil || a.repositoryClone != nil || a.repositoryInit != nil {
			return "Finish the current repository operation"
		}
		return ""
	}
	if a.welcomeActive() && !welcomeCommandAllowed(id) {
		return "Open a repository or explicitly enter Demo Mode"
	}
	if id == selectRepo && !a.canSelectSession(target.index) {
		return "Choose a real repository or explicitly enter Demo Mode"
	}
	if isAdvancedHistoryCommand(id) {
		return a.advancedHistoryDisabled(id, target)
	}
	if a.Model.Active().Repository.Root != "" && isRealShelfCommand(id) {
		return a.realShelfDisabled(id, target)
	}
	if isChangelistCommand(id) {
		return a.changelistDisabled(id, target)
	}
	if isProjectSearchCommand(id) {
		return a.projectSearchDisabled(id)
	}
	if isEditorMetadataCommand(id) {
		return a.editorMetadataDisabled(id, target)
	}
	if id == loadMoreCommits || id == cancelLogLoading {
		return a.commitLogDisabled(id)
	}
	if id == saveCloseSource || id == discardCloseSource || id == confirmReloadSource {
		return a.sourceDecisionDisabled(id)
	}
	if id == reloadProjectFile && a.Model.Active().Repository.Root != "" {
		return a.sourceReloadDisabled(target)
	}
	if id == savePatchFile {
		return a.patchSaveDisabled()
	}
	if (id == copyPatch || id == createPatch) && a.Model.Active().Repository.Root != "" {
		return a.patchExportDisabled(id, target)
	}
	if id == loadMoreFileHistory || id == closeFileHistory || id == openFileHistoryRevision || id == openFileHistoryDiff {
		return a.fileHistoryDisabled(id)
	}
	if isHunkCommand(id) {
		return a.hunkDisabled(id, target)
	}
	if id == saveSourceFile {
		return a.sourceSaveDisabled(target)
	}
	if id == showBlame {
		if a.Model.Active().Repository.Root == "" || a.ReadRepositoryBlame == nil {
			return "Open a local Git repository"
		}
		if target.file.Path == "" || a.blameRevision(target) == "" {
			return "Select a committed file"
		}
		if target.file.Status == "?" {
			return "Untracked files have no committed Blame"
		}
		return ""
	}
	if id == cloneRepo {
		if a.CloneRepository == nil {
			return "Repository cloning is unavailable"
		}
		if a.repositoryWrite != "" || a.repositoryClone != nil || a.repositoryInit != nil || a.repositoryOperation != nil {
			return "Wait for the current repository operation"
		}
		return ""
	}
	if id == cancelGitTask {
		if (a.repositoryWriteCancel == nil || a.repositoryCancelRequested) && a.patchCopyRead == nil && a.background.refresh == nil && a.background.remote == nil {
			return "No cancellable Git task"
		}
		return ""
	}
	if id == openRecentRepository && (a.ReadRepository == nil || target.source == "") {
		return "Select an available repository"
	}
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
	if id == fileHistory && (a.ReadRepositoryFileHistory == nil || a.blameRevision(target) == "" || a.repositoryWrite != "") {
		return "Connect the file history reader and select a committed file"
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
			addRemote, editRemote, removeRemote,
			renameBranch, deleteBranch, newTag,
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
	case importPatch:
		a.choosePatchSource()
		return
	case previewImportedPatch:
		a.previewPatchImport()
		return
	case applyImportedPatch:
		a.applyPatchImport()
		return
	}
	if isProjectFileCommand(id) {
		a.runProjectFile(id, target)
		return
	}
	if isBranchComparisonCommand(id) && (a.Model.Active().Repository.Root != "" || !isBranchComparisonEntry(id)) {
		a.runBranchComparison(id, target)
		return
	}
	if id == openDemo {
		a.enterDemo()
		return
	}
	if id == leaveDemo {
		a.exitDemo()
		return
	}
	if isAdvancedHistoryCommand(id) {
		a.runAdvancedHistory(id, target)
		return
	}
	if a.Model.Active().Repository.Root != "" && isRealShelfCommand(id) {
		a.runRealShelf(id, target)
		return
	}
	if isChangelistCommand(id) {
		a.runChangelist(id, target)
		return
	}
	if isProjectSearchCommand(id) {
		a.runProjectSearch(id)
		return
	}
	if isEditorMetadataCommand(id) {
		a.runEditorMetadata(id, target)
		return
	}
	switch id {
	case saveCloseSource, discardCloseSource, confirmReloadSource:
		a.runSourceDecision(id)
		return
	case loadMoreCommits:
		a.loadMoreCommits()
		return
	case cancelLogLoading:
		stopRead(&a.logRead)
		return
	case reloadProjectFile:
		a.requestSourceReload(target)
		return
	case openMergeEditor:
		a.loadMergeEditor(target.file.Path)
		return
	case previousConflict, nextConflict, applyCurrentBlock, applyIncomingBlock, saveMergeResult, closeMergeEditor:
		a.runMergeEditor(id)
		return
	}
	if a.Model.Active().Repository.Root != "" {
		switch id {
		case copyPatch, createPatch:
			a.requestPatchExport(id, target)
			return
		case showBlame:
			a.readFileForCommand(id, target)
			return
		case fileHistory:
			a.openFileHistory(target)
			return
		case showBisectCandidate:
			a.selectBisectCandidate()
			return
		case openWorktree:
			a.loadRepository(target.source, false)
			return
		case newBranch, checkoutBranch, fetch, pull, push, updateBranch, updateProject,
			addRemote, editRemote, removeRemote,
			renameBranch, deleteBranch, newTag,
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
	case savePatchFile:
		a.choosePatchDestination()
		return
	case loadMoreFileHistory, closeFileHistory, openFileHistoryRevision, openFileHistoryDiff:
		a.runFileHistory(id)
		return
	case saveSourceFile:
		a.saveSource(target)
		return
	case stageHunk, unstageHunk, discardHunk:
		a.openHunkDialog(id, target)
		return
	case cloneRepo:
		a.repositoryClone = &repositoryCloneDialog{open: true}
		return
	case cancelGitTask:
		if a.repositoryWriteCancel == nil {
			if a.background.refresh != nil || a.background.remote != nil {
				a.stopBackgroundTasks()
				a.background.nextRemote = time.Now().Add(5 * time.Minute)
				a.background.message = "Background task cancellation requested"
				return
			}
			stopRead(&a.patchCopyRead)
			a.status = "Patch copy canceled"
			return
		}
		a.repositoryCancelRequested = true
		a.repositoryWriteCancel()
		a.status = "Cancellation requested · waiting for Git and refresh"
		return
	case openRecentRepository:
		a.loadRepository(target.source, false)
		return
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
		a.queueClipboard(value, "")
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
		a.queueClipboard(target.source, "")
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
	case restoreLayout:
		defaults := application.DefaultLayout()
		a.Layout.Top, a.Layout.Commit, a.Layout.Branches, a.Layout.Log, a.Layout.Files = defaults.Top, defaults.Commit, defaults.Branches, defaults.Log, defaults.Files
		a.Layout.HideCommit, a.Layout.HideGit, a.Layout.HideBranches, a.Layout.HideDetails = false, false, false, false
		a.repo().toolPage, a.repo().consoleOpen = "", false
		a.record("Default layout restored")
	case showLoading:
		a.Model.Active().HistoryState = application.HistoryLoading
	case showError:
		a.Model.Active().HistoryState = application.HistoryError
	case retryHistory:
		a.Model.Active().HistoryState = application.HistoryReady
	case switchRepo:
		for step := 1; step <= len(a.Model.Sessions); step++ {
			index := (a.Model.Index + step) % len(a.Model.Sessions)
			if a.canSelectSession(index) {
				a.run(selectRepo, commandTarget{index: index})
				break
			}
		}
	case selectRepo:
		a.selectRepository(target.index)
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
	case closeTab:
		a.requestCloseSource(target)
	case closeOthers, closeAllTabs, closeUnmodified, closeLeft, closeRight, closeReadOnly:
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
