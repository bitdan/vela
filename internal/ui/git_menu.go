package ui

import (
	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	initializeRepository commandID = "workspace.initialize-repository"
	openLocalRepository  commandID = "workspace.open-local-repository"
	updateProject        commandID = "git.update-project"
	showWorktrees        commandID = "git.show-worktrees"
	openRemoteRepository commandID = "remote.open-repository"
	copyRemoteRepository commandID = "remote.copy-repository-url"
	showPullRequests     commandID = "remote.pull-requests"
	showMergeRequests    commandID = "remote.merge-requests"
)

func (a *Workspace) mainGitMenu(m *ui.Menu) {
	target := commandTarget{}
	a.menuNamed(m, showCommit, target, "Commit…", "")
	a.menuNamed(m, push, target, "Push…", "")
	a.menu(m, updateProject, target)
	a.menuNamed(m, pull, target, "Pull…", "")
	a.menu(m, fetch, target)
	m.Separator()
	a.menuGroup(m, target, mergeBranch, rebaseBranch)
	if len(a.Model.Active().Repository.MergeHeads) > 0 {
		a.menuGroup(m, target, continueMerge, abortMerge)
	} else if a.Model.Active().Repository.Rebase.Active() {
		a.menuGroup(m, target, continueRebase, skipRebase, abortRebase)
	} else if repo := a.Model.Active().Repository; repo.Sequencer.Active() {
		for _, id := range []commandID{continueSequence, skipSequence, abortSequence} {
			a.menuNamed(m, id, target, sequenceActionLabel(repo, id), "")
		}
	}
	m.Separator()
	a.menuGroup(m, target, openBranches, newBranch, newTag)
	a.menuNamed(m, resetRevision, target, "Reset HEAD…", "")
	m.Separator()
	a.menu(m, showWorktrees, target)
	m.Submenu("Bisect", func(sub *ui.Menu) {
		a.menuGroup(sub, target, startBisect, showBisectCandidate, createBisectWorktree, bisectGood, bisectBad, bisectSkip, resetBisect)
	})
	m.Separator()
	a.menuNamed(m, showLog, target, "Show Git Log", "")
	m.Submenu("Patch", func(sub *ui.Menu) {
		a.menuGroup(sub, target, createPatch, copyPatch, importPatch)
	})
	m.Submenu("Uncommitted Changes", func(sub *ui.Menu) {
		a.menuNamed(sub, showCommit, target, "Show Local Changes", "")
		a.menuGroup(sub, target, rollback, shelve, openShelf, stashChanges, unstashChanges)
	})
	m.Submenu("Current File", func(sub *ui.Menu) {
		if entry := a.activeDocument(); entry != nil {
			target := commandTarget{file: entry.file, source: entry.source}
			a.menuGroup(sub, target, previewDiff, openDiff, fileHistory, showBlame,copyPath)
		} else {
			sub.Item("Open a file to see its Git actions").Disabled(true)
		}
	})
	m.Separator()
	for _, provider := range []string{"GitLab", "GitHub", "Gitee"} {
		m.Submenu(provider, func(sub *ui.Menu) { a.remoteProviderMenu(sub, provider) })
	}
	a.menuGroup(m, target, manageRemotes, cloneRepo)
	a.menu(m, initializeRepository, target)
	if a.ReadRepository != nil && a.PickRepositoryDirectory != nil {
		a.menu(m, openLocalRepository, target)
	}
}

func (a *Workspace) remoteProviderMenu(m *ui.Menu, provider string) {
	type hostedRemote struct{ name, url string }
	var hosted []hostedRemote
	for _, remote := range a.Model.Active().Repository.Remotes {
		if kind, url := domain.RemoteWebURL(remote.URL); kind == provider {
			hosted = append(hosted, hostedRemote{remote.Name, url})
		}
	}
	actions := func(sub *ui.Menu, remote hostedRemote) {
		a.menuGroup(sub, commandTarget{source: remote.url}, openRemoteRepository, copyRemoteRepository)
	}
	switch len(hosted) {
	case 0:
		m.Item("No " + provider + " remote configured").Disabled(true)
	case 1:
		actions(m, hosted[0])
	default:
		for _, remote := range hosted {
			m.Submenu(remote.name, func(sub *ui.Menu) { actions(sub, remote) })
		}
	}
	m.Separator()
	if provider == "GitLab" {
		a.menu(m, showMergeRequests, commandTarget{})
	} else {
		a.menu(m, showPullRequests, commandTarget{})
	}
}
