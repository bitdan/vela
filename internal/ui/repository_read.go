package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

type readRequest struct {
	cancel context.CancelFunc
}

func stopRead(request **readRequest) {
	if *request != nil {
		(*request).cancel()
		*request = nil
	}
}

func (a *Workspace) chooseRepository() {
	if a.pickingRepository {
		return
	}
	a.pickingRepository = true
	a.PickRepositoryDirectory(func(directory string, err error) {
		a.pickingRepository = false
		if err != nil {
			a.record("Could not choose repository · " + err.Error())
		} else if directory != "" {
			a.loadRepository(directory, false)
		}
	})
}

func (a *Workspace) loadRepository(directory string, refresh bool) {
	a.closeProjectSearchDialog()
	a.fileTypeDialog = nil
	stopRead(&a.logRead)
	a.stopSourceDecision()
	a.stopPatchExports()
	a.closeFileHistoryDialog()
	a.repositoryNavigation++
	stopRead(&a.sessionRestore)
	stopRead(&a.repositoryRead)
	stopRead(&a.repositoryFileRead)
	ctx, cancel := context.WithCancel(context.Background())
	request := &readRequest{cancel: cancel}
	a.repositoryRead = request
	writeAtStart := a.repositoryWrite
	a.status = "Loading repository…"
	a.ReadRepository(ctx, directory, func(repo domain.Repository, err error) {
		if a.repositoryRead != request {
			return
		}
		stopRead(&a.repositoryRead)
		if err != nil {
			a.record("Could not load repository · " + err.Error())
			return
		}
		// The reader returns Git's canonical root (including Windows long paths).
		for i := range a.savedRepositories {
			if a.savedRepositories[i].Root == directory {
				a.savedRepositories[i].Root = repo.Root
			}
		}
		if writeAtStart != "" && writeAtStart == repo.Root {
			for i, session := range a.Model.Sessions {
				if session.Repository.Root == repo.Root {
					a.Model.Switch(i)
					a.status = "Opened " + repo.Name
					return
				}
			}
		}
		a.Model.Switch(a.installRepository(repo))
		if refresh {
			a.status = "Refreshed " + repo.Name
		} else {
			a.status = "Opened " + repo.Name
		}
		a.restoreLoadedRepository()
	})
}

func (a *Workspace) installRepository(repo domain.Repository) int {
	if a.Model.Active().Repository.Root == repo.Root {
		stopRead(&a.logRead)
	}
	index := slices.IndexFunc(a.Model.Sessions, func(s application.Session) bool { return s.Repository.Root == repo.Root })
	view := newRepoView(repo, a.Layout)
	session := application.Session{Repository: repo, Branch: repo.CurrentBranch}
	if index < 0 {
		index = len(a.repos)
		a.Model.Sessions = append(a.Model.Sessions, session)
		a.repos = append(a.repos, view)
	} else {
		previous, oldView := a.Model.Sessions[index], a.repos[index]
		session.Message = previous.Message
		session.Amend = previous.Amend && previous.Repository.Head == repo.Head
		view.tabs, view.closed, view.active = oldView.tabs, oldView.closed, oldView.active
		view.pendingTabs = oldView.pendingTabs
		view.pendingActiveTab = oldView.pendingActiveTab
		view.mergeDrafts, view.mergeActive = oldView.mergeDrafts, oldView.mergeActive
		view.split, view.splitMoved, view.splitSize, view.splitVertical = oldView.split, oldView.splitMoved, oldView.splitSize, oldView.splitVertical
		view.toolPage, view.console, view.consoleOpen = oldView.toolPage, oldView.console, oldView.consoleOpen
		view.project, view.projectReady, view.projectShowPaths = oldView.project, oldView.projectReady, oldView.projectShowPaths
		view.commitGroup, view.historyGroup = oldView.commitGroup, oldView.historyGroup
		view.historyHideDetails, view.historyPreview = oldView.historyHideDetails, oldView.historyPreview
		view.logQuery, view.logAuthor, view.logRef, view.logPath = oldView.logQuery, oldView.logAuthor, oldView.logRef, oldView.logPath
		view.logHistoryPath, view.logDate = oldView.logHistoryPath, oldView.logDate
		view.logNoMerges, view.logFirstParent, view.logTopo = oldView.logNoMerges, oldView.logFirstParent, oldView.logTopo
		view.logExcludeRef, view.logView, view.logHideRoot = oldView.logExcludeRef, oldView.logView, oldView.logHideRoot
		if previous.Commit >= 0 && previous.Commit < len(previous.Repository.History) {
			hash := previous.Repository.History[previous.Commit].Hash
			if selected := slices.IndexFunc(repo.History, func(c domain.Commit) bool { return c.Hash == hash }); selected >= 0 {
				session.Commit = selected
			}
		}
		a.Model.Sessions[index], a.repos[index] = session, view
	}
	return index
}

