package ui

import (
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	previewDiff            commandID = "editor.preview-diff"
	closePreview           commandID = "editor.close-preview"
	compareLocal           commandID = "editor.compare-local"
	compareBefore          commandID = "editor.compare-before-local"
	openRevision           commandID = "editor.open-revision"
	copyPath               commandID = "clipboard.copy-path"
	copyHash               commandID = "clipboard.copy-revision"
	goParent               commandID = "history.go-parent"
	goChild                commandID = "history.go-child"
	toggleConsole          commandID = "workspace.toggle-console"
	showLog                commandID = "workspace.show-log"
	showSettings           commandID = "workspace.settings"
	refreshLog             commandID = "mock.refresh-history"
	openShelf              commandID = "workspace.show-shelf"
	openProject            commandID = "workspace.show-project"
	showCommit             commandID = "workspace.show-commit"
	branchDiff             commandID = "mock.compare-working-tree"
	compareCurrentBranch   commandID = "mock.compare-current-branch"
	missingBranchCommits   commandID = "mock.missing-branch-commits"
	checkoutRebase         commandID = "git.checkout-rebase"
	checkoutUpdate         commandID = "git.checkout-update"
	updateBranch           commandID = "git.update-branch"
	checkoutBranch         commandID = "git.checkout-branch"
	newBranch              commandID = "git.new-branch"
	newWorktree            commandID = "git.new-worktree"
	renameBranch           commandID = "git.rename-branch"
	rollback               commandID = "git.rollback"
	shelve                 commandID = "shelf.shelve"
	unshelve               commandID = "shelf.unshelve"
	restoreShelf           commandID = "shelf.restore"
	renameShelf            commandID = "shelf.rename"
	deleteShelf            commandID = "shelf.delete"
	importPatch            commandID = "patch.import"
	createPatch            commandID = "patch.create"
	copyPatch              commandID = "patch.copy"
	cherryPick             commandID = "git.cherry-pick"
	checkoutRevision       commandID = "git.checkout-revision"
	showRepositoryRevision commandID = "git.show-repository-at-revision"
	resetRevision          commandID = "git.reset-revision"
	revertCommit           commandID = "git.revert-commit"
	undoCommit             commandID = "git.undo-commit"
	editCommit             commandID = "git.edit-commit-message"
	fixupCommit            commandID = "git.fixup-commit"
	squashCommit           commandID = "git.squash-commit"
	dropCommit             commandID = "git.drop-commit"
	rebaseFrom             commandID = "git.rebase-from"
	pushUpTo               commandID = "git.push-up-to"
	newTag                 commandID = "git.new-tag"
	browseRevision         commandID = "git.browse-revision"
	revertChanges          commandID = "git.revert-selected-changes"
	cherryPickChanges      commandID = "git.cherry-pick-selected-changes"
	extractChanges         commandID = "git.extract-selected-changes"
	dropChanges            commandID = "git.drop-selected-changes"
	getRevision            commandID = "git.get-from-revision"
	historyToRevision      commandID = "git.history-up-to-here"
	showChangesToParents   commandID = "git.changes-to-parents"
	addIgnore              commandID = "git.add-ignore"
	fileHistory            commandID = "git.file-history"
	openRightSplit         commandID = "editor.open-right-split"
	refreshChanges         commandID = "mock.refresh-changes"
	previewFileCommit      commandID = "mock.preview-file-commit"
	moveChangelist         commandID = "changes.move-changelist"
	newChangelist          commandID = "changes.new-changelist"
	editChangelist         commandID = "changes.edit-changelist"
	addToVCS               commandID = "git.add-file"
	deleteFile             commandID = "file.delete"
	compareRevision        commandID = "git.compare-revision"
	compareBranch          commandID = "git.compare-branch"
	historySelection       commandID = "git.history-selection"
	mergeBranch            commandID = "git.merge"
	rebaseBranch           commandID = "git.rebase"
	stashChanges           commandID = "git.stash"
	unstashChanges         commandID = "git.unstash"
	manageRemotes          commandID = "git.manage-remotes"
	cloneRepo              commandID = "git.clone"
	deleteBranch           commandID = "git.delete-branch"
	newProjectFile         commandID = "project.new-file"
	newProjectDirectory    commandID = "project.new-directory"
	cutProjectFile         commandID = "project.cut-file"
	copyProjectFile        commandID = "project.copy-file"
	pasteProjectFile       commandID = "project.paste-file"
	findInProject          commandID = "project.find-in-files"
	replaceInProject       commandID = "project.replace-in-files"
	reloadProjectFile      commandID = "project.reload-from-disk"
	clearLogFilters        commandID = "history.clear-filters"
	clearConsole           commandID = "mock.console-clear"
	openBranches           commandID = "workspace.show-branches"
)

