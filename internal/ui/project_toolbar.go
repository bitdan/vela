package ui

import (
	"github.com/egoist/mygo/ui"
)

const (
	projectNewIcon    commandID = "project.new-menu"
	projectLocateIcon commandID = "project.locate-file"
	projectViewIcon   commandID = "project.view"
	newScratch        commandID = "mock.project-scratch"
)

func (a *Workspace) activeDocument() *tab {
	r := a.repo()
	if r.preview != nil {
		return r.preview
	}
	if len(r.tabs) > 0 {
		return &r.tabs[r.active]
	}
	return nil
}

func (a *Workspace) projectToolbar(c *ui.Context, p tokens, folders map[string][]string, canLocate bool) {
	r := a.repo()
	ui.Row(c).FillWidth().Height(headerHeight).Padding(0, gap).Gap(2).AlignItems(ui.Center).Background(p.chrome).Children(func() {
		ui.Text(c, "Project").Bold().Padding(0, gap)
		ui.Box(c).Grow(1)
		commitIcon(c, projectNewIcon, "New Project Item").Tooltip("New").Menu(func(m *ui.Menu) {
			a.menuGroup(m, commandTarget{}, newProjectFile, newScratch, newProjectDirectory)
		})
		ui.Box(c).Width(1).Height(16).Background(p.theme.Border)
		commitIcon(c, projectLocateIcon, "Select Opened File").Tooltip("Select Opened File").Disabled(!canLocate).
			OnClick(func() { r.projectLocate = a.activeDocument().path })
		commitIcon(c, expandChanges, "Expand Project Tree").Tooltip("Expand All").OnClick(func() {
			for folder := range folders {
				r.project.Open.Add(folder)
			}
		})
		commitIcon(c, collapseChanges, "Collapse Project Tree").Tooltip("Collapse All").OnClick(func() { r.project.Open.Clear() })
		commitIcon(c, projectViewIcon, "Project View Options").Tooltip("Project View Options").Menu(func(m *ui.Menu) {
			logCheck(m, "Show File Paths", &r.projectShowPaths)
		})
		a.panelAction(c, toggleCommit, "−")
	})
	divider(c, p)
}
