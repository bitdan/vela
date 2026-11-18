package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	purgeShelf        commandID = "shelf.purge"
	previewShelfIndex commandID = "shelf.preview-index"
)

func isRealShelfCommand(id commandID) bool {
	switch id {
	case shelve, shelveSilently, unshelve, renameShelf, deleteShelf, restoreShelf, purgeShelf, previewShelfIndex:
		return true
	}
	return false
}

func (a *Workspace) hasDirtySource(path string) bool {
	if a.dirtySourcePath(path) {
		return true
	}
	entry := a.repo().split
	return entry != nil && entry.path == path && entry.source == "project" && entry.code != nil && entry.code.Buffer.Modified()
}

func (a *Workspace) realShelfEntry(t commandTarget) (domain.Shelf, bool) {
	entries := a.Model.Active().Repository.Shelf
	if t.shelfID != "" {
		for _, entry := range entries {
			if entry.ID == t.shelfID && entry.OID == t.shelfOID {
				return entry, true
			}
		}
		return domain.Shelf{}, false
	}
	if t.index < 0 || t.index >= len(entries) {
		return domain.Shelf{}, false
	}
	return entries[t.index], true
}

func (a *Workspace) shelfSelection(t commandTarget) []string {
	if t.file.Path == "" {
		t = a.selectedChange()
	}
	source, files := a.patchExportSelection(t)
	if source != "working" && source != "staged" {
		return nil
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
		if f.OldPath != "" {
			paths = append(paths, f.OldPath)
		}
	}
	slices.Sort(paths)
	return slices.Compact(paths)
}