var extraCommands = map[commandID]string{
	saveSourceFile: "Save Source File",
	showBlame:      "Show Blame",
	cancelGitTask:  "Cancel Git Task",
	addRemote:      "Add Remote", editRemote: "Edit Remote", removeRemote: "Remove Remote",
	newScratch:        "Scratch File",
	historyToRevision: "History Up to Here", expandHistorical: "Expand Historical Files", collapseHistorical: "Collapse Historical Files",
	compareCurrentBranch: "Compare with Current Branch", missingBranchCommits: "Show Missing Commits",
	showChangesToParents: "Show Changes to Parents",
	unshelve:             "Unshelve…", restoreShelf: "Restore", renameShelf: "Rename…", deleteShelf: "Delete…",
	clearLogFilters: "Clear filters", clearConsole: "Clear Console",
	groupDirectory: "Directory", groupRepository: "Repository", groupModule: "Module",
	showIgnored: "Ignored Files", locateChange: "Locate Selected Change", expandChanges: "Expand All", collapseChanges: "Collapse All",
	closeOthers: "Close Other Tabs", closeAllTabs: "Close All Tabs", closeUnmodified: "Close Unmodified Tabs",
	closeLeft: "Close Tabs to the Left", closeRight: "Close Tabs to the Right", closeReadOnly: "Close All Read-Only",
	pinTab: "Pin Tab", reopenTab: "Reopen Closed Tab", splitRight: "Split Right", splitDown: "Split Down",
	previewDiff: "Show Diff", closePreview: "Close Diff Preview", compareLocal: "Compare with Local",
	compareBefore: "Compare Before with Local", openRevision: "Open Repository Version",
	copyPath: "Copy Path/Reference…", copyHash: "Copy Revision Number",
	goParent: "Go to Parent Commit", goChild: "Go to Child Commit",
	toggleConsole: "Console", showLog: "Log", showSettings: "Settings",
	refreshLog: "Refresh", openShelf: "Shelf", openProject: "Project", showCommit: "Commit",
	branchDiff:     "Show Diff with Working Tree",
	openRightSplit: "Open in Right Split", refreshChanges: "Refresh", previewFileCommit: "Commit File…",
	moveRight: "Split and Move Right", moveDown: "Split and Move Down", closeSplit: "Close Split", openBranches: "Branches…",
	manageRemotes: "Manage Remotes…", openRemoteRepository: "Open Repository in Browser", copyRemoteRepository: "Copy Repository URL",
	initializeRepository: "Initialize Git Repository…",
}

