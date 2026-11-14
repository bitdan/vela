package ui

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

const savePatchFile commandID = "patch.save-file"

type patchExportDialog struct {
	open, busy, picking, truncated bool
	snapshot                       domain.Repository
	doc                            domain.PatchExport
	preview                        *editor.State
	request                        *readRequest
	cancel                         context.CancelFunc
	message                        string
}

func (a *Workspace) stopPatchExports() {
	stopRead(&a.patchCopyRead)
	if s := a.patchExport; s != nil && !s.busy {
		stopRead(&s.request)
		a.patchExport = nil
	}
}

func (a *Workspace) queueClipboard(value, notice string) {
	stopRead(&a.patchCopyRead)
	a.clipboard = &value
	a.clipboardNotice = notice
}

func (a *Workspace) patchExportSelection(target commandTarget) (string, []domain.File) {
	if target.file.Path == "" {
		if entry := a.activeDocument(); entry != nil {
			target.file, target.source = entry.file, entry.source
		}
	}
	if target.source == "project" || target.source == "editor" {
		source, file := a.projectChange(target.file.Path)
		if source == "" {
			return "", nil
		}
		return source, []domain.File{file}
	}
	if target.source == "working" || target.source == "staged" {
		return target.source, a.indexFiles(target.source == "working", target)
	}
	source := strings.TrimPrefix(target.source, "revision:")
	if fixedObjectID(source) && target.file.Path != "" {
		return source, []domain.File{target.file}
	}
	return "", nil
}

