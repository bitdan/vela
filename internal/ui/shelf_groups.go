package ui

import (
	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	newShelfGroup    commandID = "shelf.new-group"
	renameShelfGroup commandID = "shelf.rename-group"
	deleteShelfGroup commandID = "shelf.delete-group"
	moveShelfGroup   commandID = "shelf.move-group"
	filterShelfGroup commandID = "shelf.filter-group"
	applyShelfGroup  commandID = "shelf.apply-group"
	cancelShelfGroup commandID = "shelf.cancel-group"
)

type shelfGroupDialog struct {
	kind                    commandID
	root, id, name, problem string
	repository              int
}

func isShelfGroupCommand(id commandID) bool {
	return id == newShelfGroup || id == renameShelfGroup || id == deleteShelfGroup || id == moveShelfGroup || id == filterShelfGroup || id == applyShelfGroup || id == cancelShelfGroup
}
func (a *Workspace) shelfGroupEntry(t commandTarget) bool {
	if t.shelfID == "" {
		return false
	}
	for _, s := range a.Model.Active().Repository.Shelf {
		if s.ID == t.shelfID && s.OID == t.shelfOID {
			return true
		}
	}
	return false
}
func (a *Workspace) shelfGroupDisabled(id commandID, t commandTarget) string {
	s := a.shelfGroupDialog
	state := a.repo().shelfGroups
	if id == cancelShelfGroup {
		return ""
	}
	if id == applyShelfGroup {
		if s == nil || s.root != a.Model.Active().Repository.Root || s.repository != a.Model.Index {
			return "Return to the original repository"
		}
		return ""
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || s != nil || a.confirmExit || a.sessionSave != nil || a.squash != nil || a.repositoryOperation != nil || a.patchDialog != nil || a.patchImport != nil {
		return "Finish the current operation"
	}
	if id == newShelfGroup && len(state.Groups) >= 100 {
		return "Shelf groups are limited to 100"
	}
	if (id == renameShelfGroup || id == deleteShelfGroup) && !state.Has(t.source) {
		return "Select a named Shelf group"
	}
	if (id == moveShelfGroup || id == filterShelfGroup) && t.source != "" && !(id == filterShelfGroup && t.source == "ungrouped") && !state.Has(t.source) {
		return "Shelf group no longer exists"
	}
	if id == moveShelfGroup && !a.shelfGroupEntry(t) {
		return "Shelf changed; refresh the list"
	}
	return ""
}
func (a *Workspace) runShelfGroup(id commandID, t commandTarget) {
	state := &a.repo().shelfGroups
	switch id {
	case cancelShelfGroup:
		a.shelfGroupDialog = nil
	case filterShelfGroup:
		state.Filter = t.source
		a.repo().shelfIndex = -1
	case moveShelfGroup:
		if err := state.Move(t.shelfID, t.source); err != nil {
			a.record(err.Error())
		}
		a.repo().shelfIndex = -1
	case newShelfGroup, renameShelfGroup, deleteShelfGroup:
		name := ""
		if id != newShelfGroup {
			name = state.Name(t.source)
		}
		a.shelfGroupDialog = &shelfGroupDialog{kind: id, root: a.Model.Active().Repository.Root, repository: a.Model.Index, id: t.source, name: name}
	case applyShelfGroup:
		s := a.shelfGroupDialog
		var err error
		switch s.kind {
		case newShelfGroup:
			_, err = state.Create(s.name)
		case renameShelfGroup:
			err = state.Rename(s.id, s.name)
		case deleteShelfGroup:
			err = state.Delete(s.id)
		}
		if err != nil {
			s.problem = err.Error()
			return
		}
		a.shelfGroupDialog = nil
		a.repo().shelfIndex = -1
	}
}
func (a *Workspace) shelfInGroup(entry domain.Shelf) bool {
	s := a.repo().shelfGroups
	if s.Filter == "" {
		return true
	}
	group := s.Assignments[entry.ID]
	if s.Filter == "ungrouped" {
		return group == ""
	}
	return group == s.Filter
}
func (a *Workspace) shelfGroupToolbar(c *ui.Context, p tokens) {
	s := a.repo().shelfGroups
	ui.Button(c, s.Name(s.Filter)+" ▾").Label("Shelf Group Filter").Grow(1).Height(controlHeight).Menu(func(m *ui.Menu) {
		a.menuNamed(m, filterShelfGroup, commandTarget{}, "All Shelves", "")
		a.menuNamed(m, filterShelfGroup, commandTarget{source: "ungrouped"}, "Ungrouped", "")
		for _, g := range s.Groups {
			a.menuNamed(m, filterShelfGroup, commandTarget{source: g.ID}, g.Name, "")
		}
	})
	ui.Button(c, "+").Label("New Shelf Group").Height(controlHeight).Tooltip("New Shelf Group").Disabled(a.disabledFor(newShelfGroup, commandTarget{}) != "").OnClick(func() { a.run(newShelfGroup, commandTarget{}) })
	ui.Button(c, "⋯").Label("Manage Shelf Group").Height(controlHeight).Menu(func(m *ui.Menu) { a.menuGroup(m, commandTarget{source: s.Filter}, renameShelfGroup, deleteShelfGroup) })
}
func (a *Workspace) shelfMoveGroupMenu(m *ui.Menu, t commandTarget) {
	m.Submenu("Move to Group", func(m *ui.Menu) {
		t.source = ""
		a.menuNamed(m, moveShelfGroup, t, "Ungrouped", "")
		for _, g := range a.repo().shelfGroups.Groups {
			t.source = g.ID
			a.menuNamed(m, moveShelfGroup, t, g.Name, "")
		}
	})
}
func (a *Workspace) shelfGroupView(c *ui.Context, p tokens) {
	s := a.shelfGroupDialog
	open := true
	w, h := c.Size()
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 520)).MaxHeight(h-48).Column().Background(p.panel).Border(1, p.theme.Border).Children(func() {
			heading(c, p, commandLabel(s.kind), "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				if s.kind == deleteShelfGroup {
					ui.Text(c, "Delete group "+s.name+"? Entries move to Ungrouped; all Shelf backups remain.").FillWidth()
				} else {
					ui.TextInput(c, &s.name).Label("Shelf group name").FillWidth().AutoFocus()
				}
				if s.problem != "" {
					ui.Text(c, s.problem).FillWidth().TextColor(p.theme.Danger)
				}
				ui.Row(c).FillWidth().Justify(ui.End).Gap(gap).Children(func() {
					ui.Button(c, "Cancel").Label("Cancel Shelf Group").OnClick(func() { a.run(cancelShelfGroup, commandTarget{}) })
					ui.Button(c, "Apply").Label("Apply Shelf Group").Disabled(a.disabledFor(applyShelfGroup, commandTarget{}) != "").OnClick(func() { a.run(applyShelfGroup, commandTarget{}) })
				})
			})
		})
	})
	if !open {
		a.run(cancelShelfGroup, commandTarget{})
	}
}
