package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func (a *Workspace) confirmEmptyChangelists(c *ui.Context) {
	confirmation := a.emptyChangelists
	if confirmation.repository != a.Model.Index {
		return
	}
	open := true
	result := ui.AlertDialog(c, &open, "Remove empty changelists?",
		"Commit left these changelists empty: "+strings.Join(confirmation.names, ", ")+".",
		"Keep Empty Changelists", "Remove Empty Changelists")
	if !open && result != 1 {
		a.emptyChangelists = nil
		return
	}
	if result == 1 {
		a.removeEmptyChangelists(confirmation.repository, confirmation.ids)
		a.emptyChangelists = nil
	}
}

func (a *Workspace) removeEmptyChangelists(repository int, ids []string) {
	if repository < 0 || repository >= len(a.Model.Sessions) || repository >= len(a.repos) {
		return
	}
	if err := a.Model.Sessions[repository].Changelists.RemoveEmpty(ids); err != nil {
		a.record("Could not remove empty changelists · " + err.Error())
		return
	}
	view := &a.repos[repository]
	if slices.Contains(ids, view.changelistFilter) {
		view.changelistFilter = ""
		view.workingChosen.Clear()
		view.workingIndex = -1
		a.Model.Sessions[repository].Working = ""
	}
}

const (
	setActiveChangelist commandID = "changes.set-active-changelist"
	removeChangelist    commandID = "changes.remove-changelist"
	applyChangelist     commandID = "changes.apply-changelist"
	cancelChangelist    commandID = "changes.cancel-changelist"
	filterChangelist    commandID = "changes.filter-changelist"
)

type changelistDialog struct {
	open                      bool
	repository                int
	root                      string
	kind                      commandID
	id, name, choice, message string
	paths                     []string
}

func isChangelistCommand(id commandID) bool {
	switch id {
	case newChangelist, editChangelist, moveChangelist, setActiveChangelist, removeChangelist, applyChangelist, cancelChangelist, filterChangelist:
		return true
	}
	return false
}

func (a *Workspace) changelistPaths(target commandTarget) []string {
	if target.file.Path == "" {
		target = a.selectedChange()
	}
	source := target.source
	if source == "project" || source == "editor" {
		source, target.file = a.projectChange(target.file.Path)
	}
	if source != "working" && source != "staged" {
		return nil
	}
	var paths []string
	for _, file := range a.indexFiles(source == "working", target) {
		paths = append(paths, file.Path)
	}
	return paths
}

func (a *Workspace) changelistDisabled(id commandID, t commandTarget) string {
	s := a.Model.Active()
	if id == cancelChangelist {
		if a.changelistDialog == nil {
			return "No changelist dialog"
		}
		return ""
	}
	if id == applyChangelist {
		d := a.changelistDialog
		if d == nil || d.repository != a.Model.Index || d.root != s.Repository.Root {
			return "Return to the original repository"
		}
		return ""
	}
	if a.changelistDialog != nil || a.repositoryWrite != "" {
		return "Finish the current operation"
	}
	if id == newChangelist || id == filterChangelist {
		return ""
	}
	if id == moveChangelist {
		if len(a.changelistPaths(t)) == 0 {
			return "Select changed files"
		}
		return ""
	}
	target := a.changelistTargetID(t)
	if !slices.ContainsFunc(s.Changelists.Lists, func(c application.Changelist) bool { return c.ID == target }) {
		return "Select an existing changelist"
	}
	if (id == editChangelist || id == removeChangelist) && target == "default" {
		return "Default is reserved; create a named changelist"
	}
	return ""
}

func (a *Workspace) changelistTargetID(t commandTarget) string {
	state := &a.Model.Active().Changelists
	switch t.source {
	case "":
		return state.Active
	case "working", "staged", "project", "editor":
		if id := state.Owner(t.file.Path); id != "" {
			return id
		}
		return state.Active
	default:
		return t.source
	}
}

func (a *Workspace) runChangelist(id commandID, t commandTarget) {
	session := a.Model.Active()
	session.Changelists.Normalize(session.Repository)
	state := &session.Changelists
	if id == cancelChangelist {
		a.changelistDialog = nil
		return
	}
	if id == applyChangelist {
		d := a.changelistDialog
		var err error
		switch d.kind {
		case newChangelist:
			_, err = state.Create(d.name)
		case editChangelist:
			err = state.Rename(d.id, d.name)
		case removeChangelist:
			err = state.Remove(d.id)
			if a.repo().changelistFilter == d.id {
				a.repo().changelistFilter = ""
			}
		case moveChangelist:
			i := slices.IndexFunc(state.Lists, func(c application.Changelist) bool { return c.Name == d.choice })
			if i >= 0 {
				err = state.Move(d.paths, state.Lists[i].ID)
			} else {
				d.message = "Destination no longer exists"
				return
			}
		}
		if err != nil {
			d.message = err.Error()
			return
		}
		if d.kind == moveChangelist || d.kind == removeChangelist {
			a.repo().workingChosen.Clear()
			a.repo().workingIndex = -1
			session.Working = ""
		}
		a.changelistDialog = nil
		return
	}
	target := a.changelistTargetID(t)
	if id == setActiveChangelist {
		state.Active = target
		return
	}
	if id == filterChangelist {
		a.repo().changelistFilter = t.source
		a.repo().workingChosen.Clear()
		a.repo().workingIndex = -1
		session.Working = ""
		return
	}
	d := &changelistDialog{open: true, repository: a.Model.Index, root: session.Repository.Root, kind: id, id: target}
	for _, list := range state.Lists {
		if list.ID == target {
			d.name = list.Name
		}
	}
	if id == newChangelist {
		d.name = ""
	}
	if id == moveChangelist {
		d.paths = a.changelistPaths(t)
		d.choice = state.Lists[0].Name
	}
	a.changelistDialog = d
}

