package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	startProjectSearch      commandID = "project.search-start"
	cancelProjectSearch     commandID = "project.search-cancel"
	openProjectSearchResult commandID = "project.search-open"
	closeProjectSearch      commandID = "project.search-close"
)

type projectSearchDialog struct {
	open, loading, matchCase bool
	root, query, message     string
	repository, selected     int
	result                   application.ProjectSearchResult
	list                     ui.ListState
	request                  *readRequest
}

func isProjectSearchCommand(id commandID) bool {
	switch id {
	case findInProject, startProjectSearch, cancelProjectSearch, openProjectSearchResult, closeProjectSearch:
		return true
	}
	return false
}

func (a *Workspace) closeProjectSearchDialog() {
	if s := a.projectSearch; s != nil {
		stopRead(&s.request)
		a.projectSearch = nil
	}
}

func (a *Workspace) invalidateProjectSearch() {
	if s := a.projectSearch; s != nil {
		stopRead(&s.request)
		s.loading = false
		s.result = application.ProjectSearchResult{}
		s.selected = -1
		s.message = "Press Find to search the project"
	}
}

func (a *Workspace) projectSearchDisabled(id commandID) string {
	if id == findInProject {
		if a.Model.Active().Repository.Root != "" && a.SearchProjectFiles == nil {
			return "Project search is unavailable"
		}
		return ""
	}
	s := a.projectSearch
	if s == nil || s.repository != a.Model.Index || s.root != a.Model.Active().Repository.Root {
		return "Open Find in Files"
	}
	switch id {
	case startProjectSearch:
		if s.loading {
			return "Search task is running"
		}
		if s.query == "" {
			return "Enter text to find"
		}
	case cancelProjectSearch:
		if !s.loading {
			return "No search task is running"
		}
	case openProjectSearchResult:
		if s.loading || s.selected < 0 || s.selected >= len(s.result.Matches) {
			return "Select a search result"
		}
	}
	return ""
}

func (a *Workspace) runProjectSearch(id commandID) {
	switch id {
	case findInProject:
		a.closeProjectSearchDialog()
		a.search.open = false
		a.projectSearch = &projectSearchDialog{open: true, root: a.Model.Active().Repository.Root, repository: a.Model.Index, selected: -1, message: "Press Find to search the project"}
	case closeProjectSearch:
		a.closeProjectSearchDialog()
	case cancelProjectSearch:
		s := a.projectSearch
		stopRead(&s.request)
		s.loading = false
		s.message = "Search canceled"
	case startProjectSearch:
		a.startProjectTextSearch()
	case openProjectSearchResult:
		a.openProjectTextResult()
	}
}

func (a *Workspace) projectSearchInput(s *projectSearchDialog) application.ProjectSearchInput {
	input := application.ProjectSearchInput{Root: s.root, Query: application.ProjectSearchQuery{Text: s.query, MatchCase: s.matchCase}, Overlays: map[string]string{}}
	seen := map[string]bool{}
	add := func(path string) {
		if !seen[path] {
			input.Paths = append(input.Paths, path)
			seen[path] = true
		}
	}
	for _, f := range projectFiles(a.Model.Active().Repository) {
		add(f.Path)
		if s.root == "" {
			input.Overlays[f.Path] = f.New
		}
	}
	collect := func(entry *tab) {
		if entry == nil || entry.code == nil || entry.source != "project" {
			return
		}
		add(entry.path)
		if s.root == "" || entry.code.Buffer.Modified() {
			input.Overlays[entry.path] = entry.code.Buffer.String()
		}
	}
	for i := range a.repo().tabs {
		collect(&a.repo().tabs[i])
	}
	collect(a.repo().split)
	return input
}