var deferredCommands = map[commandID]string{
	checkoutRebase: "Checkout and Rebase", checkoutUpdate: "Checkout and Update",
	newProjectFile: "File…", newProjectDirectory: "Directory…", cutProjectFile: "Cut", copyProjectFile: "Copy",
	pasteProjectFile: "Paste", replaceInProject: "Replace in Files…", reloadProjectFile: "Reload from Disk",
	shelveSilently: "Shelve Silently",
	newTabWindow:   "Open Tab in New Window",
	configureTabs:  "Configure Editor Tabs…",
	openInExplorer: "Open In", localHistory: "Local History", renameFile: "Rename File…",
	updateBranch: "Update", checkoutBranch: "Checkout", newBranch: "New Branch…", newWorktree: "New Worktree…",
	renameBranch: "Rename…", rollback: "Rollback…", shelve: "Shelve Changes…",
	importPatch: "Import Patches…", createPatch: "Create Patch…", copyPatch: "Copy as Patch to Clipboard",
	cherryPick: "Cherry-Pick", checkoutRevision: "Checkout Revision", showRepositoryRevision: "Show Repository at Revision",
	resetRevision: "Reset Current Branch to Here…", revertCommit: "Revert Commit", undoCommit: "Undo Commit…",
	editCommit: "Edit Commit Message…", fixupCommit: "Fixup…", squashCommit: "Squash Into…",
	dropCommit: "Drop Commit", rebaseFrom: "Interactively Rebase from Here…", pushUpTo: "Push All up to Here…",
	newTag: "New Tag…", browseRevision: "View in browser", revertChanges: "Revert Selected Changes",
	cherryPickChanges: "Cherry-Pick Selected Changes", extractChanges: "Extract Selected Changes to Separate Commit…",
	dropChanges: "Drop Selected Changes", getRevision: "Get from Revision",

	addIgnore:      "Add to .gitignore",
	moveChangelist: "Move to Another Changelist…", newChangelist: "New Changelist…", editChangelist: "Edit Changelist…",
	addToVCS: "Add to VCS", deleteFile: "Delete…", compareRevision: "Compare with Revision…",
	compareBranch: "Compare with Branch or Tag…", historySelection: "Show History for Selection…",
	mergeBranch: "Merge…", rebaseBranch: "Rebase…", stashChanges: "Stash Changes…", unstashChanges: "Unstash Changes…",
	cloneRepo: "Clone…", deleteBranch: "Delete Branch…",
	updateProject: "Update Project…", showWorktrees: "Worktrees…",
	showPullRequests: "Pull Requests…", showMergeRequests: "Merge Requests…",
}

func commandLabel(id commandID) string {
	if id == fileHistory {
		return "Show History"
	}
	if id == openLocalRepository {
		return "Open Local Repository…"
	}
	if label := commands[id]; label != "" {
		return label
	}
	if label := extraCommands[id]; label != "" {
		return label
	}
	return deferredCommands[id]
}

func (a *Workspace) menuNamed(m *ui.Menu, id commandID, target commandTarget, label, reason string) {
	if reason == "" {
		reason = a.disabledFor(id, target)
	}
	item := m.Item(label).ID(string(id) + ":" + target.source + ":" + target.file.Path + ":" + strconv.Itoa(target.index)).Disabled(reason != "").Tooltip(reason)
	if mods, key, known := a.nativeWorkspaceShortcut(id); known {
		if key != ui.KeyUnknown {
			item.Shortcut(mods, key)
		}
	} else {
		switch id {
		case toggleBookmark:
			item.Shortcut(0, ui.KeyF11)
		case nextBookmark:
			item.Shortcut(0, ui.KeyF2)
		case previousBookmark:
			item.Shortcut(ui.Shift, ui.KeyF2)
		case unshelve:
			item.Shortcut(ui.Ctrl|ui.Shift, ui.KeyU)
		case deleteShelf:
			item.Shortcut(0, ui.KeyDelete)
		case rollback:
			item.Shortcut(ui.Ctrl|ui.Alt, ui.KeyZ)
		case moveChangelist:
			item.Shortcut(ui.Alt|ui.Shift, ui.KeyM)
		case addToVCS, stageChanges:
			item.Shortcut(ui.Ctrl|ui.Alt, ui.KeyA)
		case editChangelist:
			item.Shortcut(0, ui.KeyF2)
		case cutProjectFile:
			item.Shortcut(ui.Cmd, ui.KeyX)
		case copyProjectFile:
			item.Shortcut(ui.Cmd, ui.KeyC)
		case pasteProjectFile:
			item.Shortcut(ui.Cmd, ui.KeyV)
		case findInProject:
			item.Shortcut(ui.Cmd|ui.Shift, ui.KeyF)
		case replaceInProject:
			item.Shortcut(ui.Cmd|ui.Shift, ui.KeyR)
		case renameFile:
			item.Shortcut(ui.Shift, ui.KeyF6)
		case deleteFile:
			item.Shortcut(0, ui.KeyDelete)
		case newTabWindow:
			item.Shortcut(ui.Shift, ui.KeyF4)
		case openRightSplit:
			item.Shortcut(ui.Shift, ui.KeyEnter)
		case previewDiff:
			item.Shortcut(ui.Cmd, ui.KeyD)
		case openCode:
			item.Shortcut(0, ui.KeyF4)
		case copyHash:
			item.Shortcut(ui.Cmd|ui.Alt|ui.Shift, ui.KeyC)
		case renameBranch, renameShelf, editCommit:
			item.Shortcut(0, ui.KeyF2)
		case goParent:
			item.Shortcut(0, ui.KeyRight)
		case goChild:
			item.Shortcut(0, ui.KeyLeft)
		}
	}
	if item.Chosen() {
		a.pending = func() { a.run(id, target) }
	}
}

