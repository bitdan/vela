package ui

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"vela/internal/application"
	"vela/internal/domain"
)

type backgroundState struct {
	root, task, message     string
	refresh, remote         *readRequest
	pending                 bool
	nextRemote, nextRefresh time.Time
}

func (a *Workspace) syncBackgroundRepository() {
	repo := a.Model.Active().Repository
	if a.background.root != repo.Root {
		a.stopBackgroundTasks()
		a.background = backgroundState{root: repo.Root, nextRemote: time.Now().Add(30 * time.Second)}
	}
	if a.WatchRepository != nil {
		a.WatchRepository(repo)
	}
}

func (a *Workspace) stopBackgroundTasks() {
	if a.background.remote != nil {
		a.background.pending = true
	}
	stopRead(&a.background.refresh)
	stopRead(&a.background.remote)
	a.background.task = ""
}

func (a *Workspace) backgroundBlocked() bool {
	return a.repositoryWrite != "" || a.repositoryRead != nil || a.logRead != nil || a.sessionRestore != nil || a.repositoryFileRead != nil ||
		a.repositoryOperation != nil || a.squash != nil || a.indexConfirmation != nil || a.commitConfirmation != nil || a.patchDialog != nil || a.patchExport != nil || a.patchImport != nil ||
		a.projectFile != nil || a.projectSearch != nil || a.sourceDecision != nil || a.fileHistory != nil || a.branchComparison != nil || a.revisionBrowser != nil ||
		a.repositoryInit != nil || a.repositoryClone != nil || a.settingsOpen || a.remotes.open || a.confirmExit
}

// BackgroundPulse is called by Bootstrap on the main thread, never by View.
func (a *Workspace) BackgroundPulse(root string, dirty bool, now time.Time) {
	if root == "" || root != a.Model.Active().Repository.Root || root != a.background.root {
		return
	}
	a.background.pending = a.background.pending || dirty
	if a.backgroundBlocked() || a.backgroundInFlight > 0 || a.background.refresh != nil || a.background.remote != nil {
		return
	}
	if a.background.pending && !now.Before(a.background.nextRefresh) && a.RefreshRepository != nil {
		a.startBackgroundRefresh(now)
		return
	}
	settings := a.Layout.VersionControl.Git
	if (settings.AutoFetch || settings.CheckIncoming) && len(a.Model.Active().Repository.Remotes) > 0 && !now.Before(a.background.nextRemote) && a.CheckRepositoryRemote != nil {
		a.startBackgroundRemote(now)
	}
}

func (a *Workspace) BackgroundNotice(root, message string) {
	if root != a.Model.Active().Repository.Root {
		return
	}
	a.background.message = message
	a.repo().addConsole(message)
}

func (a *Workspace) startBackgroundRefresh(now time.Time) {
	repo, navigation := a.Model.Active().Repository, a.repositoryNavigation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	a.background.refresh, a.background.task, a.background.pending = request, "Refreshing files", false
	a.background.nextRefresh = now.Add(3 * time.Second)
	a.backgroundInFlight++
	a.RefreshRepository(ctx, repo, func(updated domain.Repository, err error) {
		a.backgroundInFlight--
		if a.background.refresh != request || a.Model.Active().Repository.Root != repo.Root || a.repositoryNavigation != navigation {
			return
		}
		a.background.refresh, a.background.task = nil, ""
		if err == nil {
			err = ctx.Err()
		}
		cancel()
		if err != nil {
			a.BackgroundNotice(repo.Root, "Background refresh failed · "+err.Error())
			return
		}
		if a.backgroundBlocked() {
			a.background.pending = true
			return
		}
		if updated.Root != repo.Root {
			a.BackgroundNotice(repo.Root, "Background refresh returned a different repository")
			return
		}
		a.installBackgroundRepository(updated)
	})
}

