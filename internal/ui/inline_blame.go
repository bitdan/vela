package ui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"strings"
	"time"
	"vela/internal/domain"
)

const (
	toggleInlineBlame   commandID = "editor.toggle-inline-blame"
	inlineCommitDetails commandID = "editor.inline-blame-details"
	closeInlineDetails  commandID = "editor.close-inline-blame-details"
)

type inlineIdentity struct {
	root, head, path, tabID, disk, work string
	navigation, version                 uint64
	code                                *editor.State
}
type inlineBlameState struct {
	enabled             bool
	watchCancel         context.CancelFunc
	identity            inlineIdentity
	request, detailRead *readRequest
	doc                 *domain.BlameDocument
	line                domain.BlameLine
	status              string
	details             *domain.Commit
	detailsOpen         bool
}

func isInlineBlameCommand(id commandID) bool {
	return id == toggleInlineBlame || id == inlineCommitDetails || id == closeInlineDetails
}
func (a *Workspace) inlineDisabled(id commandID) string {
	if id == closeInlineDetails {
		return ""
	}
	if id == toggleInlineBlame && a.inlineBlame.enabled {
		return ""
	}
	if a.ReadRepositoryBlame == nil || a.ReadRepositoryFile == nil || a.WatchEditorActivity == nil || a.Model.Active().Repository.Root == "" {
		return "Inline Blame requires a local repository and editor activity reader"
	}
	if id == inlineCommitDetails && (a.inlineBlame.line.Commit == "" || a.ReadRepositoryLog == nil) {
		return "Select an accurately attributed committed line"
	}
	return ""
}
func (a *Workspace) runInlineBlame(id commandID) {
	s := &a.inlineBlame
	switch id {
	case toggleInlineBlame:
		if s.enabled {
			a.StopEditorActivity()
			return
		}
		s.enabled = true
		ctx, cancel := context.WithCancel(context.Background())
		s.watchCancel = cancel
		a.WatchEditorActivity(ctx, func() {
			if ctx.Err() == nil {
				a.tickInlineBlame()
			}
		})
		a.tickInlineBlame()
	case closeInlineDetails:
		stopRead(&s.detailRead)
		s.detailsOpen = false
		s.details = nil
	case inlineCommitDetails:
		a.tickInlineBlame()
		if s.line.Commit == "" {
			return
		}
		stopRead(&s.detailRead)
		s.details = nil
		s.detailsOpen = true
		root, hash := s.identity.root, s.line.Commit
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		req := &readRequest{cancel: cancel}
		s.detailRead = req
		a.ReadRepositoryLog(ctx, root, domain.CommitLogSnapshot{Tips: []string{hash}}, 0, 1, func(page domain.CommitLogPage, err error) {
			if s.detailRead != req {
				return
			}
			stopRead(&s.detailRead)
			if err != nil {
				s.status = "Commit details: " + err.Error()
				return
			}
			if len(page.Commits) != 1 || page.Commits[0].Hash != hash {
				s.status = "Commit details unavailable"
				return
			}
			copy := page.Commits[0]
			s.details = &copy
		})
	}
}