func (a *Workspace) menuGroup(m *ui.Menu, target commandTarget, ids ...commandID) {
	for _, id := range ids {
		a.menu(m, id, target)
	}
}

func (a *Workspace) fileMenu(m *ui.Menu, source string, f domain.File) {
	t := commandTarget{source: source, file: f}
	if a.Model.Active().Repository.Root != "" {
		a.menu(m, showBlame, t)
		if source == "working" {
			a.menuGroup(m, t, stageHunk, discardHunk)
		} else if source == "staged" {
			a.menu(m, unstageHunk, t)
		}
		m.Separator()
	}
	if source == "working" && f.Status == "U" && a.Model.Active().Repository.ResolvingConflicts() {
		if a.ReadMergeDocument != nil {
			a.menu(m, openMergeEditor, t)
		}
		a.menuGroup(m, t, previewDiff, openDiff, openCode, copyPath)
		m.Separator()
		repo := a.Model.Active().Repository
		a.menuNamed(m, acceptCurrent, t, conflictActionLabel(repo, acceptCurrent), "")
		a.menuNamed(m, acceptIncoming, t, conflictActionLabel(repo, acceptIncoming), "")
		a.menu(m, markResolved, t)
		return
	}
	switch source {
	case "project":
		a.projectMenu(m, t, false)
	case "working", "staged":
		a.menuGroup(m, t, previewFileCommit, rollback, moveChangelist, previewDiff, openDiff)
		a.menuNamed(m, openCode, t, "Jump to Source", "")
		a.menu(m, copyPath, t)
		m.Separator()
		a.menu(m, deleteFile, t)
		if source == "working" {
			if a.Model.Active().Repository.Root == "" {
				a.menu(m, addToVCS, t)
			}
			a.menu(m, stageChanges, t)
		} else {
			a.menu(m, unstageChanges, t)
		}
		m.Separator()
		a.menuGroup(m, t, newChangelist, editChangelist)
		a.menuNamed(m, createPatch, t, "Create Patch from Local Changes…", "")
		a.menuGroup(m, t, copyPatch, shelve)
		m.Separator()
		a.menu(m, refreshChanges, t)
		m.Separator()
		a.historyMenu(m, t)
		m.Submenu("Git", func(g *ui.Menu) { a.editorGitMenu(g, t) })
	case "shelf":
		a.menuGroup(m, t, previewDiff, openDiff, compareLocal, copyPath)
		m.Separator()
		a.menuGroup(m, t, unshelve, createPatch)
	default:
		a.menuGroup(m, t, previewDiff, openDiff, compareLocal, compareBefore)
		a.menuNamed(m, openCode, t, "Edit Source", "")
		a.menu(m, openRevision, t)
		m.Separator()
		a.menuGroup(m, t, revertChanges, cherryPickChanges, extractChanges, dropChanges, copyPatch, createPatch, getRevision, historyToRevision)
		item := m.Item(commandLabel(showChangesToParents)).ID(string(showChangesToParents)).Checked(!a.repo().hideChangesToParents).
			Disabled(a.disabledFor(showChangesToParents, t) != "")
		if item.Chosen() {
			a.pending = func() { a.run(showChangesToParents, t) }
		}
	}
}