func (a *Workspace) installBackgroundRepository(updated domain.Repository) {
	s, r := a.Model.Active(), a.repo()
	previous := s.Repository
	sameLog := previous.Head == updated.Head && previous.CurrentBranch == updated.CurrentBranch && reflect.DeepEqual(previous.LogSnapshot, updated.LogSnapshot)
	if sameLog {
		// A page may have completed after the background reader captured its input.
		updated.History, updated.LogHasMore = previous.History, previous.LogHasMore
	} else {
		stopRead(&a.logRead)
		selected := ""
		if s.Commit >= 0 && s.Commit < len(previous.History) {
			selected = previous.History[s.Commit].Hash
		}
		s.Commit = slices.IndexFunc(updated.History, func(c domain.Commit) bool { return c.Hash == selected })
		if s.Commit < 0 {
			s.Commit = 0
			s.Historical = ""
			r.fileIndex = -1
			r.historicalChosen.Clear()
		}
		r.rebuildCommitLog(updated)
		r.detailsBranches.index = -1
	}
	for _, old := range previous.Working {
		if _, ok := matchFile(updated.Working, old.Path); !ok {
			r.workingChosen.Remove(old.Path)
		}
	}
	for _, old := range previous.Staged {
		if _, ok := matchFile(updated.Staged, old.Path); !ok {
			r.stagedChosen.Remove(old.Path)
		}
	}
	if _, ok := matchFile(updated.Working, s.Working); !ok {
		s.Working = ""
		r.workingIndex = -1
	}
	if _, ok := matchFile(updated.Staged, s.Staged); !ok {
		s.Staged = ""
		r.stagedIndex = -1
	}
	if !reflect.DeepEqual(previous.Project, updated.Project) || !reflect.DeepEqual(previous.ProjectDirectories, updated.ProjectDirectories) {
		r.projectTree = buildProjectTree(updated.Project, updated.ProjectDirectories...)
	}
	s.Amend = s.Amend && previous.Head == updated.Head
	s.Repository = updated
	s.Changelists.Normalize(updated)
	if previous.Head == updated.Head && previous.IndexVersion == updated.IndexVersion && previous.WorktreeVersion == updated.WorktreeVersion {
		a.syncBackgroundRepository()
		return
	}
	paths := map[string]bool{}
	collect := func(entry *tab) {
		if entry != nil && (entry.source == "project" || entry.source == "working" || entry.source == "staged") {
			paths[entry.path] = true
		}
	}
	for i := range r.tabs {
		collect(&r.tabs[i])
	}
	collect(r.split)
	collect(r.preview)
	for path := range paths {
		a.refreshOpenFileViews(updated.Root, path)
	}
	a.syncBackgroundRepository()
}

func (a *Workspace) startBackgroundRemote(now time.Time) {
	repo, navigation := a.Model.Active().Repository, a.repositoryNavigation
	settings := a.Layout.VersionControl.Git
	policy := application.BackgroundRemotePolicy{AutoFetch: settings.AutoFetch, CheckIncoming: settings.CheckIncoming, Network: domain.NetworkPolicy{FetchTags: settings.FetchTags, DisableCredentialHelper: !settings.CredentialHelper, NonInteractive: true}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	request := &readRequest{cancel: cancel}
	a.background.remote, a.background.task = request, "Checking remote"
	if policy.AutoFetch {
		a.background.task = "Auto Fetch"
	}
	a.background.nextRemote = now.Add(5 * time.Minute)
	a.backgroundInFlight++
	a.CheckRepositoryRemote(ctx, repo, policy, func(result application.BackgroundRemoteResult, err error) {
		a.backgroundInFlight--
		if a.background.remote != request || repo.Root != a.Model.Active().Repository.Root || navigation != a.repositoryNavigation {
			return
		}
		a.background.remote, a.background.task = nil, ""
		if err == nil {
			err = ctx.Err()
		}
		cancel()
		message := result.Message
		if err != nil {
			message = "Background remote task stopped · " + err.Error()
			if len(result.Fetched) > 0 {
				message += fmt.Sprintf(" · fetched: %s", strings.Join(result.Fetched, ", "))
			}
		}
		if message != "" {
			a.BackgroundNotice(repo.Root, message)
		}
		if policy.AutoFetch && (len(result.Fetched) > 0 || err != nil) {
			a.background.pending = true
		}
		a.BackgroundPulse(repo.Root, false, time.Now())
	})
}
