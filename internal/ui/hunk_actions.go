package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	stageHunk   commandID = "git.stage-hunk"
	unstageHunk commandID = "git.unstage-hunk"
	discardHunk commandID = "git.discard-hunk"
)

func isHunkCommand(id commandID) bool {
	return id == stageHunk || id == unstageHunk || id == discardHunk
}

type patchDialog struct {
	lineMode                 bool
	lineError                string
	lineChoices              map[int][]bool
	lineRows                 map[int][]application.ChangedPatchLine
	lineList                 ui.ListState
	open, reading, busy, ack bool
	id                       commandID
	snapshot                 domain.Repository
	doc                      domain.PatchDocument
	choice, shown, message   string
	choices                  []string
	preview                  *editor.State
	cancel                   context.CancelFunc
}

func (a *Workspace) hunkTarget(target commandTarget) commandTarget {
	if target.file.Path == "" {
		if entry := a.activeDocument(); entry != nil {
			target.file, target.source = entry.file, entry.source
		}
	}
	if target.source == "project" {
		target.source = "working"
	}
	if file, ok := findFile(a.Model.Active().Repository, target.source, target.file.Path); ok {
		target.file = file
	} else if target.source == "working" || target.source == "staged" {
		target.file.Status = ""
	}
	return target
}

func (a *Workspace) dirtySourcePath(path string) bool {
	for _, entry := range a.repo().tabs {
		if entry.path == path && entry.source == "project" && entry.code != nil && entry.code.Buffer.Modified() {
			return true
		}
	}
	return false
}

func (a *Workspace) hunkDisabled(id commandID, target commandTarget) string {
	repo := a.Model.Active().Repository
	target = a.hunkTarget(target)
	switch {
	case repo.Root == "" || a.ReadRepositoryPatch == nil || a.OperateRepository == nil:
		return "Connect a local Git repository"
	case repo.IndexVersion == "":
		return "Refresh the repository"
	case a.repositoryWrite != "" || a.repositoryRead != nil || a.patchDialog != nil || a.repositoryOperation != nil || a.repositoryClone != nil || a.indexConfirmation != nil || a.commitConfirmation != nil:
		return "Wait for the current Git operation"
	case repo.ResolvingConflicts() || repo.Bisect.Active():
		return "Finish the current Git operation first"
	case target.file.Path == "" || target.file.Status != "M" || target.file.OldPath != "":
		return "Select an ordinary tracked modified file"
	case id == unstageHunk && target.source != "staged":
		return "Select a staged change"
	case id != unstageHunk && target.source != "working":
		return "Select a working change"
	case id == discardHunk && a.dirtySourcePath(target.file.Path):
		return "Save or discard the unsaved source buffer first"
	}
	return ""
}

