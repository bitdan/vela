package ui

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

type logViewOptions struct {
	root, timestamps, refsLeft, hideTags, expandedRefs bool
	hideAuthor, hideDate, showHash                     bool
	noMyHighlight, noMergeHighlight, noBranchHighlight bool
}

const (
	logSortIcon   commandID = "history.sort"
	logViewIcon   commandID = "history.view"
	logSearchIcon commandID = "history.search"
)

func (a *Workspace) logFilters(c *ui.Context, p tokens) {
	r, repo := a.repo(), a.Model.Active().Repository
	ui.Row(c).FillWidth().Height(headerHeight).Gap(3).Padding(0, gap).AlignItems(ui.Center).Children(func() {
		search := ui.TextInput(c, &r.logQuery).Label("Filter commits").Placeholder("Text or hash").Grow(1)
		if r.logSearchFocus {
			search.Focus()
			r.logSearchFocus = false
		}
		filterLabel := func(title, value string) string {
			if value == "" {
				return title
			}
			return title + " •"
		}
		menuOpen := false
		filterMenu := func(label, value string, build func(*ui.Menu)) ui.Element {
			button, open := flatToolMenuState(c, p, label, filterLabel(label, value), nil, build)
			menuOpen = menuOpen || open
			return button
		}
		filterMenu("Branch", r.logRef, func(m *ui.Menu) {
			previous := r.logRef
			defer func() {
				if r.logRef != previous || r.logRef == "" {
					r.logExcludeRef = ""
				}
			}()
			if m.Item("Select…").Chosen() {
				r.logRef = ""
			}
			m.Separator()
			m.Item("Favorites").Disabled(true)
			logRadio(m, "HEAD", repo.CurrentBranch, &r.logRef)
			for _, ref := range r.logRefs {
				if ref == repo.CurrentBranch || ref == "origin/"+repo.CurrentBranch {
					logRadio(m, ref, ref, &r.logRef)
				}
			}
			m.Separator()
			for _, group := range []string{"Local", "Remote", "Tags"} {
				m.Submenu(group, func(sub *ui.Menu) {
					for _, ref := range r.logRefs {
						if repoBranchGroup(repo, ref) == group {
							logRadio(sub, displayBranch(ref), ref, &r.logRef)
						}
					}
				})
			}
		})
		filterMenu("User", r.logAuthor, func(m *ui.Menu) {
			if m.Item("Select…").Chosen() {
				r.logAuthor = ""
			}
			if repo.Root == "" {
				logRadio(m, "me", "Lin", &r.logAuthor)
				m.Separator()
				m.Item("Recent").Disabled(true)
			}
			for _, author := range r.logAuthors {
				logRadio(m, author, author, &r.logAuthor)
			}
		})
		var dateButton ui.Element
		var latest time.Time
		if len(repo.History) != 0 {
			latest = repo.History[0].Timestamp
			dateButton = a.logDateButton(c, p, filterLabel("Date", r.logDate.preset), latest)
		}
		openPathEditor := false
		anchor := filterMenu("Paths", r.logPath, func(m *ui.Menu) {
			if m.Item("Select…").Chosen() {
				openPathEditor = true
			}
			m.Submenu("Select in Tree…", func(sub *ui.Menu) {
				for _, path := range r.logFilePaths {
					logRadio(sub, path, path, &r.logPath)
				}
			})
			m.Separator()
			m.Item("Roots").Disabled(true)
			if m.Item(repo.Name).Checked(!r.logHideRoot).Chosen() {
				r.logHideRoot = !r.logHideRoot
			}
		})
		sort := commitIcon(c, logSortIcon, "Log Sort").Tooltip("Sort and graph options").Radius(5)
		sortOpen := toolbarPopoverMenu(sort, p, func(m *ui.Menu) {
			m.Item("Sort").Disabled(true)
			if m.Item("By Commit Date").Radio(!r.logTopo).Chosen() {
				r.logTopo = false
			}
			if m.Item("Topologically").Radio(r.logTopo).Chosen() {
				r.logTopo = true
			}
			m.Separator()
			m.Item("Options").Disabled(true)
			logCheck(m, "First Parent", &r.logFirstParent)
			logCheck(m, "No Merges", &r.logNoMerges)
		})
		ui.Box(c).Width(1).Height(16).Background(p.theme.Border)
		commitIcon(c, refreshLog, "Refresh Log").Tooltip("Refresh  Ctrl+F5").OnClick(func() { a.run(refreshLog, commandTarget{}) })
		target := commandTarget{}
		if commit, ok := a.operationCommit(target); ok {
			target.source = commit.Hash
		}
		reason := a.disabledFor(cherryPick, target)
		tip := "Cherry-Pick"
		if reason != "" {
			tip += " — " + reason
		}
		commitIcon(c, cherryPick, "Cherry-Pick").Disabled(reason != "").Tooltip(tip).OnClick(func() { a.run(cherryPick, target) })
		view := commitIcon(c, logViewIcon, "Log View Options").Tooltip("Log View Options").Radius(5)
		viewOpen := toolbarPopoverMenu(view, p, func(m *ui.Menu) {
			v := &r.logView
			m.Item("Show").Disabled(true)
			logCheck(m, "Root Names", &v.root)
			logCheckInverted(m, "Compact References View", &v.expandedRefs)
			logCheckInverted(m, "Tag Names", &v.hideTags)
			m.Item("Long Edges").Disabled(true).Tooltip("Available with the full history graph")
			logCheck(m, "Commit Timestamp", &v.timestamps)
			logCheck(m, "References on the Left", &v.refsLeft)
			m.Submenu("Columns", func(sub *ui.Menu) {
				logCheckInverted(sub, "Author", &v.hideAuthor)
				logCheckInverted(sub, "Date", &v.hideDate)
				logCheck(sub, "Hash", &v.showHash)
			})
			m.Separator()
			m.Item("Highlight").Disabled(true)
			logCheckInverted(m, "My Commits", &v.noMyHighlight)
			logCheckInverted(m, "Merge Commits", &v.noMergeHighlight)
			logCheckInverted(m, "Current Branch", &v.noBranchHighlight)
			m.Item("Not Cherry-Picked Commits").Disabled(true).Tooltip("Requires real commit ancestry")
		})
		commitIcon(c, logSearchIcon, "Search Log").Tooltip("Search Log").OnClick(func() { r.logSearchFocus = true })

		// Drawn menus share MyGo's menu state, while the custom filter
		// popovers must be closed before they are built in the same frame.
		if menuOpen || sortOpen || viewOpen {
			r.datePicker.open = false
			r.logPathsOpen = false
		}
		if openPathEditor {
			r.datePicker.open = false
			r.logPathsOpen = true
		}
		if dateButton.Valid() {
			a.logDatePopover(c, p, dateButton, latest)
		}
		ui.Popover(c, anchor, &r.logPathsOpen, func() {
			ui.TextInput(c, &r.logPath).Width(220).Placeholder("Filter by path").Label("Filter by path").AutoFocus()
		})
	})
}

func logRadio(m *ui.Menu, label, value string, target *string) {
	if m.Item(label).Radio(*target == value).Chosen() {
		*target = value
	}
}

func logCheck(m *ui.Menu, label string, target *bool) {
	if m.Item(label).Checked(*target).Chosen() {
		*target = !*target
	}
}

func logCheckInverted(m *ui.Menu, label string, target *bool) {
	if m.Item(label).Checked(!*target).Chosen() {
		*target = !*target
	}
}

func addLogValue(values []string, value string) []string {
	if value != "" && !slices.Contains(values, value) {
		return append(values, value)
	}
	return values
}