func (a *Workspace) startProjectTextSearch() {
	s := a.projectSearch
	input := a.projectSearchInput(s)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	s.loading = true
	s.message = "Searching project files…"
	s.result = application.ProjectSearchResult{}
	s.selected = -1
	done := func(result application.ProjectSearchResult, err error) {
		if !a.currentProjectSearch(s, request) || ctx.Err() == context.Canceled {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		s.loading = false
		if err != nil {
			s.message = "Could not search project · " + err.Error()
			return
		}
		s.result = result
		s.message = ""
		if len(result.Matches) > 0 {
			s.selected = 0
		} else {
			s.message = "No matches"
		}
		s.list.ScrollTo(0, ui.Start)
	}
	if s.root == "" {
		result, err := application.SearchProject(ctx, input, nil)
		done(result, err)
	} else {
		a.SearchProjectFiles(ctx, input, done)
	}
}

func (a *Workspace) currentProjectSearch(s *projectSearchDialog, request *readRequest) bool {
	return a.projectSearch == s && s.request == request && s.repository == a.Model.Index && s.root == a.Model.Active().Repository.Root
}

func (a *Workspace) projectSourceEditor(path string) *tab {
	if split := a.repo().split; split != nil && split.source == "project" && split.path == path && split.code != nil {
		return split
	}
	for i := range a.repo().tabs {
		entry := &a.repo().tabs[i]
		if entry.source == "project" && entry.path == path && entry.code != nil {
			return entry
		}
	}
	return nil
}

func (a *Workspace) selectProjectTextMatch(s *projectSearchDialog, m application.ProjectSearchMatch, entry *tab) bool {
	text := entry.code.Buffer.String()
	if m.Start < 0 || m.End < m.Start || m.End > len(text) || application.ProjectSearchVersion(text) != m.Version {
		s.message = "File or unsaved text changed; run Find again"
		return false
	}
	a.cancelSessionNavigation()
	a.repo().preview = nil
	a.repo().mergeActive = ""
	a.activate(entry.id)
	entry.code.Select(m.Start, m.End)
	a.searchFocus = entry.code
	a.closeProjectSearchDialog()
	return true
}

func (a *Workspace) openProjectTextResult() {
	s := a.projectSearch
	m := s.result.Matches[s.selected]
	if entry := a.projectSourceEditor(m.Path); entry != nil && (entry.code.Buffer.Modified() || s.root == "") {
		a.selectProjectTextMatch(s, m, entry)
		return
	}
	if s.root == "" {
		for _, f := range projectFiles(a.Model.Active().Repository) {
			if f.Path == m.Path {
				if application.ProjectSearchVersion(f.New) != m.Version {
					break
				}
				a.openCode("project", f)
				a.selectProjectTextMatch(s, m, a.activeDocument())
				return
			}
		}
		s.message = "File changed; run Find again"
		return
	}
	if m.InMemory {
		s.message = "Unsaved document changed or closed; run Find again"
		return
	}
	if a.ReadRepositoryFile == nil {
		s.message = "File reader is unavailable"
		return
	}
	a.cancelSessionNavigation()
	stopRead(&a.repositoryFileRead)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	s.loading = true
	s.message = "Opening search result…"
	a.ReadRepositoryFile(ctx, s.root, "project", domain.File{Path: m.Path}, func(file domain.File, err error) {
		if !a.currentProjectSearch(s, request) || ctx.Err() == context.Canceled {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		s.loading = false
		if err != nil {
			s.message = "Could not open result · " + err.Error()
			return
		}
		if file.Path != m.Path || application.ProjectSearchVersion(file.New) != m.Version {
			s.message = "File changed; run Find again"
			return
		}
		if entry := a.projectSourceEditor(m.Path); entry != nil && entry.code.Buffer.Modified() {
			a.selectProjectTextMatch(s, m, entry)
			return
		}
		a.openCode("project", file)
		a.selectProjectTextMatch(s, m, a.activeDocument())
	})
}

func (a *Workspace) projectSearchView(c *ui.Context, p tokens) {
	s := a.projectSearch
	width, height := c.Size()
	ui.DialogBase(c, &s.open, func(_ ui.Element, panel ui.Element) {
		panel.Size(min(width-32, 780), min(height-48, 460)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Column().Label("Find in Files dialog").Children(func() {
			heading(c, p, "Find in Files", "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, "Project files and unsaved Project documents · literal text").TextColor(p.theme.TextMuted)
				input := ui.TextInput(c, &s.query).Label("Find in Files query").Placeholder("Text to find").FillWidth().AutoFocus().OnChange(a.invalidateProjectSearch)
				ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
					ui.Checkbox(c, &s.matchCase, "Match case").OnChange(a.invalidateProjectSearch)
					ui.Text(c, "").Grow(1)
					ui.Button(c, "Find").Label("Find Project Text").Disabled(a.projectSearchDisabled(startProjectSearch) != "").OnClick(func() { a.run(startProjectSearch, commandTarget{}) })
					ui.Button(c, "Cancel Search").Label("Cancel Project Search").Disabled(a.projectSearchDisabled(cancelProjectSearch) != "").OnClick(func() { a.run(cancelProjectSearch, commandTarget{}) })
				})
				if input.Submitted() {
					a.pending = func() { a.run(startProjectSearch, commandTarget{}) }
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted)
				}
				if s.result.FirstError != "" {
					ui.Text(c, "Could not read "+s.result.FirstError).FillWidth().SingleLine().Ellipsis("…").Tooltip(s.result.FirstError).TextColor(p.theme.Danger)
				}
			})
			divider(c, p)
			matches := s.result.Matches
			s.list.Selected = &s.selected
			s.list.Label = func(i int) string {
				m := matches[i]
				return fmt.Sprintf("%s:%d:%d %s", m.Path, m.Line, m.Column, m.Preview)
			}
			list := ui.List(c, &s.list, len(matches), func(i int) {
				m := matches[i]
				ui.Row(c).Height(rowHeight).FillWidth().Padding(0, inset).Gap(gap).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%s:%d:%d", m.Path, m.Line, m.Column)).Width(300).SingleLine().Ellipsis("…")
					ui.Text(c, m.Preview).Grow(1).SingleLine().Ellipsis("…")
				})
			}).Grow(1).FillWidth().Label("Find in Files results")
			if list.Submitted() {
				a.pending = func() { a.run(openProjectSearchResult, commandTarget{}) }
			}
			divider(c, p)
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				summary := fmt.Sprintf("%d matches · %d files searched · %d skipped · %d unreadable", len(matches), s.result.Scanned, s.result.Skipped, s.result.Unreadable)
				if s.result.Limited {
					summary += " · Limit reached"
				}
				ui.Text(c, summary).FillWidth().SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted)
				ui.Row(c).FillWidth().Gap(gap).Justify(ui.End).Children(func() {
					ui.Button(c, "Close").Label("Close Find in Files").OnClick(func() { a.run(closeProjectSearch, commandTarget{}) })
					ui.PrimaryButton(c, "Open Result").Label("Open Search Result").Disabled(a.projectSearchDisabled(openProjectSearchResult) != "").OnClick(func() { a.run(openProjectSearchResult, commandTarget{}) })
				})
			})
		})
	})
	if !s.open && a.projectSearch == s {
		a.closeProjectSearchDialog()
	}
}