func (a *Workspace) openHunkDialog(id commandID, target commandTarget) {
	target = a.hunkTarget(target)
	s := &patchDialog{open: true, reading: true, id: id, snapshot: a.Model.Active().Repository}
	a.patchDialog = s
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	s.cancel = cancel
	a.ReadRepositoryPatch(ctx, s.snapshot, target.source, target.file.Path, func(doc domain.PatchDocument, err error) {
		if a.patchDialog != s || !s.reading || ctx.Err() == context.Canceled {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		cancel()
		s.cancel = nil
		s.reading = false
		if err != nil {
			s.message = "Could not read Git hunks · " + err.Error()
			return
		}
		if doc.Path != target.file.Path || doc.Source != target.source || doc.Version == "" || len(doc.Hunks) == 0 {
			s.message = "Git returned no usable hunks"
			return
		}
		s.doc = doc
		for n, h := range doc.Hunks {
			s.choices = append(s.choices, fmt.Sprintf("%d · -%d,%d +%d,%d", n+1, h.OldStart, h.OldCount, h.NewStart, h.NewCount))
		}
		s.choice = s.choices[0]
		s.updatePreview()
	})
}

func (s *patchDialog) updatePreview() {
	if s.shown == s.choice && s.preview != nil {
		return
	}
	n := slices.Index(s.choices, s.choice)
	if n < 0 || n >= len(s.doc.Hunks) {
		return
	}
	raw := s.doc.Header + s.doc.Hunks[n].Text
	s.lineError = ""
	if s.lineMode {
		selected, err := application.SelectPatchLines(s.doc, n, s.chosenLineOrdinals(), s.id == unstageHunk)
		if err != nil {
			s.lineError = err.Error()
		} else {
			raw = selected
		}
	}
	s.preview = &editor.State{Buffer: editor.NewBuffer(raw), ReadOnly: true}
	s.shown = s.choice
}

func (s *patchDialog) label() string {
	switch s.id {
	case stageHunk:
		return "Stage Hunk"
	case unstageHunk:
		return "Unstage Hunk"
	default:
		return "Discard Hunk"
	}
}

func (a *Workspace) patchDialogView(c *ui.Context, p tokens) {
	s := a.patchDialog
	w, h := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 800)).Height(min(h-32, 660)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Git Hunk dialog").Column().Children(func() {
			heading(c, p, s.label(), "")
			ui.Column(c).Grow(1).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.snapshot.Root).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, s.doc.Path).FillWidth().SingleLine().Ellipsis("…")
				detail := "Apply one freshly verified Git hunk to the index. Working files and HEAD are kept."
				if s.id == unstageHunk {
					detail = "Reverse one freshly verified Git hunk in the index. Working files and HEAD are kept."
				}
				if s.id == discardHunk {
					detail = "Permanently discard this working-tree hunk. Other hunks, index and HEAD are kept. This cannot be undone in Vela."
				}
				if s.lineMode {
					detail = "Apply only the checked changed lines to the index. Unchecked changes, working files and HEAD are kept."
				}
				ui.Text(c, detail).FillWidth().TextColor(p.theme.TextMuted)
				if s.reading {
					ui.Text(c, "Reading Git hunks…")
				} else if len(s.choices) > 0 {
					ui.Select(c, &s.choice, s.choices).Label("Git hunk").FillWidth().Disabled(s.busy)
					s.updatePreview()
					a.hunkLineControls(c, s)
					editor.View(c, s.preview).Label("Git patch preview").FillWidth().Grow(1)
				}
				if s.id == discardHunk {
					ui.Checkbox(c, &s.ack, "Discard selected working changes permanently").Disabled(s.busy)
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					if s.cancel != nil {
						s.cancel()
					}
					if s.busy {
						s.message = "Cancellation requested · waiting for Git and refresh"
					} else {
						s.open = false
					}
				})
				reason := a.disabledFor(applyHunkSelection, commandTarget{})
				label := s.label()
				if s.lineMode {
					label = "Stage Selected Lines"
					if s.id == unstageHunk {
						label = "Unstage Selected Lines"
					}
				}
				ui.PrimaryButton(c, label).Label("Run " + s.label()).Disabled(reason != "").Tooltip(reason).OnClick(func() { a.run(applyHunkSelection, commandTarget{}) })
			})
		})
	})
	if !s.open {
		if s.cancel != nil {
			s.cancel()
		}
		if s.busy {
			s.open = true
		} else {
			a.patchDialog = nil
		}
	}
}

func (a *Workspace) applySelectedHunk(s *patchDialog) {
	if s.busy || s.reading || s.doc.Version == "" || a.repositoryWrite != "" || s.snapshot.Root != a.Model.Active().Repository.Root {
		return
	}
	if s.id == discardHunk && (!s.ack || a.dirtySourcePath(s.doc.Path)) {
		s.message = "Save or discard the unsaved source buffer first"
		return
	}
	n := slices.Index(s.choices, s.choice)
	if n < 0 {
		return
	}
	kind := domain.StageHunk
	if s.id == unstageHunk {
		kind = domain.UnstageHunk
	} else if s.id == discardHunk {
		kind = domain.DiscardHunk
	}
	op := domain.RepositoryOperation{Kind: kind, Path: s.doc.Path, PatchVersion: s.doc.Version, HunkIndex: n}
	if s.lineMode {
		op.SelectPatchLines = true
		op.PatchLines = s.chosenLineOrdinals()
		if _, err := application.SelectPatchLines(s.doc, n, op.PatchLines, s.id == unstageHunk); err != nil {
			s.message = err.Error()
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	s.cancel = cancel
	s.busy = true
	a.beginRepositoryWrite(s.snapshot.Root, s.label()+"…", cancel)
	a.OperateRepository(ctx, s.snapshot, op, func(updated domain.Repository, err, refreshErr error) {
		cancel()
		s.cancel = nil
		s.busy = false
		message := s.label() + " completed"
		if err != nil {
			message = "Could not " + s.label() + " · " + err.Error()
			s.message = message
			s.doc.Version = ""
		} else {
			s.open = false
		}
		a.finishRepositoryWrite(s.snapshot.Root, updated, refreshErr, message)
		if err == nil && refreshErr == nil {
			a.refreshOpenFileViews(s.snapshot.Root, s.doc.Path)
		}
	})
}
