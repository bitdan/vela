package ui

import (
	"context"
	"reflect"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

const (
	keepDraftsClose    commandID = "workspace.keep-drafts-close"
	discardDraftsClose commandID = "workspace.discard-drafts-close"
	cancelDraftClose   commandID = "workspace.cancel-draft-close"
)

func (a *Workspace) draftCloseDisabled(id commandID) string {
	if !a.confirmExit {
		return "No exit confirmation"
	}
	if id == cancelDraftClose {
		return ""
	}
	if a.sessionSave != nil {
		return "Wait for the draft save callback"
	}
	if id == keepDraftsClose && !a.canKeepSessionDrafts() {
		return "Keep supports Project drafts; save merge drafts first"
	}
	return ""
}

func (a *Workspace) runDraftClose(id commandID) {
	switch id {
	case keepDraftsClose:
		a.keepSessionDraftsAndClose()
	case cancelDraftClose:
		if a.sessionSave != nil {
			a.sessionSave.cancel()
		} else {
			a.confirmExit = false
			a.sessionSaveError = ""
		}
	case discardDraftsClose:
		a.exitDiscardDrafts = true
		a.exitApproved = true
		a.confirmExit = false
		if a.CloseWindow != nil {
			a.CloseWindow()
		}
	}
}

func cloneEditorDraft(d *application.EditorDraftSnapshot) *application.EditorDraftSnapshot {
	if d == nil {
		return nil
	}
	copy := *d
	return &copy
}

func (a *Workspace) snapshotEditorDraft(root string, t *tab) *application.EditorDraftSnapshot {
	if a.exitDiscardDrafts || root == "" || t == nil || t.source != "project" || t.code == nil || !t.modified() {
		return nil
	}
	base := t.file.New
	if t.document != nil && t.document.draftBase != nil {
		base = *t.document.draftBase
	}
	return &application.EditorDraftSnapshot{Text: t.code.Buffer.String(), BaseText: base, DiskVersion: t.file.DiskVersion, LineEnding: t.code.Buffer.LineEnding()}
}

func (a *Workspace) restoreEditorDraft(t *tab, saved application.TabSnapshot) {
	if saved.Draft == nil || t.code == nil || t.modified() {
		return
	}
	d := saved.Draft
	changed := t.file.DiskVersion != d.DiskVersion
	base := d.BaseText
	if t.document != nil {
		t.document.draftBase = &base
	}
	t.file.DiskVersion = d.DiskVersion
	t.code.Buffer.Reset(base)
	t.code.Buffer.Set(d.Text)
	t.code.Buffer.SetLineEnding(d.LineEnding)
	t.code.ReadOnly = a.sourceReadOnly(t.source, t.file)
	if changed {
		a.record("Recovered draft · " + t.path + " · disk changed or unavailable; review/reload before saving")
	}
}

// Explicit navigation may cancel the startup reader. Opening that same Project
// identity consumes its saved draft rather than hiding it behind a clean tab.
func (a *Workspace) recoverPendingEditorDraft(t *tab) {
	if a.Model.Active().Repository.Root == "" || t.source != "project" || t.code == nil || t.modified() {
		return
	}
	r := a.repo()
	i := slices.IndexFunc(r.pendingTabs, func(s application.TabSnapshot) bool {
		return s.Code && s.Source == t.source && s.Path == t.path && s.Draft != nil
	})
	if i < 0 {
		return
	}
	saved := r.pendingTabs[i]
	a.restoreEditorDraft(t, saved)
	a.restoreEditorMetadata(t, saved)
	t.pinned = saved.Pinned
	cursor := max(0, min(saved.Cursor, t.code.Buffer.Len()))
	t.code.Select(cursor, cursor)
	t.code.SetScrollY(max(0, saved.ScrollY))
	r.pendingTabs = slices.Delete(r.pendingTabs, i, i+1)
}

// Only contents and their identities matter; cursor movement during IO is safe.
func sessionDraftContents(s application.WorkspaceSnapshot) map[string]application.EditorDraftSnapshot {
	out := make(map[string]application.EditorDraftSnapshot)
	for _, r := range s.Repositories {
		for _, t := range r.Tabs {
			if t.Draft != nil {
				out[r.Root+"\x00"+t.Source+"\x00"+t.Path] = *t.Draft
			}
		}
	}
	return out
}

func (a *Workspace) canKeepSessionDrafts() bool {
	return a.SaveWorkspaceSession != nil && !a.hasUnsavedMergeDrafts() && len(sessionDraftContents(a.SessionSnapshot())) > 0
}

func (a *Workspace) keepSessionDraftsAndClose() {
	if a.sessionSave != nil || !a.canKeepSessionDrafts() {
		return
	}
	snapshot := a.SessionSnapshot()
	if err := application.ValidateSessionDrafts(snapshot); err != nil {
		a.sessionSaveError = err.Error()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	a.sessionSave = request
	a.sessionSaveError = ""
	a.SaveWorkspaceSession(ctx, snapshot, func(err error) {
		if a.sessionSave != request {
			return
		}
		a.sessionSave = nil
		canceled := ctx.Err()
		cancel()
		if err != nil {
			a.sessionSaveError = "Could not keep drafts · " + err.Error()
			return
		}
		if canceled != nil {
			a.sessionSaveError = "Draft save canceled; window kept open"
			return
		}
		if !reflect.DeepEqual(sessionDraftContents(snapshot), sessionDraftContents(a.SessionSnapshot())) || a.hasUnsavedMergeDrafts() {
			a.sessionSaveError = "Edits changed while saving; keep drafts again before closing"
			return
		}
		a.exitApproved = true
		a.confirmExit = false
		if a.CloseWindow != nil {
			a.CloseWindow()
		}
	})
}

func (a *Workspace) exitDraftDialog(c *ui.Context, p tokens) {
	open := true
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(620).MaxHeight(600).Background(p.panel).Border(1, p.theme.Border).Radius(6).Column().Label("Unsaved edits dialog").Children(func() {
			heading(c, p, "Unsaved edits", "Close workspace")
			ui.Text(c, "Keep Project drafts for the next start, or discard unsaved edits. Disk files are saved separately with Save.").FillWidth().Padding(inset)
			if a.hasUnsavedMergeDrafts() {
				ui.Text(c, "Merge drafts must be saved before Project drafts can be kept.").FillWidth().Padding(inset)
			}
			if a.sessionSave != nil {
				ui.Text(c, "Keeping drafts… Cancel requests cancellation; waiting for completion.").FillWidth().Padding(inset)
			}
			if a.sessionSaveError != "" {
				ui.Text(c, a.sessionSaveError).FillWidth().Padding(inset)
			}
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				for _, id := range []commandID{cancelDraftClose, discardDraftsClose, keepDraftsClose} {
					reason := a.disabledFor(id, commandTarget{})
					ui.Button(c, commandLabel(id)).Disabled(reason != "").Tooltip(reason).OnClick(func() { a.run(id, commandTarget{}) })
				}
			})
		})
	})
	if !open {
		a.run(cancelDraftClose, commandTarget{})
	}
}
