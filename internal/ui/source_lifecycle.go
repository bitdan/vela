package ui

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

const (
	saveCloseSource     commandID = "editor.save-close-source"
	discardCloseSource  commandID = "editor.discard-close-source"
	confirmReloadSource commandID = "editor.confirm-reload-source"
)

type sourceFileDecision struct {
	open, saving, reading, cancelClose bool
	kind                               commandID
	root, tabID, path, message         string
	buffer                             *editor.Buffer
	request                            *readRequest
}

func (a *Workspace) canDecideSourceClose(entry *tab) bool {
	return a.Model.Active().Repository.Root != "" && entry != nil && entry.source == "project" && entry.code != nil && !entry.code.ReadOnly
}

func (a *Workspace) stopSourceDecision() {
	if s := a.sourceDecision; s != nil && !s.saving {
		if s.request != nil && a.repositoryFileRead == s.request {
			stopRead(&a.repositoryFileRead)
		}
		stopRead(&s.request)
		a.sourceDecision = nil
	}
}

func (a *Workspace) decisionEntry(s *sourceFileDecision) (*repoView, *tab) {
	for i, session := range a.Model.Sessions {
		if session.Repository.Root == s.root {
			r := &a.repos[i]
			if n := r.tabIndex(s.tabID); n >= 0 {
				entry := &r.tabs[n]
				if entry.code != nil && entry.code.Buffer == s.buffer {
					return r, entry
				}
			}
			break
		}
	}
	return nil, nil
}

func (a *Workspace) requestCloseSource(target commandTarget) {
	if !target.preview {
		if entry := a.sourceSaveEntry(target); a.canDecideSourceClose(entry) && entry.modified() {
			a.sourceDecision = &sourceFileDecision{open: true, kind: closeTab, root: a.Model.Active().Repository.Root, tabID: entry.id, path: entry.path, buffer: entry.code.Buffer}
			return
		}
	}
	a.closeTabs(closeTab, target)
}

func (a *Workspace) closeDecidedSource(s *sourceFileDecision) {
	r, entry := a.decisionEntry(s)
	if entry == nil || entry.modified() {
		return
	}
	if a.Model.Active().Repository.Root == s.root {
		a.cancelSessionNavigation()
	}
	r.closeTabs(closeTab, commandTarget{tabID: s.tabID})
	s.open = false
}

func (a *Workspace) sourceReloadEntry(target commandTarget) *tab {
	if target.tabID != "" {
		entry := a.sourceSaveEntry(target)
		if entry != nil && entry.source == "project" && entry.code != nil {
			return entry
		}
		return nil
	}
	if target.file.Path == "" {
		entry := a.activeDocument()
		if entry != nil && entry.source == "project" && entry.code != nil {
			return entry
		}
		return nil
	}
	for i := range a.repo().tabs {
		entry := &a.repo().tabs[i]
		if entry.path == target.file.Path && entry.source == "project" && entry.code != nil {
			return entry
		}
	}
	return nil
}

