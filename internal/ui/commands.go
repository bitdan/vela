package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

type commandID string

const (
	selectRepo        commandID = "workspace.select-repository"
	selectBranch      commandID = "mock.inspect-branch"
	selectCommit      commandID = "mock.inspect-commit"
	showSearch        commandID = "workspace.search-everywhere"
	showSearchActions commandID = "workspace.search-actions"
	goToFile          commandID = "workspace.go-to-file"
	showRecentFiles   commandID = "workspace.recent-files"
	focusProject      commandID = "workspace.focus-project"
	toggleMainMenu    commandID = "workspace.toggle-main-menu"
	toggleBranches    commandID = "workspace.toggle-branches"
	toggleDetails     commandID = "workspace.toggle-details"
	showLoading       commandID = "mock.history-loading"
	showError         commandID = "mock.history-error"
	retryHistory      commandID = "mock.history-retry"
	switchRepo        commandID = "workspace.switch-repository"
	toggleGit         commandID = "workspace.toggle-git"
	toggleCommit      commandID = "workspace.toggle-commit"
	toggleTheme       commandID = "workspace.toggle-theme"
	closeTab          commandID = "editor.close-tab"
	openDiff          commandID = "editor.open-diff"
	openCode          commandID = "editor.open-code"
	fetch             commandID = "git.fetch"
	pull              commandID = "git.pull"
	push              commandID = "git.push"
	stageChanges      commandID = "mock.stage-changes"
	unstageChanges    commandID = "mock.unstage-changes"
	commitMock        commandID = "mock.commit-preview"
)

type commandTarget struct {
	index   int
	tabID   string
	preview bool
	source  string
	file    domain.File
}

var commands = map[commandID]string{
	selectBranch: "Inspect Branch", selectCommit: "Inspect Commit",
	showSearch: "Search Everywhere", showSearchActions: "Search Actions", toggleBranches: "Toggle Branches", toggleDetails: "Toggle Commit Details",
	goToFile: "Go to File", showRecentFiles: "Recent Files", focusProject: "Project Navigation",
	toggleMainMenu:      "Main menu",
	expandBranchFolders: "Expand Branch Folders", collapseBranchFolders: "Collapse Branch Folders",
	showLoading: "Show mock loading", showError: "Show mock error", retryHistory: "Retry history",
	switchRepo: "Switch repository", toggleGit: "Toggle Git", toggleCommit: "Toggle Commit",
	toggleTheme: "Switch theme", closeTab: "Close active tab", openDiff: "Show Diff in a New Tab",
	openCode: "Open in Editor", fetch: "Fetch", pull: "Pull", push: "Push", commitMock: "Preview Commit", stageChanges: "Stage Changes", unstageChanges: "Unstage Changes",
}

func (a *Workspace) disabledFor(id commandID, target commandTarget) string {
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
		r.logQuery, r.logAuthor, r.logRef, r.logPath = "", "", "", ""
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
	case missingBranchCommits, historyToRevision:
		a.run(clearLogFilters, commandTarget{})
		r := a.repo()
		r.logRef = target.source
		if id == missingBranchCommits {
			r.logExcludeRef = a.Model.Active().Repository.CurrentBranch
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
		a.Model.Switch(target.index)
		a.status = "Switched to " + a.Model.Active().Repository.Name
	case selectBranch:
		a.Model.Active().Branch = target.source
		a.status = "Selected branch " + target.source
	case selectCommit:
		s, r := a.Model.Active(), a.repo()
		if s.Commit != target.index {
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
	ui.Button(c.Key(string(id)), label).Height(controlHeight).Label(commandLabel(id)).Disabled(reason != "").Tooltip(tip).OnClick(func() {
		a.run(id, commandTarget{})
	})
}

func (a *Workspace) menu(m *ui.Menu, id commandID, target commandTarget) {
	a.menuNamed(m, id, target, commandLabel(id), "")
}

func (a *Workspace) record(message string) {
	a.status = message
	r := a.repo()
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
