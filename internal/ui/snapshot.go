package ui

import (
	"github.com/egoist/mygo/ui/editor"
	"slices"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

func (a *Workspace) SessionSnapshot() application.WorkspaceSnapshot {
	out := application.WorkspaceSnapshot{ActiveRepository: a.Model.Active().Repository.Name}
	if a.Model.Active().Repository.Root != "" {
		out.ActiveRepository = ""
		out.ActiveRoot = a.Model.Active().Repository.Root
	}
	if a.Model.Production {
		out.ActiveRepository = ""
		if out.ActiveRoot == "" {
			out.ActiveRoot = a.lastRealRoot
		}
	}
	for i, s := range a.Model.Sessions {
		if a.Model.Production && s.Repository.Root == "" {
			continue
		}
		r := &a.repos[i]
		repo := application.RepositorySnapshot{Name: s.Repository.Name, Root: s.Repository.Root,
			Message: s.Message, Working: s.Working, Staged: s.Staged, Branch: s.Branch, Historical: s.Historical}
		groups := application.CloneShelfGrouping(r.shelfGroups)
		repo.ShelfGroups = &groups
		repo.WorkingSelection = slices.Sorted(r.workingChosen.All())
		lists := application.CloneChangelists(s.Changelists)
		lists.Normalize(s.Repository)
		repo.Changelists = &lists
		repo.StagedSelection = slices.Sorted(r.stagedChosen.All())
		if s.Commit >= 0 && s.Commit < len(s.Repository.History) {
			repo.Commit = s.Repository.History[s.Commit].Hash
		}
		for _, tab := range r.tabs {
			if tab.comparison != nil {
				repo.Tabs = append(repo.Tabs, application.TabSnapshot{Source: tab.id, Pinned: tab.pinned, Comparison: comparisonSnapshot(tab.comparison)})
				continue
			}
			saved := application.TabSnapshot{Source: tab.source, Path: tab.path, Code: tab.code != nil, Pinned: tab.pinned}
			identitySource := tab.source
			for _, prefix := range []string{"revision:", "local:", "before:"} {
				identitySource = strings.TrimPrefix(identitySource, prefix)
			}
			_, _, comparison := domain.ParseComparisonSource(tab.source)
			if s.Repository.Root != "" && (fixedObjectID(identitySource) || comparison) {
				saved.ChangeKind, saved.OldPath = tab.file.Status, tab.file.OldPath
			}
			if tab.code != nil {
				saved.Draft = a.snapshotEditorDraft(s.Repository.Root, &tab)
				if tab.document != nil {
					saved.Bookmarks = normalizeBookmarks(tab.document.bookmarks, tab.code.Buffer.LineCount())
					saved.FileType = tab.document.fileType
				}
				saved.Cursor = tab.code.Cursor()
				saved.ScrollY = tab.code.ScrollY()
			} else {
				saved.ScrollY = tab.diff.ScrollY()
				saved.Unified = tab.diff.Layout == editor.DiffUnified
			}
			repo.Tabs = append(repo.Tabs, saved)
		}
		if split := r.split; split != nil && a.snapshotEditorDraft(s.Repository.Root, split) != nil && !slices.ContainsFunc(repo.Tabs, func(t application.TabSnapshot) bool {
			return t.Source == split.source && t.Path == split.path && t.Code
		}) {
			repo.Tabs = append(repo.Tabs, application.TabSnapshot{Source: split.source, Path: split.path, Code: true, Pinned: split.pinned, Cursor: split.code.Cursor(), ScrollY: split.code.ScrollY(), Draft: a.snapshotEditorDraft(s.Repository.Root, split)})
		}
		for _, pending := range r.pendingTabs {
			if !slices.ContainsFunc(repo.Tabs, func(t application.TabSnapshot) bool {
				return t.Source == pending.Source && t.Path == pending.Path && t.Code == pending.Code
			}) {
				pending.Bookmarks = slices.Clone(pending.Bookmarks)
				pending.Comparison = cloneComparisonSnapshot(pending.Comparison)
				pending.Draft = cloneEditorDraft(pending.Draft)
				repo.Tabs = append(repo.Tabs, pending)
			}
		}
		if len(r.tabs) > 0 {
			repo.ActiveTab = r.tabs[r.active].id
		}
		if r.pendingActiveTab != "" {
			repo.ActiveTab = r.pendingActiveTab
		}
		out.Repositories = append(out.Repositories, repo)
	}
	for _, saved := range a.savedRepositories {
		if !slices.ContainsFunc(out.Repositories, func(s application.RepositorySnapshot) bool { return s.Root == saved.Root }) {
			out.Repositories = append(out.Repositories, cloneRepositorySnapshot(saved))
		}
	}
	if a.exitDiscardDrafts {
		for i := range out.Repositories {
			for j := range out.Repositories[i].Tabs {
				out.Repositories[i].Tabs[j].Draft = nil
			}
		}
	}
	return out
}

func (a *Workspace) RestoreSession(snapshot application.WorkspaceSnapshot) {
	a.restoreRoot = snapshot.ActiveRoot
	a.lastRealRoot = snapshot.ActiveRoot
	a.savedRepositories = nil
	for _, saved := range snapshot.Repositories {
		if saved.Root != "" && !slices.ContainsFunc(a.savedRepositories, func(s application.RepositorySnapshot) bool { return s.Root == saved.Root }) {
			a.savedRepositories = append(a.savedRepositories, cloneRepositorySnapshot(saved))
		}
	}
	active := a.Model.Index
	for i, session := range a.Model.Sessions {
		if a.Model.Production && session.Repository.Root == "" {
			continue
		}
		repo := session.Repository
		if repo.Name == snapshot.ActiveRepository {
			active = i
		}
		index := slices.IndexFunc(snapshot.Repositories, func(s application.RepositorySnapshot) bool { return s.Root == "" && s.Name == repo.Name })
		if index < 0 {
			continue
		}
		a.Model.Switch(i)
		r := a.repo()
		r.tabs, r.active = nil, 0
		saved := snapshot.Repositories[index]
		a.restoreRepositorySelections(saved)
		for _, entry := range saved.Tabs {
			file, ok := findFile(repo, entry.Source, entry.Path)
			if !ok {
				continue
			}
			if entry.Code {
				a.openCode(entry.Source, file)
				a.restoreEditorMetadata(&r.tabs[r.active], entry)
				code := r.tabs[r.active].code
				cursor := max(0, min(entry.Cursor, len(file.New)))
				code.Select(cursor, cursor)
				code.SetScrollY(max(0, entry.ScrollY))
			} else {
				a.openDiff(entry.Source, file)
				r.tabs[r.active].diff.SetScrollY(max(0, entry.ScrollY))
				if entry.Unified {
					r.tabs[r.active].diff.Layout = editor.DiffUnified
				}
			}
			r.tabs[r.active].pinned = entry.Pinned
		}
		a.activate(saved.ActiveTab)
	}
	a.Model.Switch(active)
}

func cloneRepositorySnapshot(saved application.RepositorySnapshot) application.RepositorySnapshot {
	if saved.ShelfGroups != nil {
		groups := application.CloneShelfGrouping(*saved.ShelfGroups)
		saved.ShelfGroups = &groups
	}
	if saved.Changelists != nil {
		lists := application.CloneChangelists(*saved.Changelists)
		saved.Changelists = &lists
	}
	saved.Tabs = slices.Clone(saved.Tabs)
	for i := range saved.Tabs {
		saved.Tabs[i].Bookmarks = slices.Clone(saved.Tabs[i].Bookmarks)
		saved.Tabs[i].Comparison = cloneComparisonSnapshot(saved.Tabs[i].Comparison)
		saved.Tabs[i].Draft = cloneEditorDraft(saved.Tabs[i].Draft)
	}
	saved.WorkingSelection = slices.Clone(saved.WorkingSelection)
	saved.StagedSelection = slices.Clone(saved.StagedSelection)
	return saved
}

func matchFile(files []domain.File, path string) (domain.File, bool) {
	for _, file := range files {
		if file.Path == path {
			return file, true
		}
	}
	return domain.File{}, false
}

func findFile(repo domain.Repository, source, path string) (domain.File, bool) {
	source = strings.TrimPrefix(source, "revision:")
	if strings.HasPrefix(source, "shelf:") || strings.HasPrefix(source, "shelf-index:") {
		id := strings.TrimPrefix(strings.TrimPrefix(source, "shelf-index:"), "shelf:")
		for _, entry := range repo.Shelf {
			if entry.ID == id {
				return matchFile(entry.Files, path)
			}
		}
		return domain.File{}, false
	}
	if source == "project" {
		return matchFile(projectFiles(repo), path)
	}
	if source == "shelf" {
		for _, shelf := range repo.Shelf {
			if file, ok := matchFile(shelf.Files, path); ok {
				return file, true
			}
		}
		return domain.File{}, false
	}
	if source == "working" || source == "staged" {
		if repo.Root != "" {
			if source == "working" {
				return matchFile(repo.Working, path)
			}
			return matchFile(repo.Staged, path)
		}
		for _, files := range [][]domain.File{repo.Working, repo.Staged} {
			if i := slices.IndexFunc(files, func(f domain.File) bool { return f.Path == path }); i >= 0 {
				return files[i], true
			}
		}
		return domain.File{}, false
	}
	i := slices.IndexFunc(repo.History, func(c domain.Commit) bool { return c.Hash == source })
	if i >= 0 {
		files := repo.History[i].Files
		if j := slices.IndexFunc(files, func(f domain.File) bool { return f.Path == path }); j >= 0 {
			return files[j], true
		}
	}
	return domain.File{}, false
}
