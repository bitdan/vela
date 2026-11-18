package ui

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	previewImportedPatch commandID = "patch.import-preview"
	applyImportedPatch   commandID = "patch.import-apply"
)

type patchImportDialog struct {
	input                    domain.ImportedPatch
	chosen                   map[string][]bool
	rows                     []patchImportRow
	reverse                  bool
	open, picking, busy, ack bool
	snapshot                 domain.Repository
	source, target, message  string
	doc                      domain.ImportedPatch
	preview                  *editor.State
	pathsList                ui.ListState
	selectedPath             int
	request                  *readRequest
}

func (a *Workspace) stopPatchImport() {
	if s := a.patchImport; s != nil && !s.busy {
		stopRead(&s.request)
		a.patchImport = nil
	}
}

func (a *Workspace) patchImportDisabled(id commandID) string {
	repo := a.Model.Active().Repository
	if repo.Root == "" || a.PickPatchSource == nil || a.PreviewImportedPatch == nil || a.ApplyImportedPatch == nil || a.ReviewImportedPatch == nil {
		return "Open a local repository with patch import available"
	}
	if repo.IndexVersion == "" || repo.WorktreeVersion == "" {
		return "Refresh the repository first"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || repo.ResolvingConflicts() || repo.Bisect.Active() {
		return "Finish the current repository operation first"
	}
	if id == importPatch {
		if a.patchImport != nil || a.patchExport != nil || a.patchDialog != nil || a.repositoryOperation != nil || a.indexConfirmation != nil || a.commitConfirmation != nil || a.repositoryInit != nil || a.repositoryClone != nil || a.projectFile != nil || a.sourceDecision != nil {
			return "Close the current repository dialog first"
		}
		return ""
	}
	s := a.patchImport
	if s == nil || s.source == "" || s.busy || s.picking || s.request != nil || s.snapshot.Root != repo.Root {
		return "Wait for a patch file in the original repository"
	}
	if isPatchSelectionCommand(id) && s.input.Text == "" {
		return "Read a patch before selecting its changes"
	}
	if (id == previewImportedPatch || id == applyImportedPatch) && s.input.Text != "" && s.doc.Text == "" {
		return "Select at least one patch hunk"
	}
	if id == applyImportedPatch {
		if s.doc.Version == "" || s.doc.Target != s.target || s.doc.Reverse != s.reverse || !s.ack {
			return "Preview this target and confirm applying the patch"
		}
		for _, path := range s.doc.Paths {
			if a.projectFileDirty(repo.Root, path) {
				return "Save or discard unsaved target documents first"
			}
			if entry := a.repo().preview; entry != nil && entry.source == "project" && projectPathContains(path, entry.path) && entry.modified() {
				return "Save or discard unsaved target documents first"
			}
		}
	}
	return ""
}

func (a *Workspace) choosePatchSource() {
	a.stopPatchExports()
	s := &patchImportDialog{open: true, picking: true, target: "Working tree", snapshot: a.Model.Active().Repository}
	a.patchImport = s
	navigation := a.repositoryNavigation
	a.PickPatchSource(func(path string, err error) {
		if a.patchImport != s || navigation != a.repositoryNavigation || s.snapshot.Root != a.Model.Active().Repository.Root {
			return
		}
		s.picking = false
		if err != nil {
			s.message = "Could not choose patch · " + err.Error()
			return
		}
		if path == "" {
			a.stopPatchImport()
			return
		}
		s.source = path
		a.previewPatchImport()
	})
}

func (a *Workspace) previewPatchImport() {
	if a.patchImportDisabled(previewImportedPatch) != "" {
		return
	}
	s := a.patchImport
	if s.input.Text != "" {
		a.reviewSelectedPatch()
		return
	}
	// An explicit new preview may follow a failed apply and its repository
	// refresh. Capture that refreshed identity before asking for confirmation.
	s.snapshot = a.Model.Active().Repository
	s.doc, s.preview, s.ack, s.message = domain.ImportedPatch{}, nil, false, ""
	s.pathsList, s.selectedPath = ui.ListState{}, 0
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	target, navigation := s.target, a.repositoryNavigation
	a.PreviewImportedPatch(ctx, s.snapshot, s.source, target, func(doc domain.ImportedPatch, err error) {
		if a.patchImport != s || s.request != request || navigation != a.repositoryNavigation || s.snapshot.Root != a.Model.Active().Repository.Root {
			return
		}
		s.request = nil
		if err == nil {
			err = ctx.Err()
		}
		cancel()
		if err != nil {
			s.message = "Could not preview patch · " + err.Error()
			doc.Version = ""
		}
		if doc.Text == "" {
			return
		}
		s.doc = doc
		s.input = doc
		s.chosen = make(map[string][]bool)
		for i, file := range doc.Files {
			s.rows = append(s.rows, patchImportRow{file: i, hunk: -1})
			s.chosen[file.Path] = make([]bool, len(file.Hunks))
			for j := range file.Hunks {
				s.chosen[file.Path][j] = true
				s.rows = append(s.rows, patchImportRow{file: i, hunk: j})
			}
		}
		s.setPatchPreview(doc.Text)
	})
}

func (s *patchImportDialog) setPatchPreview(text string) {
	if len(text) > 256<<10 {
		text = text[:256<<10]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	s.preview = &editor.State{Buffer: editor.NewBuffer(text), ReadOnly: true}
}

func (a *Workspace) reviewSelectedPatch() {
	s := a.patchImport
	a.rebuildImportedPatch()
	if s.doc.Text == "" {
		return
	}
	s.snapshot = a.Model.Active().Repository
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	input, navigation := s.doc, a.repositoryNavigation
	a.ReviewImportedPatch(ctx, s.snapshot, input, func(doc domain.ImportedPatch, err error) {
		if a.patchImport != s || s.request != request || navigation != a.repositoryNavigation || s.snapshot.Root != a.Model.Active().Repository.Root {
			return
		}
		s.request = nil
		if err == nil {
			err = ctx.Err()
		}
		cancel()
		if err != nil {
			s.message = "Could not check selected patch · " + err.Error()
			s.doc.Version = ""
			return
		}
		s.doc, s.message = doc, ""
	})
}

func (a *Workspace) rebuildImportedPatch() {
	s := a.patchImport
	s.ack, s.message = false, ""
	selection := make(map[string][]int)
	for path, chosen := range s.chosen {
		for i, yes := range chosen {
			if yes {
				selection[path] = append(selection[path], i)
			}
		}
	}
	doc, err := application.SelectImportedPatch(s.input.Text, selection)
	doc.Target, doc.Reverse = s.target, s.reverse
	s.doc = doc
	if err != nil {
		s.message = err.Error()
	}
	s.setPatchPreview(doc.Text)
}

func (a *Workspace) applyPatchImport() {
	if a.patchImportDisabled(applyImportedPatch) != "" {
		return
	}
	s := a.patchImport
	locked := map[*editor.State]bool{}
	lock := func(entry *tab) {
		if entry == nil || entry.source != "project" || entry.code == nil {
			return
		}
		for _, path := range s.doc.Paths {
			if projectPathContains(path, entry.path) {
				if _, ok := locked[entry.code]; !ok {
					locked[entry.code] = entry.code.ReadOnly
					entry.code.ReadOnly = true
				}
			}
		}
	}
	for i := range a.repo().tabs {
		lock(&a.repo().tabs[i])
	}
	lock(a.repo().split)
	lock(a.repo().preview)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	s.busy, s.message = true, ""
	a.beginRepositoryWrite(s.snapshot.Root, "Applying imported patch…", cancel)
	a.ApplyImportedPatch(ctx, s.snapshot, s.doc, func(updated domain.Repository, err, refreshErr error) {
		canceled := ctx.Err() != nil
		cancel()
		s.busy = false
		for state, readOnly := range locked {
			state.ReadOnly = readOnly
		}
		message := "Applied patch to " + s.doc.Target
		if s.doc.Reverse {
			message = "Applied reverse patch to " + s.doc.Target
		}
		if err != nil {
			message = "Patch import stopped · " + err.Error()
			s.message = message
			s.doc.Version, s.ack = "", false
		} else {
			if canceled {
				message += " · completed after cancellation request"
			}
			a.patchImport = nil
		}
		a.finishRepositoryWrite(s.snapshot.Root, updated, refreshErr, message)
		// Refresh even after failure/cancellation: Git may have written files
		// before interruption. The existing reader preserves subsequent edits.
		for _, path := range s.doc.Paths {
			a.refreshOpenFileViews(s.snapshot.Root, path)
		}
	})
}

func (a *Workspace) patchImportView(c *ui.Context, p tokens) {
	s := a.patchImport
	w, h := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 900)).Height(min(h-32, 700)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Import Patch dialog").Column().Children(func() {
			heading(c, p, "Import Git Patch", "")
			ui.Column(c).Grow(1).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.snapshot.Root).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, s.source).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, "Existing tracked LF text modifications · up to 2 MiB / 100 files / 10,000 hunks. Select files/hunks or apply in reverse. Additions, deletions, renames, binary, links, modes and email patches remain unsupported.").FillWidth().TextColor(p.theme.TextMuted)
				if ui.Select(c, &s.target, []string{"Working tree", "Index only"}).Label("Patch application target").FillWidth().Disabled(s.busy || s.request != nil || s.picking).Changed() && s.input.Text != "" {
					a.rebuildImportedPatch()
				}
				reverse := s.reverse
				ui.Checkbox(c, &reverse, "Apply in reverse (undo selected patch changes)").Label(commandLabel(toggleImportedReverse)).Disabled(s.busy || s.request != nil || s.picking || s.input.Text == "").OnChange(func() { a.run(toggleImportedReverse, commandTarget{}) })
				ui.Text(c, "Working tree changes local files; Index only stages the patch without changing local files. HEAD is preserved. Paths are relative to the repository root; strip depth is fixed at 1.").FillWidth().TextColor(p.theme.TextMuted)
				if s.picking || s.request != nil {
					ui.Text(c, "Reading and checking patch…")
				}
				if s.preview != nil {
					ui.Text(c, fmt.Sprintf("%d files · %d hunks · %d bytes · %s", len(s.doc.Paths), s.doc.Hunks, len(s.doc.Text), s.doc.Target)).FillWidth()
					a.patchSelectionList(c, p)
					if s.reverse {
						ui.Text(c, "Source diff shown below; Reverse applies the inverse of the selected hunks.").FillWidth().TextColor(p.theme.TextMuted)
					}
					if len(s.doc.Text) > 256<<10 {
						ui.Text(c, "Text preview limited to 256 KiB; only selected files and hunks will be applied.").FillWidth().TextColor(p.theme.TextMuted)
					}
					editor.View(c, s.preview).Label("Imported patch preview").FillWidth().Grow(1)
				}
				ui.Checkbox(c, &s.ack, "Apply the checked selection in the chosen direction and target").Disabled(s.busy || s.request != nil || s.doc.Version == "" || s.doc.Target != s.target || s.doc.Reverse != s.reverse)
				if reason := a.patchImportDisabled(applyImportedPatch); reason != "" && s.doc.Version != "" {
					ui.Text(c, reason).FillWidth().TextColor(p.theme.TextMuted)
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					if s.busy {
						a.run(cancelGitTask, commandTarget{})
						s.message = "Cancellation requested · waiting for Git and refresh"
					} else {
						a.stopPatchImport()
					}
				})
				ui.Button(c, "Preview Target").Disabled(a.patchImportDisabled(previewImportedPatch) != "").OnClick(func() { a.run(previewImportedPatch, commandTarget{}) })
				ui.PrimaryButton(c, "Apply Patch").Disabled(a.patchImportDisabled(applyImportedPatch) != "").OnClick(func() { a.run(applyImportedPatch, commandTarget{}) })
			})
		})
	})
	if !s.open {
		if s.busy {
			a.run(cancelGitTask, commandTarget{})
			s.open = true
		} else {
			a.stopPatchImport()
		}
	}
}