func (a *Workspace) sourceReloadDisabled(target commandTarget) string {
	if a.Model.Active().Repository.Root == "" {
		return "Open a local Git repository"
	}
	if a.ReadRepositoryFile == nil {
		return "Source file reader is unavailable"
	}
	if a.sourceReloadEntry(target) == nil {
		return "Open this Project file in the editor before reloading"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || a.sourceDecision != nil {
		return "Wait for the current file task"
	}
	return ""
}

func (a *Workspace) requestSourceReload(target commandTarget) {
	if a.sourceReloadDisabled(target) != "" {
		return
	}
	entry := a.sourceReloadEntry(target)
	s := &sourceFileDecision{open: true, kind: reloadProjectFile, root: a.Model.Active().Repository.Root, tabID: entry.id, path: entry.path, buffer: entry.code.Buffer}
	a.sourceDecision = s
	if !entry.modified() {
		a.reloadDecidedSource(s)
	}
}

func (a *Workspace) sourceDecisionDisabled(id commandID) string {
	s := a.sourceDecision
	if s == nil {
		return "No pending file decision"
	}
	if (s.kind == closeTab && id == confirmReloadSource) || (s.kind == reloadProjectFile && id != confirmReloadSource) {
		return "Choose an action for the current file decision"
	}
	if _, entry := a.decisionEntry(s); entry == nil {
		return "The original editor tab is no longer open"
	}
	if s.saving || s.reading || a.repositoryWrite != "" {
		return "Wait for the current file task"
	}
	if a.Model.Active().Repository.Root != s.root {
		return "Return to the file repository"
	}
	if id == saveCloseSource {
		return a.sourceSaveDisabled(commandTarget{tabID: s.tabID})
	}
	if id == confirmReloadSource && a.ReadRepositoryFile == nil {
		return "Source file reader is unavailable"
	}
	return ""
}

func (a *Workspace) runSourceDecision(id commandID) {
	s := a.sourceDecision
	switch id {
	case discardCloseSource:
		_, entry := a.decisionEntry(s)
		s.buffer.Reset(entry.file.New)
		if entry.document != nil {
			entry.document.draftBase = nil
		}
		a.closeDecidedSource(s)
	case saveCloseSource:
		s.saving, s.cancelClose = true, false
		s.message = ""
		a.saveSource(commandTarget{tabID: s.tabID}, func(err error) {
			s.saving = false
			if a.sourceDecision != s {
				return
			}
			if s.cancelClose {
				s.open = false
				return
			}
			if err != nil {
				s.message = "Could not save; edits are kept · " + err.Error()
				return
			}
			_, entry := a.decisionEntry(s)
			if entry == nil {
				s.open = false
				return
			}
			if entry.modified() {
				s.message = "Saved previous text; newer edits remain open"
				return
			}
			a.closeDecidedSource(s)
		})
	case confirmReloadSource:
		a.reloadDecidedSource(s)
	}
}

func (a *Workspace) reloadDecidedSource(s *sourceFileDecision) {
	if _, entry := a.decisionEntry(s); entry == nil {
		return
	}
	a.cancelSessionNavigation()
	stopRead(&a.repositoryFileRead)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	s.reading = true
	s.message = ""
	a.repositoryFileRead = request
	before := bufferDiskText(s.buffer)
	a.ReadRepositoryFile(ctx, s.root, "project", domain.File{Path: s.path}, func(file domain.File, err error) {
		if a.sourceDecision != s || s.request != request || a.repositoryFileRead != request || a.Model.Active().Repository.Root != s.root || ctx.Err() == context.Canceled {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&a.repositoryFileRead)
		s.request = nil
		s.reading = false
		if err != nil {
			s.message = "Could not reload; edits are kept · " + err.Error()
			return
		}
		r, entry := a.decisionEntry(s)
		if entry == nil {
			s.open = false
			return
		}
		if bufferDiskText(s.buffer) != before {
			s.message = "Newer edits are kept. Retry reload or cancel."
			return
		}
		if file.Path != s.path {
			s.message = "Reload returned a different file; edits are kept"
			return
		}
		s.buffer.Reset(file.New)
		s.buffer.SetLanguage(editor.LanguageFor(file.Path))
		update := func(entry *tab) {
			if entry == nil || entry.code == nil || entry.code.Buffer != s.buffer {
				return
			}
			cursor := min(entry.code.Cursor(), s.buffer.Len())
			entry.code.Select(cursor, cursor)
			entry.code.ReadOnly = a.sourceReadOnly("project", file)
			entry.file = file
			if entry.document != nil {
				entry.document.draftBase = nil
			}
			applyEditorFileType(entry)
		}
		for i := range r.tabs {
			update(&r.tabs[i])
		}
		update(r.split)
		s.open = false
		a.status = "Reloaded " + s.path
		if a.ReadRepository != nil {
			a.loadRepository(s.root, true)
		}
	})
}

func (a *Workspace) sourceDecisionView(c *ui.Context, p tokens) {
	s := a.sourceDecision
	w, _ := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 640)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Source file decision").Column().Children(func() {
			title := "Save changes before closing?"
			detail := "Save and close, discard only the unsaved editor changes, or cancel. Discard leaves files on disk unchanged."
			if s.kind == reloadProjectFile {
				title = "Reload file from disk?"
				detail = "Reload replaces unsaved editor content with the current disk version. A read failure keeps your edits."
			}
			heading(c, p, title, "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.path).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, s.root).FillWidth().SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted)
				ui.Text(c, detail).FillWidth()
				if s.saving {
					ui.Text(c, "Saving and refreshing…")
				}
				if s.reading {
					ui.Text(c, "Reading disk version…")
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Cancel").OnClick(func() {
					if s.saving {
						s.cancelClose = true
						a.run(cancelGitTask, commandTarget{})
					} else {
						s.open = false
					}
				})
				if s.kind == closeTab {
					ui.Button(c, "Discard and Close").Label("Discard and Close Source").Disabled(a.sourceDecisionDisabled(discardCloseSource) != "").OnClick(func() { a.run(discardCloseSource, commandTarget{}) })
					ui.PrimaryButton(c, "Save and Close").Label("Save and Close Source").Disabled(a.sourceDecisionDisabled(saveCloseSource) != "").OnClick(func() { a.run(saveCloseSource, commandTarget{}) })
				} else {
					ui.PrimaryButton(c, "Reload").Label("Reload Source File").Disabled(a.sourceDecisionDisabled(confirmReloadSource) != "").OnClick(func() { a.run(confirmReloadSource, commandTarget{}) })
				}
			})
		})
	})
	if a.sourceDecision != s {
		return
	}
	if !s.open {
		if s.saving {
			s.cancelClose = true
			a.run(cancelGitTask, commandTarget{})
			s.open = true
		} else {
			a.stopSourceDecision()
		}
	}
}