func (a *Workspace) projectMenu(m *ui.Menu, t commandTarget, folder bool) {
	m.Submenu("New", func(sub *ui.Menu) { a.menuGroup(sub, t, newProjectFile, newProjectDirectory) })
	m.Separator()
	a.menuGroup(m, t, cutProjectFile, copyProjectFile, copyPath, pasteProjectFile)
	m.Separator()
	a.menuGroup(m, t, findInProject, replaceInProject)
	m.Separator()
	a.menuGroup(m, t, renameFile, deleteFile)
	m.Separator()
	if !folder {
		a.menuGroup(m, t, openCode, openRightSplit)
	}
	m.Submenu("Open In", func(sub *ui.Menu) { a.menuNamed(sub, openInExplorer, t, "File Explorer", "") })
	a.historyMenu(m, t)
	m.Submenu("Git", func(sub *ui.Menu) {
		if !folder {
			a.editorGitMenu(sub, t)
			return
		}
		a.menuGroup(sub, t, addToVCS, addIgnore, fileHistory)
	})
	a.menu(m, reloadProjectFile, t)
	m.Separator()
	reason := ""
	if folder {
		reason = "Select a file to compare mock versions"
	}
	a.menuNamed(m, compareLocal, t, "Compare With…", reason)
}

func (a *Workspace) historyMenu(m *ui.Menu, t commandTarget) {
	m.Submenu("Local History", func(sub *ui.Menu) { a.menuNamed(sub, localHistory, t, "Show History", "") })
}

func (a *Workspace) editorGitMenu(m *ui.Menu, t commandTarget) {
	a.menu(m, previewFileCommit, t)
	if working, ok := matchFile(a.Model.Active().Repository.Working, t.file.Path); ok {
		a.menuNamed(m, stageChanges, commandTarget{source: "editor", file: working}, "Add", "")
	} else {
		a.menuNamed(m, addToVCS, t, "Add", "No matching working change")
	}
	a.menu(m, addIgnore, t)
	m.Separator()
	a.menuGroup(m, t, previewDiff, compareRevision)
	a.branchCompareMenu(m, t)
	a.menuGroup(m, t, fileHistory, showBlame, historySelection)
	a.menuNamed(m, openRevision, t, "Show Current Revision", "")
	a.menu(m, rollback, t)
	m.Separator()
	a.menuGroup(m, t, push, pull, fetch)
	m.Separator()
	a.menuGroup(m, t, mergeBranch, rebaseBranch)
	a.menu(m, openBranches, t)
	a.menuGroup(m, t, newBranch, newTag, resetRevision)
	m.Separator()
	a.menuGroup(m, t, stashChanges, unstashChanges, manageRemotes, cloneRepo)
}

func (a *Workspace) branchCompareMenu(m *ui.Menu, t commandTarget) {
	history := a.Model.Active().Repository.History
	m.Submenu(commandLabel(compareBranch), func(sub *ui.Menu) {
		added := false
		for i, entry := range history {
			if len(entry.Refs) == 0 {
				continue
			}
			if file, ok := matchFile(entry.Files, t.file.Path); ok {
				for _, ref := range entry.Refs {
					label := displayBranch(ref)
					a.menuNamed(sub, compareLocal, commandTarget{index: i, source: label, file: file}, label, "")
					added = true
				}
			}
		}
		if !added {
			sub.Item("No mock revisions for this file").Disabled(true)
		}
	})
}

func (a *Workspace) shelfMenu(m *ui.Menu, t commandTarget) {
	a.menuGroup(m, t, unshelve, restoreShelf)
	m.Separator()
	a.menuGroup(m, t, previewDiff, openDiff, compareLocal)
	m.Separator()
	a.menuGroup(m, t, createPatch, copyPatch, importPatch)
	m.Separator()
	a.menuGroup(m, t, renameShelf, deleteShelf)
}

