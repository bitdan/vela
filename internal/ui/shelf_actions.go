package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

func (a *Workspace) shelfDisabled(id commandID, t commandTarget) string {
	repo := a.Model.Active().Repository
	if t.index < 0 || t.index >= len(repo.Shelf) {
		return "Select a shelf entry"
	}
	entry := repo.Shelf[t.index]
	if id == restoreShelf {
		if !entry.Deleted {
			return "Select a recently deleted entry"
		}
		return ""
	}
	if entry.Deleted {
		return "Restore the entry first"
	}
	if id != unshelve {
		return ""
	}
	files := shelfFiles(entry, t)
	if len(files) == 0 {
		return "No changes to unshelve"
	}
	for _, file := range files {
		if _, ok := matchFile(repo.Working, file.Path); ok {
			return "A working change already exists for " + file.Path
		}
		if _, ok := matchFile(repo.Staged, file.Path); ok {
			return "A staged change already exists for " + file.Path
		}
	}
	return ""
}

func shelfFiles(entry domain.Shelf, t commandTarget) []domain.File {
	if t.source != "shelf-file" {
		return entry.Files
	}
	if f, ok := matchFile(entry.Files, t.file.Path); ok {
		return []domain.File{f}
	}
	return nil
}

func (a *Workspace) shelfPrompts(c *ui.Context) {
	r := a.repo()
	if t := r.shelfRename; t != nil {
		ui.Row(c).FillWidth().Padding(gap).Gap(gap).Children(func() {
			input := ui.TextInput(c, &r.shelfName).Grow(1).Label("Shelf name").AutoFocus()
			save := func() {
				name := strings.TrimSpace(r.shelfName)
				if name != "" && a.shelfDisabled(renameShelf, *t) == "" {
					a.Model.Active().Repository.Shelf[t.index].Title = name
					r.shelfRename = nil
					a.record("Renamed mock shelf · " + name)
				}
			}
			if input.Submitted() {
				save()
			}
			ui.Button(c, "Rename").OnClick(save)
			ui.Button(c, "Cancel").OnClick(func() { r.shelfRename = nil })
		})
	}
	if t := r.shelfApply; t != nil {
		open := true
		remove := a.Layout.VersionControl.Shelf.RemoveApplied
		message := "Copy the selected changes to Working Changes and keep the shelf entry."
		if remove {
			message = "Copy the selected changes to Working Changes and remove the applied files from the active shelf."
		}
		if ui.AlertDialog(c, &open, "Unshelve mock changes?", message, "Cancel", "Unshelve") == 1 {
			if a.shelfDisabled(unshelve, *t) == "" {
				repo := &a.Model.Active().Repository
				entry := &repo.Shelf[t.index]
				repo.Working = append(repo.Working, shelfFiles(*entry, *t)...)
				if remove {
					if t.source == "shelf-file" && len(entry.Files) > 1 {
						var remaining []domain.File
						for _, file := range entry.Files {
							if file.Path != t.file.Path {
								remaining = append(remaining, file)
							}
						}
						entry.Files = remaining
					} else {
						entry.Deleted = true
					}
					r.shelfIndex = -1
				}
				a.record("Unshelved mock changes")
			}
		}
		if !open {
			r.shelfApply = nil
		}
	}
}
