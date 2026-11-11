package ui

import (
	"slices"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

func (a *Workspace) SessionSnapshot() application.WorkspaceSnapshot {
	out := application.WorkspaceSnapshot{ActiveRepository: a.Model.Active().Repository.Name}
	for i, s := range a.Model.Sessions {
		r := &a.repos[i]
		repo := application.RepositorySnapshot{Name: s.Repository.Name}
		for _, tab := range r.tabs {
			saved := application.TabSnapshot{Source: tab.source, Path: tab.path, Code: tab.code != nil, Pinned: tab.pinned}
			if tab.code != nil {
				saved.Cursor = tab.code.Cursor()
				saved.ScrollY = tab.code.ScrollY()
			} else {
				saved.ScrollY = tab.diff.ScrollY()
			}
			repo.Tabs = append(repo.Tabs, saved)
		}
		if len(r.tabs) > 0 {
			repo.ActiveTab = r.tabs[r.active].id
		}
		out.Repositories = append(out.Repositories, repo)
	}
	return out
}

func (a *Workspace) RestoreSession(snapshot application.WorkspaceSnapshot) {
	active := a.Model.Index
	for i, session := range a.Model.Sessions {
		repo := session.Repository
		if repo.Name == snapshot.ActiveRepository {
			active = i
		}
		index := slices.IndexFunc(snapshot.Repositories, func(s application.RepositorySnapshot) bool { return s.Name == repo.Name })
		if index < 0 {
			continue
		}
		a.Model.Switch(i)
		r := a.repo()
		r.tabs, r.active = nil, 0
		saved := snapshot.Repositories[index]
		for _, entry := range saved.Tabs {
			file, ok := findFile(repo, entry.Source, entry.Path)
			if !ok {
				continue
			}
			if entry.Code {
				a.openCode(entry.Source, file)
				code := r.tabs[r.active].code
				cursor := max(0, min(entry.Cursor, len(file.New)))
				code.Select(cursor, cursor)
				code.SetScrollY(max(0, entry.ScrollY))
			} else {
				a.openDiff(entry.Source, file)
				r.tabs[r.active].diff.SetScrollY(max(0, entry.ScrollY))
			}
			r.tabs[r.active].pinned = entry.Pinned
		}
		a.activate(saved.ActiveTab)
	}
	a.Model.Switch(active)
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
