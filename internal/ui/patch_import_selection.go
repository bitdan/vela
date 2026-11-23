package ui

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	toggleImportedFile     commandID = "patch.import-toggle-file"
	toggleImportedHunk     commandID = "patch.import-toggle-hunk"
	selectImportedAll      commandID = "patch.import-select-all"
	clearImportedSelection commandID = "patch.import-clear-selection"
	toggleImportedReverse  commandID = "patch.import-toggle-reverse"
)

type patchImportRow struct{ file, hunk int }
type patchImportIdentity struct {
	path string
	hunk int
}

func isPatchSelectionCommand(id commandID) bool {
	switch id {
	case toggleImportedFile, toggleImportedHunk, selectImportedAll, clearImportedSelection, toggleImportedReverse:
		return true
	}
	return false
}

func isPatchImportCommand(id commandID) bool {
	return id == importPatch || id == previewImportedPatch || id == applyImportedPatch || isPatchSelectionCommand(id)
}

func (a *Workspace) patchSelectionDisabled(id commandID, target commandTarget) string {
	if id != toggleImportedFile && id != toggleImportedHunk {
		return ""
	}
	chosen, ok := a.patchImport.chosen[target.source]
	if !ok {
		return "Choose an imported patch file or hunk row"
	}
	if id == toggleImportedHunk && (target.index < 0 || target.index >= len(chosen)) {
		return "Choose an imported patch hunk row"
	}
	if id == toggleImportedHunk {
		for _, file := range a.patchImport.input.Files {
			if file.Path == target.source && file.Kind != "" {
				return "Select structural changes as whole files"
			}
		}
	}
	return ""
}

func (a *Workspace) runPatchSelection(id commandID, target commandTarget) {
	s := a.patchImport
	switch id {
	case toggleImportedReverse:
		s.reverse = !s.reverse
	case selectImportedAll, clearImportedSelection:
		for _, chosen := range s.chosen {
			for i := range chosen {
				chosen[i] = id == selectImportedAll
			}
		}
	case toggleImportedFile:
		chosen, ok := s.chosen[target.source]
		if !ok {
			return
		}
		all := true
		for _, yes := range chosen {
			all = all && yes
		}
		for i := range chosen {
			chosen[i] = !all
		}
	case toggleImportedHunk:
		chosen, ok := s.chosen[target.source]
		if !ok || target.index < 0 || target.index >= len(chosen) {
			return
		}
		chosen[target.index] = !chosen[target.index]
	}
	a.rebuildImportedPatch()
}

func (a *Workspace) patchSelectionList(c *ui.Context, p tokens) {
	s := a.patchImport
	busy := s.busy || s.picking || s.request != nil
	ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "Choose files and hunks").Grow(1)
		ui.Button(c, "Select All").Label(commandLabel(selectImportedAll)).Disabled(busy).OnClick(func() { a.run(selectImportedAll, commandTarget{}) })
		ui.Button(c, "Clear").Label(commandLabel(clearImportedSelection)).Disabled(busy).OnClick(func() { a.run(clearImportedSelection, commandTarget{}) })
	})
	s.pathsList.Selected = &s.selectedPath
	s.pathsList.Key = func(i int) any { row := s.rows[i]; return patchImportIdentity{s.input.Files[row.file].Path, row.hunk} }
	s.pathsList.Label = func(i int) string {
		row := s.rows[i]
		file := s.input.Files[row.file]
		destination, err := s.mappedPatchPath(file.Path)
		if err != nil {
			destination = "invalid mapping"
		}
		if row.hunk < 0 {
			if file.Kind != "" {
				return s.structuralPatchLabel(file)
			}
			return file.Path + " → " + destination
		}
		header, _, _ := strings.Cut(file.Hunks[row.hunk].Text, "\n")
		return file.Path + " → " + destination + " · " + header
	}
	ui.List(c, &s.pathsList, len(s.rows), func(i int) {
		row := s.rows[i]
		file := s.input.Files[row.file]
		destination, err := s.mappedPatchPath(file.Path)
		if err != nil {
			destination = "invalid mapping"
		}
		chosen := s.chosen[file.Path]
		label, checked, id := file.Path, true, toggleImportedFile
		indent := float32(0)
		if row.hunk >= 0 {
			label, _, _ = strings.Cut(file.Hunks[row.hunk].Text, "\n")
			checked, id, indent = chosen[row.hunk], toggleImportedHunk, inset*2
		} else {
			count := 0
			for _, yes := range chosen {
				if yes {
					count++
				}
			}
			checked = count == len(chosen)
			label = fmt.Sprintf("%s (%d/%d hunks)", file.Path, count, len(chosen))
			if destination != file.Path {
				label = fmt.Sprintf("%s → %s (%d/%d)", file.Path, destination, count, len(chosen))
			}
			if file.Kind != "" {
				label = s.structuralPatchLabel(file)
			}
		}
		target := commandTarget{source: file.Path, index: row.hunk}
		ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap, 0, gap+indent).AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c.Key("patch-choice"), &checked, label).Label(file.Path + " → " + destination + " · " + label).FillWidth().Disabled(busy).Tooltip(file.Path + " → " + destination).OnChange(func() { a.run(id, target) })
		})
	}).FillWidth().Height(min(5*rowHeight, float32(len(s.rows))*rowHeight)).Label("Imported patch file and hunk selection")
	if s.selectedPath >= 0 && s.selectedPath < len(s.rows) {
		row := s.rows[s.selectedPath]
		source := s.input.Files[row.file].Path
		destination, err := s.mappedPatchPath(source)
		if err != nil {
			destination = err.Error()
		}
		if s.input.Files[row.file].Kind == "R" {
			source = s.input.Files[row.file].OldPath + " → " + source
		}
		ui.ScrollHorizontal(c).FillWidth().Height(rowHeight).Label("Selected patch source and destination").Children(func() { ui.Text(c, source+" → "+destination).SingleLine() })
	}
}

func (s *patchImportDialog) structuralPatchLabel(file domain.PatchDocument) string {
	name, err := s.mappedPatchPath(file.Path)
	if err != nil {
		name = "invalid mapping"
	}
	kind := file.Kind
	if s.reverse {
		if kind == "A" {
			kind = "D"
		} else if kind == "D" {
			kind = "A"
		}
	}
	oldSize, newSize := file.OldSize, file.NewSize
	oldMode, newMode := file.OldMode, file.NewMode
	if s.reverse {
		oldSize, newSize = newSize, oldSize
		oldMode, newMode = newMode, oldMode
	}
	label := ""
	switch kind {
	case "A":
		label = "Add " + name + " (whole file)"
	case "D":
		label = "Delete " + name + " (whole file)"
	case "B":
		return fmt.Sprintf("Binary %s (%d → %d bytes; whole file)", name, oldSize, newSize)
	case "P":
		label = "Mode " + name + " " + oldMode + " → " + newMode + " (whole file)"
	case "R":
		old, err := s.mappedPatchPath(file.OldPath)
		if err != nil {
			old = "invalid mapping"
		}
		if s.reverse {
			old, name = name, old
		}
		label = "Rename " + old + " → " + name + " (whole file)"
		if oldMode != "" && newMode != "" && oldMode != newMode {
			label += " · " + oldMode + " → " + newMode
		}
	}
	if file.Binary {
		label += fmt.Sprintf(" · binary %d → %d bytes", oldSize, newSize)
	}
	return label
}