func (a *Workspace) changelistView(c *ui.Context, p tokens) {
	d := a.changelistDialog
	ui.DialogBase(c, &d.open, func(_ ui.Element, panel ui.Element) {
		panel.Width(420).Column().Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Changelist dialog").Children(func() {
			heading(c, p, commandLabel(d.kind), "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				switch d.kind {
				case newChangelist, editChangelist:
					ui.TextInput(c, &d.name).Label("Changelist name").FillWidth().AutoFocus()
				case moveChangelist:
					names := []string{}
					for _, list := range a.Model.Active().Changelists.Lists {
						names = append(names, list.Name)
					}
					ui.Text(c, fmt.Sprintf("Move %d whole files; Git contents and staging stay unchanged.", len(d.paths))).FillWidth()
					ui.Select(c, &d.choice, names).Label("Changelist destination").FillWidth()
				case removeChangelist:
					ui.Text(c, "Remove "+d.name+" and move its files to Default?").FillWidth()
				}
				if d.message != "" {
					ui.Text(c, d.message).TextColor(p.theme.Danger).FillWidth()
				}
				ui.Row(c).FillWidth().Gap(gap).Justify(ui.End).Children(func() { a.button(c, cancelChangelist, "Cancel"); a.button(c, applyChangelist, "Apply") })
			})
		})
	})
	if !d.open {
		a.changelistDialog = nil
	}
}

func (a *Workspace) changelistControls(c *ui.Context, p tokens) {
	session := a.Model.Active()
	state := &session.Changelists
	if session.Repository.Bisect.Active() || session.Repository.ResolvingConflicts() {
		return
	}
	if session.Repository.Root == "" && len(state.Lists) < 2 {
		return
	}
	ui.Row(c).FillWidth().Height(headerHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Children(func() {
		name := "All changelists"
		for _, list := range state.Lists {
			if list.ID == a.repo().changelistFilter {
				name = list.Name
			}
		}
		flatToolMenu(c, p, "Changelist filter", name, nil, func(m *ui.Menu) {
			a.menuNamed(m, filterChangelist, commandTarget{}, "All changelists", "")
			for _, list := range state.Lists {
				a.menuNamed(m, filterChangelist, commandTarget{source: list.ID}, fmt.Sprintf("%s (%d)", list.Name, len(list.Paths)), "")
			}
		}).Grow(1)
		titleTool(c, "+", "New Changelist").OnClick(func() { a.run(newChangelist, commandTarget{}) })
		flatToolMenu(c, p, "Manage Changelists", "…", nil, func(m *ui.Menu) {
			m.Submenu("Set Active", func(sub *ui.Menu) {
				for _, list := range state.Lists {
					label := list.Name
					if list.ID == state.Active {
						label += " (active)"
					}
					a.menuNamed(sub, setActiveChangelist, commandTarget{source: list.ID}, label, "")
				}
			})
			a.menuGroup(m, commandTarget{}, editChangelist, removeChangelist)
		})
	})
	if len(state.Lists) > 1 {
		ui.Text(c, "Commit includes staged files from all changelists").FillWidth().SingleLine().Ellipsis("…").FontSize(p.textSize(smallText)).TextColor(p.theme.TextMuted).Padding(0, inset)
	}
}

func (a *Workspace) changelistMenu(m *ui.Menu) {
	m.Submenu("Working Changelist", func(sub *ui.Menu) {
		a.menuNamed(sub, filterChangelist, commandTarget{}, "All changelists", "")
		for _, list := range a.Model.Active().Changelists.Lists {
			a.menuNamed(sub, filterChangelist, commandTarget{source: list.ID}, list.Name, "")
		}
	})
	m.Submenu("Active Changelist", func(sub *ui.Menu) {
		for _, list := range a.Model.Active().Changelists.Lists {
			label := list.Name
			if list.ID == a.Model.Active().Changelists.Active {
				label += " (active)"
			}
			a.menuNamed(sub, setActiveChangelist, commandTarget{source: list.ID}, label, "")
		}
	})
	a.menuGroup(m, commandTarget{}, newChangelist, editChangelist, removeChangelist)
}