func (a *Workspace) readFileForCommand(id commandID, target commandTarget) {
	a.cancelSessionNavigation()
	root := a.Model.Active().Repository.Root
	source := strings.TrimPrefix(target.source, "revision:")
	file := target.file
	switch id {
	case showBlame:
		source = "blame:" + a.blameRevision(target)
	case openCode, openRightSplit:
		if id == openRightSplit && target.tabID != "" && a.repo().tabIndex(target.tabID) >= 0 {
			a.splitTab(target.tabID, false, false)
			return
		}
		source = "project"
	case previewDiff, openDiff:
		if source == "project" {
			source, file = a.projectChange(file.Path)
		}
		if source == "working" && file.Status == "U" && a.Model.Active().Repository.ResolvingConflicts() {
			if id == previewDiff && a.ReadMergeDocument != nil {
				a.run(openMergeEditor, commandTarget{file: file})
				return
			}
			source = "merge-conflict"
		}
	case compareLocal:
		source = "local:" + source
	case compareBefore:
		source = "before:" + source
	}
	a.repo().mergeActive = ""
	stopRead(&a.repositoryFileRead)
	ctx, cancel := context.WithCancel(context.Background())
	request := &readRequest{cancel: cancel}
	a.repositoryFileRead = request
	a.status = "Loading " + file.Path + "…"
	readSource := source
	if id == openRevision {
		readSource = "revision:" + source
	}
	a.readFileSource(ctx, root, readSource, file, func(resolved domain.File, err error) {
		if a.repositoryFileRead != request {
			return
		}
		stopRead(&a.repositoryFileRead)
		if err != nil {
			a.record("Could not read file · " + err.Error())
			return
		}
		if id == openDiff || id == previewDiff || id == compareLocal || id == compareBefore {
			resolved.Old = strings.ReplaceAll(resolved.Old, "\r\n", "\n")
			resolved.New = strings.ReplaceAll(resolved.New, "\r\n", "\n")
		}
		switch id {
		case showBlame:
			a.openCode(source, resolved)
		case openCode:
			a.openCode("project", resolved)
		case openRevision:
			a.openCode("revision:"+source, resolved)
		case openRightSplit:
			previous := a.activeDocument()
			previousID := ""
			if previous != nil {
				previousID = previous.id
			}
			a.openCode("project", resolved)
			r := a.repo()
			a.splitTab(r.tabs[r.active].id, false, false)
			a.activate(previousID)
		case openDiff:
			a.openDiff(source, resolved)
		default:
			entry := diffTab(source, resolved)
			r := a.repo()
			if r.preview != nil && r.preview.source == source {
				entry.diff.Layout = r.preview.diff.Layout
			}
			r.preview = &entry
		}
		a.status = fmt.Sprintf("Opened %s", resolved.Path)
	})
}

func (a *Workspace) projectChange(path string) (string, domain.File) {
	repo := a.Model.Active().Repository
	if file, ok := matchFile(repo.Working, path); ok {
		return "working", file
	}
	if file, ok := matchFile(repo.Staged, path); ok {
		return "staged", file
	}
	return "", domain.File{}
}