func (a *Workspace) realShelfDisabled(id commandID, t commandTarget) string {
	repo := a.Model.Active().Repository
	if id == previewShelfIndex {
		if a.ReadRepositoryFile == nil || t.file.Path == "" {
			return "Select a shelved file"
		}
		if _, ok := a.realShelfEntry(t); !ok {
			return "Shelf changed; refresh the list"
		}
		return ""
	}
	if a.OperateRepository == nil || repo.IndexVersion == "" {
		return "Connect Git operations and refresh the repository"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || a.repositoryOperation != nil || a.indexConfirmation != nil || a.commitConfirmation != nil {
		return "Wait for the current Git operation"
	}
	if repo.ResolvingConflicts() || repo.Bisect.Active() {
		return "Finish the current Git operation first"
	}
	if id == shelve || id == shelveSilently {
		if repo.Head == "" {
			return "Create a commit before shelving"
		}
		paths := a.shelfSelection(t)
		if len(paths) == 0 || len(paths) > 100 {
			return "Select 1 to 100 tracked changed paths"
		}
		for _, path := range paths {
			if a.hasDirtySource(path) {
				return "Save or discard the unsaved source buffer first"
			}
			for _, f := range repo.Working {
				if f.Path == path && f.Status == "?" {
					return "Stage untracked files before shelving"
				}
			}
		}
		return ""
	}
	entry, ok := a.realShelfEntry(t)
	if !ok {
		return "Shelf changed; refresh the list"
	}
	if id == restoreShelf || id == purgeShelf {
		if !entry.Deleted {
			return "Select a recently deleted shelf"
		}
		return ""
	}
	if entry.Deleted {
		return "Restore the shelf first"
	}
	if id == unshelve {
		if t.source == "shelf-file" {
			if !slices.ContainsFunc(entry.Files, func(f domain.File) bool { return f.Path == t.file.Path }) {
				return "Select a file in the original shelf"
			}
		}
		if len(repo.Staged) > 0 || slices.ContainsFunc(repo.Working, func(f domain.File) bool { return f.Status != "?" }) {
			return "Commit, shelve or stash tracked changes before unshelving"
		}
		for _, f := range entry.Files {
			if a.hasDirtySource(f.Path) {
				return "Save or discard the unsaved source buffer first"
			}
		}
	}
	return ""
}

func (a *Workspace) runRealShelf(id commandID, t commandTarget) {
	if id == previewShelfIndex {
		entry, _ := a.realShelfEntry(t)
		t.source = "shelf-index:" + entry.ID
		a.readFileForCommand(openDiff, t)
		return
	}
	a.openRepositoryOperation(id, t)
}

func (a *Workspace) configureShelf(s *repositoryOperationDialog, id commandID, t commandTarget) {
	switch id {
	case shelve, shelveSilently:
		s.label = "Shelve"
		s.detail = "Save both staged and working versions of the selected paths, then restore those paths to HEAD. Unselected changes and Git Stash are kept."
		s.op = domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Shelved changes", Paths: a.shelfSelection(t)}
	default:
		entry, _ := a.realShelfEntry(t)
		s.op = domain.RepositoryOperation{Source: entry.OID, Target: entry.ID, Paths: make([]string, 0, len(entry.Files))}
		for _, f := range entry.Files {
			s.op.Paths = append(s.op.Paths, f.Path)
		}
		s.detail = entry.Title
		switch id {
		case unshelve:
			s.label = "Unshelve"
			s.op.Kind = domain.ApplyShelf
			s.op.RestoreIndex = true
			s.op.RemoveApplied = a.Layout.VersionControl.Shelf.RemoveApplied
			s.op.SelectShelfFiles = true
			s.shelfFiles = slices.Clone(entry.Files)
			s.shelfChosen = make(map[string]bool, len(entry.Files))
			for _, file := range entry.Files {
				s.shelfChosen[file.Path] = t.source != "shelf-file" || file.Path == t.file.Path
			}
			a.rebuildShelfSelection(s)
			s.detail += " · Apply checked saved file changes. Partial restore keeps the complete original shelf. Git may map target-side renames or report conflicts."
		case renameShelf:
			s.label = "Rename Shelf"
			s.op.Kind = domain.RenameShelf
			s.op.Message = entry.Title
		case deleteShelf:
			s.label = "Delete Shelf"
			s.op.Kind = domain.DeleteShelf
			s.detail += " · Move to Recently Deleted; working files stay unchanged."
		case restoreShelf:
			s.label = "Restore Shelf"
			s.op.Kind = domain.RestoreShelf
			s.detail += " · Restore the entry to the active shelf list."
		case purgeShelf:
			s.label = "Delete Shelf Permanently"
			s.op.Kind = domain.PurgeShelf
			s.detail += " · Remove the saved entry permanently. Working files stay unchanged."
		}
	}
}

func (a *Workspace) shelfFields(c *ui.Context, p tokens, s *repositoryOperationDialog, busy bool) {
	switch s.op.Kind {
	case domain.SaveShelf, domain.RenameShelf:
		ui.TextInput(c, &s.op.Message).Label("Shelf name").FillWidth().Disabled(busy)
	case domain.ApplyShelf:
		a.shelfApplyFiles(c, p, s, busy)
		ui.Checkbox(c, &s.op.RestoreIndex, "Restore original staged state").Disabled(busy)
		partial := len(s.op.Paths) < len(s.shelfFiles)
		ui.Checkbox(c, &s.op.RemoveApplied, "Move complete entry to Recently Deleted after successful apply").Disabled(busy || partial)
		if partial {
			ui.Text(c, "Partial restore retains every saved file in the original shelf, including applied files. Rename source/destination paths are listed separately; select both to restore the complete rename.").FillWidth().TextColor(p.theme.TextMuted)
		}
		if reason := a.shelfApplyReason(s); reason != "" {
			ui.Text(c, reason).FillWidth().TextColor(p.theme.TextMuted)
		}
		return
	case domain.PurgeShelf:
		ui.Checkbox(c, &s.acknowledged, "Permanently delete this saved shelf").Disabled(busy)
	default:
		return
	}
	if len(s.op.Paths) > 0 {
		ui.Text(c, fmt.Sprintf("%d paths", len(s.op.Paths)))
		ui.Scroll(c).FillWidth().MaxHeight(100).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, path := range s.op.Paths {
					ui.Text(c, path).SingleLine().Ellipsis("…").FillWidth()
				}
			})
		})
	}
}

func shelfFieldsDisabled(s *repositoryOperationDialog) bool {
	if s.op.Kind == domain.ApplyShelf {
		return len(s.op.Paths) == 0
	}
	if s.op.Kind == domain.PurgeShelf {
		return !s.acknowledged
	}
	return (s.op.Kind == domain.SaveShelf || s.op.Kind == domain.RenameShelf) && (strings.TrimSpace(s.op.Message) == "" || len(s.op.Message) > 200 || strings.ContainsAny(s.op.Message, "\x00\r\n"))
}