// StopEditorActivity is called only when the window actually closes or the user disables the hint.
func (a *Workspace) StopEditorActivity() {
	s := &a.inlineBlame
	if s.watchCancel != nil {
		s.watchCancel()
		s.watchCancel = nil
	}
	s.enabled = false
	a.clearInlineBlame()
}
func (a *Workspace) clearInlineBlame() {
	s := &a.inlineBlame
	if s.identity.code != nil {
		s.identity.code.SetInlays(nil)
	}
	stopRead(&s.request)
	stopRead(&s.detailRead)
	s.identity = inlineIdentity{}
	s.doc = nil
	s.line = domain.BlameLine{}
	s.details = nil
	s.detailsOpen = false
	s.status = ""
}
func (a *Workspace) inlineTarget() inlineIdentity {
	entry := a.sourceSaveEntry(commandTarget{})
	r := a.Model.Active().Repository
	if entry == nil || entry.code == nil || entry.source != "project" || r.Root == "" || !fixedObjectID(r.Head) {
		return inlineIdentity{}
	}
	return inlineIdentity{root: r.Root, head: r.Head, path: entry.path, tabID: entry.id, disk: entry.file.DiskVersion, work: r.WorktreeVersion, navigation: a.repositoryNavigation, version: entry.code.Buffer.Version(), code: entry.code}
}
func (a *Workspace) inlineCurrent(id inlineIdentity) bool {
	return a.inlineBlame.enabled && a.inlineBlame.identity == id && a.inlineTarget() == id && !id.code.Buffer.Modified() && a.repositoryRead == nil && a.repositoryWrite == "" && !a.confirmExit && a.sessionSave == nil
}
func (a *Workspace) tickInlineBlame() {
	s := &a.inlineBlame
	if !s.enabled {
		return
	}
	id := a.inlineTarget()
	if id != s.identity {
		a.clearInlineBlame()
		s.identity = id
	}
	if id.code == nil {
		s.status = "Select a Project source file"
		return
	}
	if id.code.Buffer.Modified() || a.repositoryRead != nil || a.repositoryWrite != "" || a.confirmExit || a.sessionSave != nil {
		id.code.SetInlays(nil)
		s.line = domain.BlameLine{}
		stopRead(&s.request)
		s.doc = nil
		s.status = "Blame paused for unsaved changes or active tasks"
		return
	}
	if s.doc != nil {
		a.showInlineLine()
		return
	}
	if s.request != nil || s.status != "" {
		return
	} // A failed/canceled identity is not retried by the timer.
	s.status = "Loading line attribution…"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	req := &readRequest{cancel: cancel}
	s.request = req
	a.ReadRepositoryBlame(ctx, id.root, id.head, id.path, func(doc domain.BlameDocument, err error) {
		if s.request != req || !a.inlineCurrent(id) {
			return
		}
		if err != nil {
			stopRead(&s.request)
			s.status = "Blame: " + err.Error()
			return
		}
		if doc.Revision != id.head || doc.Path != id.path || doc.Text != bufferDiskText(id.code.Buffer) {
			stopRead(&s.request)
			s.status = "Buffer differs from committed HEAD"
			return
		}
		entry := a.sourceSaveEntry(commandTarget{})
		file := entry.file
		a.ReadRepositoryFile(ctx, id.root, "project", file, func(disk domain.File, err error) {
			if s.request != req || !a.inlineCurrent(id) {
				return
			}
			stopRead(&s.request)
			if err != nil {
				s.status = "Blame disk check: " + err.Error()
				return
			}
			if disk.Path != id.path || disk.New != doc.Text || disk.DiskVersion != id.disk || id.disk == "" {
				s.status = "Disk differs from this committed editor version"
				return
			}
			s.doc = &doc
			s.status = ""
			a.showInlineLine()
		})
	})
}
func (a *Workspace) showInlineLine() {
	s := &a.inlineBlame
	code := s.identity.code
	code.SetInlays(nil)
	s.line = domain.BlameLine{}
	index := code.CursorPos().Line
	if index < 0 || index >= len(s.doc.Lines) {
		s.status = "No committed line at cursor"
		return
	}
	line := s.doc.Lines[index]
	if line.Line != index+1 || !fixedObjectID(line.Commit) {
		s.status = "Attribution mapping unavailable"
		return
	}
	s.line = line
	s.status = ""
	text := fmt.Sprintf("  %s · %s · %s", line.Author, line.AuthoredAt.Format("2006-01-02"), line.Summary)
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > 200 {
		text = string(runes[:200]) + "…"
	}
	text = "  " + text
	code.SetInlays([]editor.Inlay{{Offset: code.Buffer.Snapshot().LineEnd(index), Text: text}})
}
func (a *Workspace) inlineBlameControls(c *ui.Context, p tokens) {
	s := &a.inlineBlame
	reasonToggle := a.disabledFor(toggleInlineBlame, commandTarget{})
	ui.Button(c, "Inline Blame").Label("Inline Blame").Width(104).Shrink(0).Height(controlHeight).Disabled(reasonToggle != "").Tooltip(reasonToggle).OnClick(func() { a.run(toggleInlineBlame, commandTarget{}) })
	if s.enabled {
		reason := a.disabledFor(inlineCommitDetails, commandTarget{})
		ui.Button(c, "Commit Details").Label("Inline Blame Commit Details").Shrink(0).Height(controlHeight).Disabled(reason != "").Tooltip(reason).OnClick(func() { a.run(inlineCommitDetails, commandTarget{}) })
		ui.Text(c, s.status).SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted).FontSize(p.textSize(smallText))
	}
}
func (a *Workspace) inlineDetailsView(c *ui.Context, p tokens) {
	s := &a.inlineBlame
	open := true
	w, h := c.Size()
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 760)).MaxHeight(h-48).Column().Background(p.panel).Border(1, p.theme.Border).Children(func() {
			heading(c, p, "Line Commit Details", s.identity.path)
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				if s.detailRead != nil {
					ui.Text(c, "Loading commit details…")
				} else if d := s.details; d != nil {
					ui.Text(c, d.Hash).FillWidth().SingleLine().Ellipsis("…")
					ui.Text(c, d.Author+" <"+d.AuthorEmail+"> · "+d.AuthoredAt.Format(time.RFC3339))
					ui.Text(c, d.Subject).FillWidth()
					ui.Scroll(c).MaxHeight(300).FillWidth().Children(func() { ui.Text(c, d.Body).FillWidth() })
				} else {
					ui.Text(c, s.status).FillWidth()
				}
				a.panelAction(c, closeInlineDetails, "Close")
			})
		})
	})
	if !open {
		a.run(closeInlineDetails, commandTarget{})
	}
}
