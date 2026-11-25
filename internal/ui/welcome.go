package ui

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

const (
	openDemo  commandID = "workspace.open-demo"
	leaveDemo commandID = "workspace.leave-demo"
)

func (a *Workspace) welcomeActive() bool {
	return a.Model.Active().Repository.Root == "" && a.Model.Active().Repository.Name == ""
}
func (a *Workspace) demoActive() bool {
	return a.Model.Active().Repository.Root == "" && a.Model.Active().Repository.Name != ""
}
func welcomeCommandAllowed(id commandID) bool {
	switch id {
	case openDemo, leaveDemo, openLocalRepository, openRecentRepository, initializeRepository, cloneRepo, showSettings, toggleMainMenu, toggleTheme, cancelGitTask, selectRepo:
		return true
	}
	return false
}
func (a *Workspace) canSelectSession(index int) bool {
	if index < 0 || index >= len(a.Model.Sessions) {
		return false
	}
	if !a.Model.Production {
		return true
	}
	r := a.Model.Sessions[index].Repository
	return r.Root != "" || a.demoActive() && r.Name != ""
}

func (a *Workspace) enterDemo() {
	if root := a.Model.Active().Repository.Root; root != "" {
		a.lastRealRoot = root
	}
	stopRead(&a.repositoryRead)
	stopRead(&a.sessionRestore)
	a.repositoryNavigation++
	for i, s := range a.Model.Sessions {
		if s.Repository.Root == "" && s.Repository.Name == "vela" {
			a.selectRepository(i)
			a.status = "Demo Mode · simulated repositories"
			return
		}
	}
	demo := application.NewDemo()
	index := len(a.Model.Sessions)
	for _, s := range demo.Sessions {
		a.Model.Sessions = append(a.Model.Sessions, s)
		a.repos = append(a.repos, newRepoView(s.Repository, a.Layout))
	}
	a.selectRepository(index)
	if len(a.Model.Active().Repository.Working) > 0 {
		a.openDiff("working", a.Model.Active().Repository.Working[0])
	}
	a.status = "Demo Mode · simulated repositories"
}

func (a *Workspace) exitDemo() {
	index := slices.IndexFunc(a.Model.Sessions, func(s application.Session) bool {
		return s.Repository.Root != "" && s.Repository.Root == a.lastRealRoot
	})
	if index < 0 {
		index = 0
	}
	a.selectRepository(index)
	// The empty production landing session is intentionally absent from selectors.
	if a.Model.Production && a.Model.Sessions[index].Repository.Name == "" {
		a.Model.Switch(index)
		a.status = "Open a repository to get started"
	}
}

func (a *Workspace) selectRepository(index int) {
	a.closeIdleHunkDialog()
	if s := a.repositoryOperation; s != nil {
		stopRead(&s.shelfHunkRead)
	}
	a.stopBackgroundTasks()
	a.closeProjectFileDialog()
	a.closeBranchComparisonDialog()
	a.stopComparisonTabReads()
	if index < 0 || index >= len(a.Model.Sessions) {
		return
	}
	a.stopHistoryComparison()
	a.closeRevisionBrowser()
	a.changelistDialog = nil
	a.closeProjectSearchDialog()
	a.fileTypeDialog = nil
	stopRead(&a.logRead)
	a.stopSourceDecision()
	a.stopPatchExports()
	a.closeFileHistoryDialog()
	a.repositoryNavigation++
	stopRead(&a.sessionRestore)
	a.indexConfirmation = nil
	a.commitConfirmation = nil
	if a.repositoryOperation != nil && a.repositoryOperation.cancel == nil {
		a.repositoryOperation = nil
	}
	stopRead(&a.repositoryRead)
	stopRead(&a.repositoryFileRead)
	a.Model.Switch(index)
	a.syncBackgroundRepository()
	if root := a.Model.Active().Repository.Root; root != "" {
		a.lastRealRoot = root
	}
	a.status = "Switched to " + a.Model.Active().Repository.Name
}

func (a *Workspace) welcomeView(c *ui.Context, p tokens) {
	w, _ := c.Size()
	ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		ui.Column(c).Width(min(w-48, 640)).Gap(inset).Padding(inset).Children(func() {
			ui.Text(c, "Welcome to vela").Bold()
			ui.Text(c, "Open a local repository, clone a project, or initialize a new repository.").FillWidth()
			ui.Row(c).FillWidth().Gap(gap).Children(func() {
				for _, id := range []commandID{openLocalRepository, cloneRepo, initializeRepository} {
					id := id
					ui.Button(c, commandLabel(id)).Label(commandLabel(id)).Disabled(a.disabledFor(id, commandTarget{}) != "").OnClick(func() { a.run(id, commandTarget{}) })
				}
			})
			if a.repositoryRead != nil {
				ui.Text(c, "Opening repository…").TextColor(p.theme.TextMuted)
			} else if a.status != "Ready" {
				color := p.theme.TextMuted
				if strings.HasPrefix(a.status, "Could not") {
					color = p.theme.Danger
				}
				ui.Text(c, a.status).FillWidth().TextColor(color)
			}
			if len(a.savedRepositories) > 0 {
				ui.Text(c, "Recent repositories").Bold()
				ui.Scroll(c).FillWidth().MaxHeight(220).Children(func() {
					ui.Column(c).FillWidth().Gap(gap).Children(func() {
						for _, saved := range a.savedRepositories {
							root := saved.Root
							ui.Button(c, saved.Name+" · "+root).Label("Open recent " + root).FillWidth().Disabled(a.repositoryRead != nil).OnClick(func() { a.run(openRecentRepository, commandTarget{source: root}) })
						}
					})
				})
			}
			divider(c, p)
			ui.Button(c, "Explore Demo Mode").Label(commandLabel(openDemo)).Disabled(a.disabledFor(openDemo, commandTarget{}) != "").OnClick(func() { a.run(openDemo, commandTarget{}) })
			ui.Text(c, "Demo Mode uses simulated repositories and does not run Git writes.").TextColor(p.theme.TextMuted).FillWidth()
		})
	})
}
