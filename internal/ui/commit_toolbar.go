package ui

import (
	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	shelveSilently  commandID = "shelf.shelve-silently"
	groupDirectory  commandID = "changes.group-directory"
	groupRepository commandID = "changes.group-repository"
	groupModule     commandID = "changes.group-module"
	showIgnored     commandID = "changes.show-ignored"
	locateChange    commandID = "changes.locate-selected"
	expandChanges   commandID = "changes.expand-all"
	collapseChanges commandID = "changes.collapse-all"
)

const commitToolSize = 24
const viewChangeOptions commandID = "changes.view-options"

var commitIcons = map[commandID]*ui.SVG{
	projectNewIcon:        changeIcon("M8 2v12M2 8h12"),
	projectLocateIcon:     changeIcon("M8 1v4m0 6v4M1 8h4m6 0h4M13 8a5 5 0 1 1-10 0 5 5 0 0 1 10 0z"),
	projectViewIcon:       changeIcon("M8 3h.01M8 8h.01M8 13h.01"),
	historicalDiffIcon:    changeIcon("M2 5h12l-3-3m3 3-3 3M14 11H2l3-3m-3 3 3 3"),
	historicalRevertIcon:  changeIcon("M6 3 2 7l4 4M2 7h8a4 4 0 0 1 0 8H7"),
	historicalHistoryIcon: changeIcon("M14 8a6 6 0 1 1-12 0 6 6 0 0 1 12 0zM8 4.5V8l2.5 1.5"),
	historicalViewIcon:    changeIcon("M1 8s2.5-4.5 7-4.5S15 8 15 8s-2.5 4.5-7 4.5S1 8 1 8zm9 0a2 2 0 1 1-4 0 2 2 0 0 1 4 0z"),
	refreshLog:            changeIcon("M13.5 7A5.5 5.5 0 0 0 4 4L2 6m0-4v4h4m-3.5 3A5.5 5.5 0 0 0 12 12l2-2m0 4v-4h-4"),
	cherryPick:            changeIcon("M5.5 10c0-3 4-3 4-8m1 8c0-4-1-5-1-8M8 12a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0zm5 0a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0z"),
	logSortIcon:           changeIcon("M4 2v12m-2-2 2 2 2-2M9 3h5m-5 4h4m-4 4h3"),
	logViewIcon:           changeIcon("M1 8s2.5-4.5 7-4.5S15 8 15 8s-2.5 4.5-7 4.5S1 8 1 8zm9 0a2 2 0 1 1-4 0 2 2 0 0 1 4 0z"),
	logSearchIcon:         changeIcon("M11 7a4 4 0 1 1-8 0 4 4 0 0 1 8 0zm-1 3 4 4"),
	refreshChanges:        changeIcon(`M13.5 7A5.5 5.5 0 0 0 4 4L2 6m0-4v4h4m-3.5 3A5.5 5.5 0 0 0 12 12l2-2m0 4v-4h-4`),
	rollback:              changeIcon(`M6 3 2 7l4 4M2 7h8a4 4 0 0 1 0 8H7`),
	previewDiff:           changeIcon(`M2 5h12l-3-3m3 3-3 3M14 11H2l3-3m-3 3 3 3`),
	shelveSilently:        changeIcon(`M8 1v8M5 6l3 3 3-3M2 8v5a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V8`),
	viewChangeOptions:     changeIcon(`M1 8s2.5-4.5 7-4.5S15 8 15 8s-2.5 4.5-7 4.5S1 8 1 8zm9 0a2 2 0 1 1-4 0 2 2 0 0 1 4 0z`),
	locateChange:          changeIcon(`M8 1v4m0 6v4M1 8h4m6 0h4M13 8a5 5 0 1 1-10 0 5 5 0 0 1 10 0z`),
	expandChanges:         changeIcon(`m5 5 3-3 3 3m-6 6 3 3 3-3`),
	collapseChanges:       changeIcon(`m5 2 3 3 3-3m-6 12 3-3 3 3`),
}

func changeIcon(d string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><path fill="none" stroke="currentColor" stroke-width="1.15" stroke-linecap="round" stroke-linejoin="round" d="` + d + `"/></svg>`))
}

func (a *Workspace) selectedChange() commandTarget {
	s, r := a.Model.Active(), a.repo()
	files, selected, source := s.Repository.Working, s.Working, "working"
	if r.commitSource == "staged" {
		files, selected, source = s.Repository.Staged, s.Staged, "staged"
	}
	if source == "working" && r.showIgnored {
		files = appendIgnored(files)
	}
	file, _ := matchFile(files, selected)
	return commandTarget{source: source, file: file}
}

func appendIgnored(files []domain.File) []domain.File {
	return append(append([]domain.File(nil), files...), domain.File{Path: ".idea/workspace.xml", Status: "I", New: "<workspace />\n"})
}

func commitIcon(c *ui.Context, id commandID, label string) ui.Element {
	return ui.ButtonBase(c.Key(string(id))).Size(commitToolSize, commitToolSize).Center().
		Label(label).Children(func() { ui.Icon(c, commitIcons[id]).Size(15, 15) })
}

func (a *Workspace) commitToolbar(c *ui.Context, p tokens) {
	r := a.repo()
	ui.Row(c).Height(headerHeight).FillWidth().Padding(0, gap).Gap(2).AlignItems(ui.Center).Children(func() {
		tool := func(id commandID, shortcut string, target commandTarget) {
			label := commandLabel(id)
			reason := a.disabledFor(id, target)
			tip := label
			if shortcut != "" {
				tip += "  " + shortcut
			}
			if reason != "" {
				tip += " — " + reason
			}
			commitIcon(c, id, label).Disabled(reason != "").Tooltip(tip).OnClick(func() { a.run(id, target) })
		}
		target := a.selectedChange()
		tool(refreshChanges, "", commandTarget{})
		tool(rollback, "Ctrl+Alt+Z", target)
		tool(previewDiff, "Ctrl+D", target)
		tool(shelveSilently, "Ctrl+Shift+H", target)
		commitIcon(c, viewChangeOptions, "View Options").Tooltip("View Options").Menu(func(m *ui.Menu) {
			m.Item("Group By").Disabled(true)
			for _, id := range []commandID{groupDirectory, groupRepository, groupModule} {
				item := m.Item(commandLabel(id)).ID(string(id)).Radio(r.commitGroup == id || (r.commitGroup == "" && id == groupRepository))
				if id == groupDirectory {
					item.Shortcut(ui.Ctrl|ui.Alt, ui.KeyP)
				}
				if id == groupModule {
					item.Shortcut(ui.Ctrl|ui.Alt, ui.KeyM)
				}
				if item.Chosen() {
					a.run(id, commandTarget{})
				}
			}
			m.Separator()
			m.Item("Show").Disabled(true)
			if m.Item(commandLabel(showIgnored)).ID(string(showIgnored)).Checked(r.showIgnored).Chosen() {
				a.run(showIgnored, commandTarget{})
			}
		})
		ui.Box(c).Width(1).Height(16).Background(p.theme.Border)
		tool(locateChange, "", target)
		tool(expandChanges, "", commandTarget{})
		tool(collapseChanges, "", commandTarget{})
	})
}
