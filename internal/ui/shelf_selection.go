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
	return id == toggleShelfFile || id == selectShelfAll || id == clearShelfSelection || id == previewShelfHunks || id == toggleShelfHunk || id == wholeShelfFile
}

func (a *Workspace) shelfSelectionDisabled(id commandID, t commandTarget) string {
	s := a.repositoryOperation
	if s == nil || s.op.Kind != domain.ApplyShelf || s.shelfChosen == nil {
		return "Open a real Unshelve file selection"
	}
	if s.cancel != nil || a.repositoryWrite != "" || a.repositoryRead != nil || a.Model.Active().Repository.Root != s.root {
		return "Wait for the original repository operation"
	}
	if s.shelfHunkRead != nil {
		return "Wait for the Shelf hunk preview"
	}
	if id == previewShelfHunks {
		if a.ReadRepositoryPatch == nil {
			return "Shelf hunk reader unavailable"
		}
		if !slices.ContainsFunc(s.shelfFiles, func(f domain.File) bool { return f.Path == t.source && f.Status == "M" && f.OldPath == "" }) {
			return "Select an ordinary modified Shelf file"
		}
	}
	if id == toggleShelfHunk {
		chosen, ok := s.shelfHunkChosen[t.source]
		if !ok || t.index < 0 || t.index >= len(chosen) {
			return "Choose a loaded Shelf hunk"
		}
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
	if id == previewShelfHunks {
		a.loadShelfHunks(s, t.source)
		return
	}
	if id == toggleShelfHunk {
		s.shelfHunkChosen[t.source][t.index] = !s.shelfHunkChosen[t.source][t.index]
		a.rebuildShelfSelection(s)
		return
	}
	if id == wholeShelfFile {
		delete(s.shelfHunkChosen, t.source)
		delete(s.shelfHunkDocs, t.source)
		s.shelfHunkPath = ""
		s.shelfChosen[t.source] = true
		a.rebuildShelfSelection(s)
		return
	}
	if id == toggleShelfFile {
		s.shelfChosen[t.source] = !s.shelfChosen[t.source]
		for i := range s.shelfHunkChosen[t.source] {
			s.shelfHunkChosen[t.source][i] = s.shelfChosen[t.source]
		}
	} else {
		s.shelfHunkChosen = nil
		s.shelfHunkDocs = nil
		s.shelfHunkPath = ""
		for path := range s.shelfChosen {
			s.shelfChosen[path] = id == selectShelfAll
		}
	}
	a.rebuildShelfSelection(s)
}

func (a *Workspace) rebuildShelfSelection(s *repositoryOperationDialog) {
	s.op.Paths = make([]string, 0, len(s.shelfFiles))
	s.op.ShelfHunks = nil
	for _, file := range s.shelfFiles {
		if chosen, ok := s.shelfHunkChosen[file.Path]; ok {
			indices := make([]int, 0, len(chosen))
			for i, on := range chosen {
				if on {
					indices = append(indices, i)
				}
			}
			s.shelfChosen[file.Path] = len(indices) > 0
			if len(indices) > 0 {
				s.op.ShelfHunks = append(s.op.ShelfHunks, domain.ShelfHunkSelection{Path: file.Path, Version: s.shelfHunkDocs[file.Path].Version, Hunks: indices})
			}
		}
		if s.shelfChosen[file.Path] {
			s.op.Paths = append(s.op.Paths, file.Path)
		}
	}
	if len(s.op.Paths) < len(s.shelfFiles) {
		s.op.RemoveApplied = false
	}
	if len(s.op.ShelfHunks) > 0 {
		s.op.RemoveApplied = false
		s.op.RestoreIndex = false
	}
	s.message = ""
}

func (a *Workspace) shelfApplyReason(s *repositoryOperationDialog) string {
	if s.op.Kind != domain.ApplyShelf {
		return ""
	}
	if s.shelfHunkRead != nil {
		return "Wait for the Shelf hunk preview"
	}
	if err := application.ValidateShelfHunks(s.op.Paths, s.op.ShelfHunks); err != nil {
		return err.Error()
	}
	repo := a.Model.Active().Repository
	if repo.Root != s.root {
		return "Return to the original repository"
	}
	if repo.Head != s.snapshot.Head || repo.CurrentBranch != s.snapshot.CurrentBranch || repo.IndexVersion != s.snapshot.IndexVersion || repo.WorktreeVersion != s.snapshot.WorktreeVersion {
		return "Repository changed; close and reopen the shelf selection"
	}
	if len(repo.Staged) > 0 || slices.ContainsFunc(repo.Working, func(file domain.File) bool { return file.Status != "?" }) {
		return "Commit, shelve tracked changes before Unshelve"
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
	busy = busy || s.shelfHunkRead != nil
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
			label := file.Status + " · " + file.Path
			on, total := 0, 0
			if choices, ok := s.shelfHunkChosen[file.Path]; ok {
				total = len(choices)
				for _, v := range choices {
					if v {
						on++
					}
				}
				label += fmt.Sprintf(" · %d/%d hunks", on, total)
			}
			box := ui.Checkbox(c.Key("restore-file"), &checked, label).Label("Unshelve " + file.Path).Grow(1).Tooltip(file.Path).Disabled(busy).OnChange(func() { a.run(toggleShelfFile, commandTarget{source: file.Path}) })
			if on > 0 && on < total {
				box.Mixed()
			}
			ui.Button(c, "Hunks…").Label("Hunks " + file.Path).Disabled(busy || a.disabledFor(previewShelfHunks, commandTarget{source: file.Path}) != "").OnClick(func() { a.run(previewShelfHunks, commandTarget{source: file.Path}) })
		})
	}).FillWidth().Height(min(5*rowHeight, float32(len(s.shelfFiles))*rowHeight)).Label("Unshelve saved file selection")
	a.shelfHunkFields(c, p, s, busy)
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