func (a *Workspace) patchExportDisabled(id commandID, target commandTarget) string {
	if a.ReadRepositoryChangesPatch == nil {
		return "Patch export is unavailable"
	}
	if id == createPatch && (a.PickPatchDestination == nil || a.WritePatchFile == nil) {
		return "Patch file export is unavailable"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || a.patchExport != nil || a.patchCopyRead != nil {
		return "Wait for the current repository task"
	}
	if a.Model.Active().Repository.IndexVersion == "" {
		return "Refresh the repository"
	}
	_, files := a.patchExportSelection(target)
	if len(files) == 0 || len(files) > 100 {
		return "Select 1 to 100 changed files"
	}
	for _, file := range files {
		if file.Status == "?" || file.Status == "U" {
			return "Stage new files and resolve conflicts before exporting"
		}
	}
	return ""
}

func (a *Workspace) requestPatchExport(id commandID, target commandTarget) {
	source, files := a.patchExportSelection(target)
	repo := a.Model.Active().Repository
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	navigation := a.repositoryNavigation
	var dialog *patchExportDialog
	if id == createPatch {
		dialog = &patchExportDialog{open: true, snapshot: repo, request: request}
		a.patchExport = dialog
	} else {
		a.patchCopyRead = request
		a.status = "Reading Git patch for clipboard…"
	}
	a.ReadRepositoryChangesPatch(ctx, repo, source, files, func(doc domain.PatchExport, err error) {
		if ctx.Err() == context.Canceled {
			return
		}
		if dialog != nil {
			if a.patchExport != dialog || dialog.request != request {
				return
			}
			dialog.request = nil
		} else {
			if a.patchCopyRead != request || navigation != a.repositoryNavigation || a.Model.Active().Repository.Root != repo.Root {
				return
			}
			a.patchCopyRead = nil
		}
		if err == nil {
			err = ctx.Err()
		}
		cancel()
		if err != nil {
			message := "Could not read patch · " + err.Error()
			if dialog != nil {
				dialog.message = message
			} else {
				a.record(message)
			}
			return
		}
		if doc.Text == "" || !utf8.ValidString(doc.Text) || strings.ContainsRune(doc.Text, 0) {
			if dialog != nil {
				dialog.message = "Git returned no usable patch"
			} else {
				a.record("Git returned no usable patch")
			}
			return
		}
		if dialog == nil {
			a.queueClipboard(doc.Text, fmt.Sprintf("Copied Git patch · %d files · %d bytes", len(doc.Paths), len(doc.Text)))
			return
		}
		dialog.doc = doc
		preview := doc.Text
		const previewLimit = 256 << 10
		if len(preview) > previewLimit {
			preview = preview[:previewLimit]
			for !utf8.ValidString(preview) {
				preview = preview[:len(preview)-1]
			}
			dialog.truncated = true
		}
		dialog.preview = &editor.State{Buffer: editor.NewBuffer(preview), ReadOnly: true}
	})
}

func (a *Workspace) patchSaveDisabled() string {
	s := a.patchExport
	if s == nil || s.preview == nil || s.doc.Text == "" {
		return "Wait for a patch preview"
	}
	if s.busy || s.picking || a.repositoryWrite != "" {
		return "Wait for the current file task"
	}
	if s.snapshot.Root != a.Model.Active().Repository.Root {
		return "Return to the patch repository"
	}
	if a.PickPatchDestination == nil || a.WritePatchFile == nil {
		return "Patch destination picker is unavailable"
	}
	return ""
}

func (a *Workspace) choosePatchDestination() {
	s := a.patchExport
	s.picking = true
	a.PickPatchDestination(func(path string, err error) {
		if a.patchExport != s {
			return
		}
		s.picking = false
		if err != nil {
			s.message = "Could not choose patch destination · " + err.Error()
			return
		}
		if path == "" {
			return
		}
		if s.snapshot.Root != a.Model.Active().Repository.Root {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		s.cancel = cancel
		s.busy = true
		s.message = ""
		a.beginRepositoryWrite(s.snapshot.Root, "Creating patch file…", cancel)
		a.WritePatchFile(ctx, s.snapshot, path, s.doc.Text, func(updated domain.Repository, err, refreshErr error) {
			wasCanceled := ctx.Err() != nil
			cancel()
			s.cancel = nil
			s.busy = false
			message := "Created patch · " + path
			if err != nil {
				message = "Could not create patch · " + err.Error()
				s.message = message
			} else {
				if wasCanceled {
					message = "Patch created after cancellation · " + path
				}
				s.open = false
			}
			a.finishRepositoryWrite(s.snapshot.Root, updated, refreshErr, message)
		})
	})
}

func (a *Workspace) patchExportView(c *ui.Context, p tokens) {
	s := a.patchExport
	w, h := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 900)).Height(min(h-32, 680)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Create Patch dialog").Column().Children(func() {
			heading(c, p, "Create Git Patch", "")
			ui.Column(c).Grow(1).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.snapshot.Root).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, "Captured Git snapshot. Choose a new .patch or .diff file; existing files are kept. Export does not stage or commit.").FillWidth().TextColor(p.theme.TextMuted)
				if s.request != nil {
					ui.Text(c, "Reading selected Git changes…")
				} else if s.preview != nil {
					ui.Text(c, fmt.Sprintf("%s · %d files · %d bytes", s.doc.Source, len(s.doc.Paths), len(s.doc.Text))).FillWidth()
					ui.Text(c, strings.Join(s.doc.Paths, ", ")).FillWidth().SingleLine().Ellipsis("…")
					if s.truncated {
						ui.Text(c, "Preview limited to 256 KiB; the exported file contains the complete patch.").FillWidth().TextColor(p.theme.TextMuted)
					}
					editor.View(c, s.preview).Label("Export patch preview").FillWidth().Grow(1)
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").Disabled(s.picking).OnClick(func() {
					if s.busy {
						if s.cancel != nil {
							s.cancel()
						}
						s.message = "Cancellation requested · waiting for file task and refresh"
					} else {
						s.open = false
					}
				})
				ui.PrimaryButton(c, "Save Patch…").Label("Save Patch File").Disabled(a.patchSaveDisabled() != "").OnClick(func() { a.run(savePatchFile, commandTarget{}) })
			})
		})
	})
	if !s.open {
		if s.busy {
			if s.cancel != nil {
				s.cancel()
			}
			s.open = true
		} else {
			stopRead(&s.request)
			a.patchExport = nil
		}
	}
}