func (a *Workspace) branchMenu(m *ui.Menu, name string) {
	t := commandTarget{source: name}
	repo := a.Model.Active().Repository
	currentName := repo.CurrentBranch
	current := name == currentName
	group := repoBranchGroup(repo, name)
	tag, remote := group == "Tags", group == "Remote"
	if !current {
		a.menu(m, checkoutBranch, t)
	}
	a.menuNamed(m, newBranch, t, "New Branch from '"+displayBranch(name)+"'…", "")
	if !current && !tag {
		a.menuNamed(m, checkoutRebase, t, "Checkout and Rebase onto '"+currentName+"'", "")
		if !remote {
			a.menu(m, checkoutUpdate, t)
		}
	}
	m.Separator()
	if !current {
		a.menuNamed(m, compareCurrentBranch, t, "Compare with '"+currentName+"'", "")
	}
	a.menu(m, branchDiff, t)
	if !current {
		a.menuNamed(m, missingBranchCommits, t, "Show commits in '"+displayBranch(name)+"' that are missing in '"+currentName+"'", "")
	}
	a.menuNamed(m, newWorktree, t, "New Worktree from '"+displayBranch(name)+"'…", "")
	if !current {
		m.Separator()
		a.menuNamed(m, mergeBranch, t, "Merge '"+displayBranch(name)+"' into '"+currentName+"'", "")
		if repo.Root != "" {
			a.menuNamed(m, rebaseBranch, t, "Rebase '"+currentName+"' onto '"+displayBranch(name)+"'…", "")
		}
	}
	if tag {
		return
	}
	m.Separator()
	a.menu(m, updateBranch, t)
	if !remote {
		a.menu(m, push, t)
		tracked := "origin/" + name
		if repo.Root != "" {
			upstream := repo.Upstreams[name]
			tracked = upstream.Remote + "/" + upstream.Branch
		}
		if slices.Contains(repo.Branches, tracked) {
			m.Submenu("Tracked Branch '"+tracked+"'", func(sub *ui.Menu) {
				a.menuGroup(sub, commandTarget{source: tracked}, branchDiff, missingBranchCommits)
			})
		}
		a.menu(m, renameBranch, t)
	}
	if !current {
		a.menu(m, deleteBranch, t)
	}
}

func (a *Workspace) logMenu(m *ui.Menu, index int, entry domain.Commit) {
	t := commandTarget{index: index, source: entry.Hash}
	if len(entry.Files) > 0 {
		t.file = entry.Files[0]
	}
	a.menuGroup(m, t, copyHash, createPatch, cherryPick)
	m.Separator()
	a.menuGroup(m, t, checkoutRevision, showRepositoryRevision, compareLocal)
	m.Separator()
	a.menuGroup(m, t, resetRevision, revertCommit, undoCommit)
	m.Separator()
	a.menuGroup(m, t, editCommit, fixupCommit, squashCommit, dropCommit, rebaseFrom, pushUpTo)
	m.Separator()
	a.menuGroup(m, t, newBranch, newTag)
	m.Separator()
	a.relativeMenu(m, goChild, a.repo().children[index])
	a.relativeMenu(m, goParent, entry.Parents)
	m.Separator()
	a.menu(m, browseRevision, t)
}

func (a *Workspace) relativeMenu(m *ui.Menu, id commandID, relatives []int) {
	if len(relatives) == 0 {
		a.menuNamed(m, id, commandTarget{}, commandLabel(id), "No matching commit in mock history")
		return
	}
	if len(relatives) == 1 {
		a.menu(m, id, commandTarget{index: relatives[0]})
		return
	}
	m.Submenu(commandLabel(id), func(sub *ui.Menu) {
		for _, i := range relatives {
			entry := a.Model.Active().Repository.History[i]
			a.menuNamed(sub, id, commandTarget{index: i}, entry.Hash+" "+entry.Subject, "")
		}
	})
}
