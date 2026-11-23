package ui

import (
	"context"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

const openRecentRepository commandID = "workspace.open-recent-repository"

// StartSessionRestore runs only after Bootstrap has connected the async readers.
// Only the active repository is opened eagerly; other saved roots remain recent.
func (a *Workspace) StartSessionRestore() {
	root := a.restoreRoot
	a.restoreRoot = ""
	if root != "" && a.ReadRepository != nil {
		a.loadRepository(root, false)
	}
}

func (a *Workspace) recentRepositoryMenu(m *ui.Menu) {
	if a.ReadRepository == nil || len(a.savedRepositories) == 0 {
		return
	}
	m.Separator()
	for _, saved := range a.savedRepositories {
		a.menuNamed(m, openRecentRepository, commandTarget{source: saved.Root}, saved.Name+" · "+saved.Root, "")
	}
}

func (a *Workspace) restoreRepositorySelections(saved application.RepositorySnapshot) {
	s := a.Model.Active()
	r := a.repo()
	s.Message, s.Branch = saved.Message, saved.Branch
	if saved.Changelists != nil {
		s.Changelists = application.CloneChangelists(*saved.Changelists)
	}
	s.Changelists.Normalize(s.Repository)
	if s.Branch == "" {
		s.Branch = s.Repository.CurrentBranch
	}
	if file, ok := matchFile(s.Repository.Working, saved.Working); ok {
		s.Working = file.Path
	}
	if file, ok := matchFile(s.Repository.Staged, saved.Staged); ok {
		s.Staged = file.Path
	}
	for _, path := range append(saved.WorkingSelection, s.Working) {
		if _, ok := matchFile(s.Repository.Working, path); ok {
			r.workingChosen.Add(path)
		}
	}
	for _, path := range append(saved.StagedSelection, s.Staged) {
		if _, ok := matchFile(s.Repository.Staged, path); ok {
			r.stagedChosen.Add(path)
		}
	}
	if selected := slices.IndexFunc(s.Repository.History, func(c domain.Commit) bool { return c.Hash == saved.Commit }); selected >= 0 {
		s.Commit = selected
	}
	if s.Commit >= 0 && s.Commit < len(s.Repository.History) {
		if file, ok := matchFile(s.Repository.History[s.Commit].Files, saved.Historical); ok {
			s.Historical = file.Path
		}
	}
}

func (a *Workspace) restoreLoadedRepository() {
	root := a.Model.Active().Repository.Root
	index := slices.IndexFunc(a.savedRepositories, func(s application.RepositorySnapshot) bool { return s.Root == root })
	if index < 0 {
		return
	}
	saved := a.savedRepositories[index]
	a.savedRepositories = slices.Delete(a.savedRepositories, index, index+1)
	a.restoreRepositorySelections(saved)
	r := a.repo()
	r.pendingTabs = append([]application.TabSnapshot(nil), saved.Tabs...)
	r.pendingActiveTab = saved.ActiveTab
	if len(r.pendingTabs) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := &readRequest{cancel: cancel}
	a.sessionRestore = request
	remaining := append([]application.TabSnapshot(nil), r.pendingTabs...)
	var failed []application.TabSnapshot
	var next func()
	next = func() {
		if a.sessionRestore != request || a.Model.Active().Repository.Root != root {
			return
		}
		if len(remaining) == 0 {
			r.pendingTabs = failed
			if a.activate(r.pendingActiveTab) {
				r.pendingActiveTab = ""
			}
			stopRead(&a.sessionRestore)
			return
		}
		entry := remaining[0]
		remaining = remaining[1:]
		if entry.Comparison != nil {
			if !a.restoreComparisonTab(entry) {
				failed = append(failed, entry)
				a.record("Could not restore comparison tab · invalid identity or reader unavailable")
			}
			r.pendingTabs = append(append([]application.TabSnapshot(nil), failed...), remaining...)
			next()
			return
		}
		lookup := entry.Source
		var file domain.File
		var ok bool
		if strings.HasPrefix(lookup, "blame:") {
			file, ok = matchFile(a.Model.Active().Repository.Project, entry.Path)
		}
		if kind, revision, ok := strings.Cut(lookup, ":"); ok && (kind == "local" || kind == "before") {
			lookup = revision
		}
		if !ok {
			file, ok = findFile(a.Model.Active().Repository, strings.TrimPrefix(lookup, "blame:"), entry.Path)
		}
		_, _, comparison := domain.ParseComparisonSource(lookup)
		if !ok && (fixedObjectID(strings.TrimPrefix(strings.TrimPrefix(lookup, "revision:"), "blame:")) || comparison) {
			file = domain.File{Path: entry.Path, Status: entry.ChangeKind, OldPath: entry.OldPath}
			ok = true
		}
		if !ok {
			failed = append(failed, entry)
			a.record("Could not restore tab · " + entry.Path + " is not in the current repository snapshot")
			next()
			return
		}
		source := entry.Source
		a.readFileSource(ctx, root, source, file, func(file domain.File, err error) {
			if a.sessionRestore != request || a.Model.Active().Repository.Root != root {
				return
			}
			if err != nil {
				failed = append(failed, entry)
				a.record("Could not restore tab · " + entry.Path + " · " + err.Error())
			} else {
				if entry.Code {
					a.openCode(entry.Source, file)
					a.restoreEditorMetadata(&r.tabs[r.active], entry)
				} else {
					a.openDiff(entry.Source, file)
				}
				tab := &r.tabs[r.active]
				tab.pinned = entry.Pinned
				if tab.code != nil {
					cursor := max(0, min(entry.Cursor, len(file.New)))
					tab.code.Select(cursor, cursor)
					tab.code.SetScrollY(max(0, entry.ScrollY))
				} else {
					tab.diff.SetScrollY(max(0, entry.ScrollY))
					if entry.Unified {
						tab.diff.Layout = editor.DiffUnified
					}
				}
			}
			r.pendingTabs = append(append([]application.TabSnapshot(nil), failed...), remaining...)
			next()
		})
	}
	next()
}

// User navigation wins over an asynchronous startup restoration. Unread tab
// identities remain retryable; the user's explicit active-tab choice replaces
// the pending active identity.
func (a *Workspace) cancelSessionNavigation() {
	stopRead(&a.sessionRestore)
	a.repo().pendingActiveTab = ""
}
