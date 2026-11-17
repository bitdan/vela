package ui

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

const (
	previewImportedPatch commandID = "patch.import-preview"
	applyImportedPatch   commandID = "patch.import-apply"
)

type patchImportDialog struct {
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
	if repo.Root == "" || a.PickPatchSource == nil || a.PreviewImportedPatch == nil || a.ApplyImportedPatch == nil {
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
	if id == applyImportedPatch {
		if s.doc.Version == "" || s.doc.Target != s.target || !s.ack {
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
		text := doc.Text
		if len(text) > 256<<10 {
			text = text[:256<<10]
			for !utf8.ValidString(text) {
				text = text[:len(text)-1]
			}
		}
		s.preview = &editor.State{Buffer: editor.NewBuffer(text), ReadOnly: true}
	})
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
				ui.Text(c, "Existing tracked LF text modifications only · up to 2 MiB / 100 files. No additions, deletions, renames, binary, links, modes or email patches.").FillWidth().TextColor(p.theme.TextMuted)
				ui.Select(c, &s.target, []string{"Working tree", "Index only"}).Label("Patch application target").FillWidth().Disabled(s.busy || s.request != nil || s.picking)
				ui.Text(c, "Working tree changes local files; Index only stages the patch without changing local files. HEAD is preserved. Paths are relative to the repository root; strip depth is fixed at 1.").FillWidth().TextColor(p.theme.TextMuted)
				if s.picking || s.request != nil {
					ui.Text(c, "Reading and checking patch…")
				}
				if s.preview != nil {
					ui.Text(c, fmt.Sprintf("%d files · %d hunks · %d bytes · %s", len(s.doc.Paths), s.doc.Hunks, len(s.doc.Text), s.doc.Target)).FillWidth()
					s.pathsList.Selected = &s.selectedPath
					s.pathsList.Key = func(i int) any { return s.doc.Paths[i] }
					s.pathsList.Label = func(i int) string { return s.doc.Paths[i] }
					ui.List(c, &s.pathsList, len(s.doc.Paths), func(i int) {
						ui.Text(c, s.doc.Paths[i]).FillWidth().Height(rowHeight).SingleLine().Ellipsis("…").Tooltip(s.doc.Paths[i])
					}).FillWidth().Height(min(4*rowHeight, float32(len(s.doc.Paths))*rowHeight)).Label("All imported patch paths")
					if s.selectedPath >= 0 && s.selectedPath < len(s.doc.Paths) {
						ui.ScrollHorizontal(c).FillWidth().Height(rowHeight + gap).Label("Complete selected patch path").Children(func() {
							ui.Text(c, s.doc.Paths[s.selectedPath]).SingleLine()
						})
					}
					if len(s.doc.Text) > 256<<10 {
						ui.Text(c, "Text preview limited to 256 KiB; all listed files and hunks will be applied.").FillWidth().TextColor(p.theme.TextMuted)
					}
					editor.View(c, s.preview).Label("Imported patch preview").FillWidth().Grow(1)
				}
				ui.Checkbox(c, &s.ack, "Apply all previewed changes to the selected target").Disabled(s.busy || s.request != nil || s.doc.Version == "" || s.doc.Target != s.target)
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
