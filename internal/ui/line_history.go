package ui

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"slices"
	"time"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	closeLineHistory        commandID = "history.close-line-range"
	moreLineHistory         commandID = "history.more-line-range"
	selectLineHistory       commandID = "history.select-line-range"
	nextLineHistoryHunk     commandID = "history.next-line-hunk"
	previousLineHistoryHunk commandID = "history.previous-line-hunk"
)

type lineHistoryDialog struct {
	root             string
	query            domain.LineHistoryQuery
	repository       domain.Repository
	identity         inlineIdentity
	entries          []domain.LineHistoryEntry
	selected, hunk   int
	hasMore, loading bool
	problem          string
	list             ui.ListState
	request          *readRequest
	diff             *editor.DiffState
}

func isLineHistoryCommand(id commandID) bool {
	return id == historySelection || id == closeLineHistory || id == moreLineHistory || id == selectLineHistory || id == nextLineHistoryHunk || id == previousLineHistoryHunk
}
func (a *Workspace) lineHistoryTarget(t commandTarget) *tab {
	if t.editorState != nil {
		if s := a.repo().split; s != nil && s.code == t.editorState {
			return s
		}
		for i := range a.repo().tabs {
			if a.repo().tabs[i].code == t.editorState {
				return &a.repo().tabs[i]
			}
		}
		return nil
	}
	return a.sourceSaveEntry(t)
}
func (a *Workspace) lineHistoryQuery(t commandTarget) (domain.LineHistoryQuery, inlineIdentity) {
	entry := a.lineHistoryTarget(t)
	r := a.Model.Active().Repository
	if entry == nil || entry.code == nil || entry.source != "project" || entry.code.Buffer.Modified() || (t.file.Path != "" && t.file.Path != entry.path) {
		return domain.LineHistoryQuery{}, inlineIdentity{}
	}
	b := entry.code.Buffer
	snapshot := b.Snapshot()
	sel := entry.code.Selection()
	start, end := min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)
	first, last := snapshot.LineOf(start), snapshot.LineOf(end)
	if end > start && end == snapshot.LineStart(last) {
		last--
	}
	q := domain.LineHistoryQuery{Revision: r.Head, Path: entry.path, Text: bufferDiskText(b), DiskVersion: entry.file.DiskVersion, Start: first + 1, End: last + 1, Limit: 100}
	return q, inlineIdentity{root: r.Root, head: r.Head, path: entry.path, tabID: entry.id, disk: entry.file.DiskVersion, work: r.WorktreeVersion, navigation: a.repositoryNavigation, version: b.Version(), code: entry.code}
}
func (a *Workspace) lineHistoryDisabled(id commandID, t commandTarget) string {
	if id == closeLineHistory {
		return ""
	}
	if id == historySelection {
		if a.ReadLineHistory == nil || a.Model.Active().Repository.Root == "" {
			return "Line history requires a local Project file"
		}
		if a.repositoryWrite != "" || a.repositoryRead != nil || a.squash != nil || a.lineHistory != nil {
			return "Finish or close the current task"
		}
		q, identity := a.lineHistoryQuery(t)
		if identity.code == nil {
			return "Select an unchanged Project editor"
		}
		if err := application.ValidateLineHistoryQuery(q); err != nil {
			return err.Error()
		}
		return ""
	}
	s := a.lineHistory
	if s == nil || s.loading {
		return "Wait for line history"
	}
	if id == moreLineHistory && (!s.hasMore || len(s.entries) >= 1000) {
		return "No more range history"
	}
	if id == selectLineHistory && (t.index < 0 || t.index >= len(s.entries)) {
		return "Select a range commit"
	}
	if (id == nextLineHistoryHunk || id == previousLineHistoryHunk) && (s.selected < 0 || s.selected >= len(s.entries) || len(s.entries[s.selected].Hunks) < 2) {
		return "No other range hunk"
	}
	return ""
}
func (a *Workspace) runLineHistory(id commandID, t commandTarget) {
	switch id {
	case historySelection:
		q, identity := a.lineHistoryQuery(t)
		s := &lineHistoryDialog{root: identity.root, query: q, identity: identity, repository: a.Model.Active().Repository, selected: -1, hasMore: true}
		a.lineHistory = s
		a.loadLineHistory(s)
	case closeLineHistory:
		a.closeLineHistory()
	case moreLineHistory:
		a.loadLineHistory(a.lineHistory)
	case selectLineHistory:
		a.lineHistory.selected = t.index
		a.lineHistory.hunk = 0
		a.prepareLineHistoryDiff()
	case nextLineHistoryHunk, previousLineHistoryHunk:
		s := a.lineHistory
		step := 1
		if id == previousLineHistoryHunk {
			step = -1
		}
		s.hunk = (s.hunk + step + len(s.entries[s.selected].Hunks)) % len(s.entries[s.selected].Hunks)
		a.prepareLineHistoryDiff()
	}
}
func (a *Workspace) closeLineHistory() {
	if s := a.lineHistory; s != nil {
		stopRead(&s.request)
		a.lineHistory = nil
	}
}
func (a *Workspace) validLineHistoryOrigin(s *lineHistoryDialog) bool {
	q, id := a.lineHistoryQuery(commandTarget{editorState: s.identity.code})
	return id == s.identity && q.Text == s.query.Text && a.repositoryWrite == "" && a.repositoryRead == nil
}
func (a *Workspace) loadLineHistory(s *lineHistoryDialog) {
	if !a.validLineHistoryOrigin(s) {
		s.problem = "Editor or repository changed; reopen line history"
		return
	}
	q := s.query
	q.Offset = len(s.entries)
	q.Limit = min(100, 1000-q.Offset)
	if q.Limit < 1 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	req := &readRequest{cancel: cancel}
	s.request = req
	s.loading = true
	s.problem = ""
	a.ReadLineHistory(ctx, s.repository, q, func(page domain.LineHistoryPage, err error) {
		if a.lineHistory != s || s.request != req {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		s.loading = false
		if err != nil {
			s.problem = "Could not read line history · " + err.Error()
			return
		}
		if !a.validLineHistoryOrigin(s) {
			s.problem = "Editor or repository changed; reopen line history"
			return
		}
		if len(page.Entries) > q.Limit || len(page.Entries) == 0 && page.HasMore {
			s.problem = "Invalid range history page"
			return
		}
		for _, entry := range page.Entries {
			if !fixedObjectID(entry.Commit.Hash) || len(entry.Hunks) == 0 || slices.ContainsFunc(s.entries, func(e domain.LineHistoryEntry) bool { return e.Commit.Hash == entry.Commit.Hash }) {
				s.problem = "Invalid or repeated range commit"
				return
			}
		}
		s.entries = append(s.entries, page.Entries...)
		s.hasMore = page.HasMore
		if s.selected < 0 && len(s.entries) > 0 {
			s.selected = 0
			s.hunk = 0
			a.prepareLineHistoryDiff()
		}
	})
}
func (a *Workspace) prepareLineHistoryDiff() {
	s := a.lineHistory
	if s == nil || s.selected < 0 || s.selected >= len(s.entries) {
		return
	}
	h := s.entries[s.selected].Hunks[s.hunk]
	s.diff = &editor.DiffState{Old: editor.NewBuffer(h.Before), New: editor.NewBuffer(h.After), Layout: editor.DiffSplit}
}
func (a *Workspace) lineHistoryView(c *ui.Context, p tokens) {
	s := a.lineHistory
	open := true
	w, h := c.Size()
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 1160)).Height(min(h-32, 790)).Background(p.panel).Border(1, p.theme.Border).Column().Label("Line History dialog").Children(func() {
			heading(c, p, "History for Selection", fmt.Sprintf("%s · lines %d–%d", s.query.Path, s.query.Start, s.query.End))
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.root+" · "+s.query.Revision).SingleLine().Ellipsis("…").FillWidth().TextColor(p.theme.TextMuted)
				if s.loading {
					ui.Text(c, "Loading line history…")
				} else if len(s.entries) == 0 && s.problem == "" {
					ui.Text(c, "No matching line history")
				}
				if s.problem != "" {
					ui.Text(c, s.problem).FillWidth().TextColor(p.theme.Danger)
				}
			})
			s.list.Selected = &s.selected
			s.list.Key = func(i int) any { return s.entries[i].Commit.Hash }
			list := ui.List(c, &s.list, len(s.entries), func(i int) {
				commit := s.entries[i].Commit
				ui.Row(c).FillWidth().Height(rowHeight).Padding(0, inset).Gap(gap).AlignItems(ui.Center).Label("Line history commit " + commit.Hash).ContextMenu(func(m *ui.Menu) { a.menu(m, selectLineHistory, commandTarget{index: i}) }).Children(func() {
					ui.Text(c, commit.Hash[:10]).Width(88)
					ui.Text(c, commit.Subject).Grow(1).SingleLine().Ellipsis("…")
					ui.Text(c, commit.Author).Width(130).SingleLine().Ellipsis("…")
				})
			}).FillWidth().Height(170).Label("Line History commits")
			if list.Changed() {
				s.hunk = 0
				a.prepareLineHistoryDiff()
			}
			if s.diff != nil && s.selected >= 0 && s.selected < len(s.entries) {
				rangeHunk := s.entries[s.selected].Hunks[s.hunk]
				ui.Row(c).FillWidth().Padding(0, inset).Gap(gap).Children(func() {
					ui.Text(c, fmt.Sprintf("Range fragment %d/%d · old line %d → new line %d", s.hunk+1, len(s.entries[s.selected].Hunks), rangeHunk.OldStart, rangeHunk.NewStart)).Grow(1)
					a.panelAction(c, previousLineHistoryHunk, "‹")
					a.panelAction(c, nextLineHistoryHunk, "›")
				})
				s.diff.Font, s.diff.TabSize = a.editorPreferences()
				editor.DiffView(c.Key("line-history-diff"), s.diff).Grow(1).FillWidth().Label("Line History Diff")
			} else {
				ui.Box(c).Grow(1)
			}
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Text(c, fmt.Sprintf("%d commits · fixed HEAD · read-only range fragments", len(s.entries))).Grow(1)
				ui.Button(c, "Load More").Label("Load More Line History").Disabled(a.disabledFor(moreLineHistory, commandTarget{}) != "").OnClick(func() { a.run(moreLineHistory, commandTarget{}) })
				ui.Button(c, "Close").Label("Close Line History").OnClick(func() { a.run(closeLineHistory, commandTarget{}) })
			})
		})
	})
	if !open {
		a.run(closeLineHistory, commandTarget{})
	}
}
