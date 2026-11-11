package ui

import (
	"cmp"
	"fmt"
	"path"
	"slices"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

var branchIcon = changeIcon("M4 5v6m0-2c4 0 8-2 8-4M6 3a2 2 0 1 1-4 0 2 2 0 0 1 4 0zm0 10a2 2 0 1 1-4 0 2 2 0 0 1 4 0zm8-10a2 2 0 1 1-4 0 2 2 0 0 1 4 0z")
var tagIcon = changeIcon("M2 2h6l6 6-6 6-6-6zM5 5h.1")
var favoriteIcon = changeIcon("M8 1.5 10 5.5l4.5.5-3.3 3 1 4.5L8 11.3l-4.2 2.2 1-4.5L1.5 6 6 5.5z")

var activityIcons = map[commandID]*ui.SVG{
	toggleGit:   branchIcon,
	openProject: changeIcon("M1.5 4.5A1 1 0 0 1 2.5 3.5H6L8 5.5h5.5a1 1 0 0 1 1 1v5a1 1 0 0 1-1 1h-11a1 1 0 0 1-1-1z"),
	showCommit:  changeIcon("M1.5 8H5m6 0h3.5M11 8a3 3 0 1 1-6 0 3 3 0 0 1 6 0z"),
}

func toolUnderline(e ui.Element, p tokens) {
	e.Draw(func(painter *ui.Painter, box ui.Rect) {
		painter.Fill(ui.Rect{X: box.X + 2, Y: box.Y + box.H - 2, W: max(0, box.W-4), H: 2}, p.selectionLine, 1)
	})
}

func toolTab(c *ui.Context, p tokens, label string, active bool, clicked func()) ui.Element {
	button := ui.ButtonBase(c.Key(label)).Height(headerHeight+4).Padding(0, inset).Center().Label(label).Role(ui.RoleTab).AccessSelected(active).
		Children(func() { ui.Text(c, label) })
	if active {
		toolUnderline(button, p)
	}
	return button.OnClick(clicked)
}

type flatMenuOpenKey struct{}

func flatToolMenu(c *ui.Context, p tokens, label, text string, icon *ui.SVG, build func(*ui.Menu)) ui.Element {
	button, _ := flatToolMenuState(c, p, label, text, icon, build)
	return button
}

func flatToolMenuState(c *ui.Context, p tokens, label, text string, icon *ui.SVG, build func(*ui.Menu)) (ui.Element, bool) {
	button := ui.ButtonBase(c.Key(label)).Height(controlHeight).Padding(0, gap).Gap(4).
		Row().AlignItems(ui.Center).Label(label).Radius(5)
	button.Children(func() {
		if icon != nil {
			ui.Icon(c, icon).Size(14, 14).TextColor(p.theme.TextMuted)
		}
		ui.Text(c, text).SingleLine()
		ui.Text(c, "⌄").TextColor(p.theme.TextMuted)
	})
	return button, toolbarPopoverMenu(button, p, build)
}

func toolbarPopoverMenu(button ui.Element, p tokens, build func(*ui.Menu)) bool {
	open := ui.Local(button, flatMenuOpenKey{}, func() bool { return false })
	button.PopoverMenuButton(build, ui.PopoverMenuButtonOptions{Open: open})
	if *open {
		button.Background(p.theme.Accent)
	}
	return *open
}

func (a *Workspace) panelAction(c *ui.Context, id commandID, glyph string) {
	label := commandLabel(id)
	reason := a.disabledFor(id, commandTarget{})
	tip := label
	if reason != "" {
		tip = reason
	}
	titleTool(c, glyph, label).Tooltip(tip).Disabled(reason != "").OnClick(func() { a.run(id, commandTarget{}) })
}

func (a *Workspace) changeTools(c *ui.Context, p tokens) {
	page := a.repo().toolPage
	ui.Column(c).Fill().Background(p.panel).Children(func() {
		if page == commandLabel(openProject) {
			a.projectPanel(c, p)
			return
		}
		ui.Row(c).Height(headerHeight+4).FillWidth().Padding(0, gap).Gap(gap).AlignItems(ui.Center).Background(p.chrome).Children(func() {
			for _, id := range []commandID{showCommit, openShelf} {
				label := commandLabel(id)
				active := page == label || (page == "" && id == showCommit)
				toolTab(c, p, label, active, func() { a.run(id, commandTarget{}) })
			}
			ui.Box(c).Grow(1)
			a.panelAction(c, toggleCommit, "−")
		})
		divider(c, p)
		switch page {
		case commandLabel(openShelf):
			a.shelfPanel(c, p)
		default:
			a.commit(c, p)
		}
	})
}

func projectFiles(repo domain.Repository) []domain.File {
	files := make(map[string]domain.File)
	for _, file := range repo.Staged {
		files[file.Path] = file
	}
	for _, file := range repo.Working {
		files[file.Path] = file
	}
	result := make([]domain.File, 0, len(files))
	for _, file := range files {
		result = append(result, file)
	}
	slices.SortFunc(result, func(a, b domain.File) int { return cmp.Compare(a.Path, b.Path) })
	return result
}

func (a *Workspace) projectPanel(c *ui.Context, p tokens) {
	r, repo := a.repo(), a.Model.Active().Repository
	files := projectFiles(repo)
	children := make(map[string][]string)
	byPath := make(map[string]domain.File, len(files))
	for _, file := range files {
		byPath[file.Path] = file
		for item := file.Path; item != "."; {
			parent := path.Dir(item)
			if !slices.Contains(children[parent], item) {
				children[parent] = append(children[parent], item)
			}
			item = parent
		}
	}
	for _, items := range children {
		slices.SortStableFunc(items, func(a, b string) int {
			switch {
			case len(children[a]) != 0 && len(children[b]) == 0:
				return -1
			case len(children[b]) != 0 && len(children[a]) == 0:
				return 1
			default:
				return cmp.Compare(path.Base(a), path.Base(b))
			}
		})
	}
	canLocate := false
	if entry := a.activeDocument(); entry != nil {
		_, canLocate = byPath[entry.path]
	}
	a.projectToolbar(c, p, children, canLocate)
	if len(files) == 0 {
		r.projectFocus = false
		ui.Text(c, "No project files").Padding(inset).TextColor(p.theme.TextMuted)
		return
	}
	if r.projectLocate != "" {
		r.project.Open.Add(".")
		for folder := path.Dir(r.projectLocate); folder != "."; folder = path.Dir(folder) {
			r.project.Open.Add(folder)
		}
	}
	if !r.projectReady {
		for folder := range children {
			r.project.Open.Add(folder)
		}
		r.projectReady = true
	}
	r.project.List.Selected = &r.projectIndex
	folderPrefix(c, p, &r.project)
	selectedFile := func() (domain.File, bool) {
		if r.projectIndex < 0 || r.projectIndex >= r.project.Rows() {
			return domain.File{}, false
		}
		f, ok := byPath[r.project.Item(r.projectIndex)]
		return f, ok
	}
	list := ui.Outline(c, &r.project, []string{"."}, func(item string) []string { return children[item] }, func(item string) {
		if file, ok := byPath[item]; ok {
			label := path.Base(item)
			if r.projectShowPaths {
				label = item
			}
			ui.Row(c).Height(rowHeight).Grow(1).Gap(gap).AlignItems(ui.Center).
				ContextMenu(func(m *ui.Menu) { a.fileMenu(m, "project", file) }).Children(func() {
				fileIcon(c, p, item, file.Status)
				ui.Text(c, label).Label(item).Tooltip(item).SingleLine().Ellipsis("…").
					TextColor(fileStatusColor(p, &a.Layout.VersionControl.Colors, file.Status))
			})
		} else {
			label, kind := path.Base(item), folderNode
			if item == "." {
				label = repo.Name
			}
			if _, ok := byPath[path.Join(item, "go.mod")]; ok || item == "." {
				kind = moduleNode
			}
			ui.Row(c).Height(rowHeight).Grow(1).Gap(gap).AlignItems(ui.Center).ContextMenu(func(m *ui.Menu) {
				a.projectMenu(m, commandTarget{source: "project", file: domain.File{Path: item}}, true)
			}).Children(func() {
				folderIcon(c, p, kind)
				name := ui.Text(c, label).Label("Project folder " + label).SingleLine().Ellipsis("…")
				if kind == moduleNode {
					name.Bold()
				}
			})
		}
	}).Grow(1).FillWidth().Label("Project tree").ContextMenu(func(m *ui.Menu) {
		if file, ok := selectedFile(); ok {
			a.fileMenu(m, "project", file)
		} else if r.projectIndex >= 0 && r.projectIndex < r.project.Rows() {
			a.projectMenu(m, commandTarget{source: "project", file: domain.File{Path: r.project.Item(r.projectIndex)}}, true)
		}
	})
	if r.projectFocus {
		if r.projectIndex < 0 {
			r.projectIndex = 0
		}
		list.Focus()
		r.projectFocus = false
	}
	if r.projectLocate != "" {
		for i := range r.project.Rows() {
			if r.project.Item(i) == r.projectLocate {
				r.projectIndex = i
				r.project.List.ScrollTo(i, ui.Center)
				break
			}
		}
		r.projectLocate = ""
	}
	if file, ok := selectedFile(); ok {
		target := commandTarget{source: "project", file: file}
		if list.Submitted() || r.project.List.Shortcut(c, 0, ui.KeyF4) {
			a.run(openCode, target)
		}
		if r.project.List.Shortcut(c, ui.Shift, ui.KeyEnter) {
			a.run(openRightSplit, target)
		}
		if r.project.List.Shortcut(c, ui.Cmd, ui.KeyD) {
			a.run(previewDiff, target)
		}
	}
}

type shelfRow struct {
	entry int
	file  *domain.File
}

func (a *Workspace) shelfPanel(c *ui.Context, p tokens) {
	shelves, r := a.Model.Active().Repository.Shelf, a.repo()
	heading(c, p, "Shelved Changes", fmt.Sprintf("%d entries", len(shelves)))
	a.shelfPrompts(c)
	ui.Row(c).Height(headerHeight).Padding(0, gap).Gap(2).AlignItems(ui.Center).Children(func() {
		commitIcon(c, expandChanges, "Expand All Shelves").Tooltip("Expand All Shelves").OnClick(func() {
			for i := range shelves {
				r.shelfExpanded[i] = true
			}
		})
		commitIcon(c, collapseChanges, "Collapse All Shelves").Tooltip("Collapse All Shelves").OnClick(func() { clear(r.shelfExpanded) })
		commitIcon(c, viewChangeOptions, "Shelf View Options").Tooltip("Shelf View Options").Menu(func(m *ui.Menu) {
			if m.Item("Show Recently Deleted").Checked(!r.hideDeletedShelf).Chosen() {
				r.hideDeletedShelf = !r.hideDeletedShelf
				r.shelfIndex = -1
			}
		})
	})
	var rows []shelfRow
	for _, deleted := range []bool{false, true} {
		if deleted && r.hideDeletedShelf {
			continue
		}
		start := len(rows)
		for i, entry := range shelves {
			if entry.Deleted != deleted {
				continue
			}
			rows = append(rows, shelfRow{entry: i})
			if r.shelfExpanded[i] && !deleted {
				for j := range entry.Files {
					rows = append(rows, shelfRow{entry: i, file: &entry.Files[j]})
				}
			}
		}
		if deleted && len(rows) > start {
			rows = slices.Insert(rows, start, shelfRow{entry: -1})
		}
	}
	if len(rows) == 0 {
		ui.Text(c, "No shelved changes").Padding(inset).TextColor(p.theme.TextMuted)
		return
	}
	r.shelf.Selected = &r.shelfIndex
	r.shelf.Key = func(i int) any {
		row := rows[i]
		if row.file != nil {
			return fmt.Sprintf("%d:%s", row.entry, row.file.Path)
		}
		return row.entry
	}
	r.shelf.Header = func(i int) bool { return rows[i].entry < 0 }
	menu := func(m *ui.Menu, row shelfRow) {
		if row.entry < 0 {
			return
		}
		if row.file != nil {
			t := commandTarget{source: "shelf", file: *row.file, index: row.entry}
			a.menuGroup(m, t, previewDiff, openDiff, compareLocal, copyPath)
			m.Separator()
			a.menu(m, unshelve, commandTarget{source: "shelf-file", file: *row.file, index: row.entry})
			a.menu(m, createPatch, t)
			return
		}
		entry := shelves[row.entry]
		t := commandTarget{source: "shelf", index: row.entry}
		if len(entry.Files) > 0 {
			t.file = entry.Files[0]
		}
		a.shelfMenu(m, t)
	}
	list := ui.List(c, &r.shelf, len(rows), func(i int) {
		row := rows[i]
		if row.entry < 0 {
			ui.Text(c, "Recently Deleted").Height(rowHeight).Padding(0, inset).Bold().Background(p.panel)
			return
		}
		entry := shelves[row.entry]
		ui.Row(c).Height(rowHeight).Padding(0, inset, 0, gap).Gap(gap).FillWidth().AlignItems(ui.Center).
			ContextMenu(func(m *ui.Menu) { menu(m, row) }).Children(func() {
			if row.file != nil {
				ui.Box(c).Width(2 * folderIndent).Shrink(0)
				ui.Text(c, row.file.Path).Grow(1).Ellipsis("…")
				return
			}
			if entry.Deleted || len(entry.Files) == 0 {
				ui.Box(c).Width(folderIndent).Shrink(0)
			} else {
				expanded := r.shelfExpanded[row.entry]
				action := "Expand "
				if expanded {
					action = "Collapse "
				}
				arrow := ui.ButtonBase(c).Size(folderIndent, rowHeight).Center().Label(action + entry.Title).
					Role(ui.RoleDisclosure).Expanded(expanded).OnClick(func() {
					r.shelfExpanded[row.entry] = !expanded
					r.shelfIndex = i
				})
				treeChevron(c, p, arrow, expanded)
			}
			ui.Text(c, entry.Title).Grow(1).Ellipsis("…")
		})
	}).Grow(1).FillWidth().Label("Shelf list").ContextMenu(func(m *ui.Menu) {
		if r.shelfIndex >= 0 && r.shelfIndex < len(rows) {
			menu(m, rows[r.shelfIndex])
		}
	})
	if r.shelfIndex >= 0 && r.shelfIndex < len(rows) {
		row := rows[r.shelfIndex]
		if row.entry < 0 {
			return
		}
		t := commandTarget{source: "shelf", index: row.entry}
		if row.file != nil {
			t.source, t.file = "shelf-file", *row.file
		}
		if r.shelf.Shortcut(c, ui.Ctrl|ui.Shift, ui.KeyU) {
			a.run(unshelve, t)
		}
		if row.file == nil && r.shelf.Shortcut(c, 0, ui.KeyF2) {
			a.run(renameShelf, t)
		}
		if row.file == nil && r.shelf.Shortcut(c, 0, ui.KeyDelete) {
			a.run(deleteShelf, t)
		}
		if list.Submitted() {
			if row.file != nil {
				a.run(previewDiff, commandTarget{source: "shelf", file: *row.file})
			} else if entry := shelves[row.entry]; !entry.Deleted && len(entry.Files) > 0 {
				r.shelfExpanded[row.entry] = !r.shelfExpanded[row.entry]
			}
		}
		if row.file != nil && r.shelf.Shortcut(c, ui.Cmd, ui.KeyD) {
			a.run(previewDiff, commandTarget{source: "shelf", file: *row.file})
		}
	}
}

func (a *Workspace) consolePanel(c *ui.Context, p tokens) {
	r := a.repo()
	heading(c, p, "Mock task output", a.Model.Active().Repository.Name)
	ui.Scroll(c).Grow(1).FillWidth().Children(func() {
		ui.Column(c).Padding(inset).Gap(gap).FillWidth().Children(func() {
			if len(r.console) == 0 {
				ui.Text(c, "No mock tasks recorded").TextColor(p.theme.TextMuted)
			}
			for _, line := range r.console {
				ui.Text(c, line).FontSize(p.textSize(smallText))
			}
		})
	})
}
