package ui

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	toggleShelfFile     commandID = "shelf.toggle-restore-file"
	selectShelfAll      commandID = "shelf.select-all-restore-files"
	clearShelfSelection commandID = "shelf.clear-restore-files"
)

func isShelfSelectionCommand(id commandID) bool {
	return id == toggleShelfFile || id == selectShelfAll || id == clearShelfSelection
}

func (a *Workspace) shelfSelectionDisabled(id commandID, t commandTarget) string {
	s := a.repositoryOperation
	if s == nil || s.op.Kind != domain.ApplyShelf || s.shelfChosen == nil {
		return "Open a real Unshelve file selection"
	}
	if s.cancel != nil || a.repositoryWrite != "" || a.repositoryRead != nil || a.Model.Active().Repository.Root != s.root {
		return "Wait for the original repository operation"
	}
	if id == toggleShelfFile {
		if _, ok := s.shelfChosen[t.source]; !ok {
			return "Choose a shelved file row"
		}
	}
	return ""
}

func (a *Workspace) runShelfSelection(id commandID, t commandTarget) {
	s := a.repositoryOperation
	if id == toggleShelfFile {
		s.shelfChosen[t.source] = !s.shelfChosen[t.source]
	} else {
		for path := range s.shelfChosen {
			s.shelfChosen[path] = id == selectShelfAll
		}
	}
	a.rebuildShelfSelection(s)
}

func (a *Workspace) rebuildShelfSelection(s *repositoryOperationDialog) {
	s.op.Paths = make([]string, 0, len(s.shelfFiles))
	for _, file := range s.shelfFiles {
		if s.shelfChosen[file.Path] {
			s.op.Paths = append(s.op.Paths, file.Path)
		}
	}
	if len(s.op.Paths) < len(s.shelfFiles) {
		s.op.RemoveApplied = false
	}
	s.message = ""
}

func (a *Workspace) shelfApplyReason(s *repositoryOperationDialog) string {
	if s.op.Kind != domain.ApplyShelf {
		return ""
	}
	repo := a.Model.Active().Repository
	if repo.Root != s.root {
		return "Return to the original repository"
	}
	if repo.Head != s.snapshot.Head || repo.CurrentBranch != s.snapshot.CurrentBranch || repo.IndexVersion != s.snapshot.IndexVersion || repo.WorktreeVersion != s.snapshot.WorktreeVersion {
		return "Repository changed; close and reopen the shelf selection"
	}
	if len(repo.Staged) > 0 || slices.ContainsFunc(repo.Working, func(file domain.File) bool { return file.Status != "?" }) {
		return "Commit, shelve or stash tracked changes before Unshelve"
	}
	entry, ok := a.realShelfEntry(commandTarget{shelfID: s.op.Target, shelfOID: s.op.Source})
	if !ok || entry.Deleted {
		return "Shelf changed; reopen the restore selection"
	}
	_, partial, err := application.SelectShelfFiles(entry, s.op.Paths, s.op.SelectShelfFiles)
	if err != nil {
		return err.Error()
	}
	if partial && s.op.RemoveApplied {
		return "Partial restore must retain the original shelf"
	}
	if a.updateDirtyBuffer(s.root) {
		return "Save or discard all unsaved source documents before Unshelve; Git may map renamed paths"
	}
	return ""
}

func (a *Workspace) shelfApplyFiles(c *ui.Context, p tokens, s *repositoryOperationDialog, busy bool) {
	ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf("%d / %d file paths", len(s.op.Paths), len(s.shelfFiles))).Grow(1)
		ui.Button(c, "Select All").Label(commandLabel(selectShelfAll)).Disabled(busy).OnClick(func() { a.run(selectShelfAll, commandTarget{}) })
		ui.Button(c, "Clear").Label(commandLabel(clearShelfSelection)).Disabled(busy).OnClick(func() { a.run(clearShelfSelection, commandTarget{}) })
	})
	s.shelfList.Selected = &s.shelfSelected
	s.shelfList.Key = func(i int) any { return s.shelfFiles[i].Path }
	s.shelfList.Label = func(i int) string { return s.shelfFiles[i].Status + " · " + s.shelfFiles[i].Path }
	ui.List(c, &s.shelfList, len(s.shelfFiles), func(i int) {
		file := s.shelfFiles[i]
		checked := s.shelfChosen[file.Path]
		ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c.Key("restore-file"), &checked, file.Status+" · "+file.Path).Label("Unshelve " + file.Path).FillWidth().Tooltip(file.Path).Disabled(busy).OnChange(func() { a.run(toggleShelfFile, commandTarget{source: file.Path}) })
		})
	}).FillWidth().Height(min(5*rowHeight, float32(len(s.shelfFiles))*rowHeight)).Label("Unshelve saved file selection")
	if s.shelfSelected >= 0 && s.shelfSelected < len(s.shelfFiles) {
		ui.ScrollHorizontal(c).FillWidth().Height(rowHeight).Children(func() { ui.Text(c, s.shelfFiles[s.shelfSelected].Path).SingleLine().TextColor(p.theme.TextMuted) })
	}
}

// Shelf application may follow target-side renames. Protect every source editor
// until the real Git callback, then refresh all mutable views for that root.
func (a *Workspace) lockShelfEditors() func() {
	locked := map[*editor.State]bool{}
	lock := func(entry *tab) {
		if entry != nil && entry.source == "project" && entry.code != nil {
			if _, ok := locked[entry.code]; !ok {
				locked[entry.code] = entry.code.ReadOnly
				entry.code.ReadOnly = true
			}
		}
	}
	for i := range a.repo().tabs {
		lock(&a.repo().tabs[i])
	}
	lock(a.repo().split)
	lock(a.repo().preview)
	return func() {
		for state, readOnly := range locked {
			state.ReadOnly = readOnly
		}
	}
}
