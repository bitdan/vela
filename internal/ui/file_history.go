package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	loadMoreFileHistory     commandID = "history.load-more-file"
	closeFileHistory        commandID = "history.close-file"
	openFileHistoryRevision commandID = "history.open-file-revision"
	openFileHistoryDiff     commandID = "history.open-file-diff"
)

type fileHistoryDialog struct {
	open, loading, hasMore        bool
	root, revision, path, message string
	commits                       []domain.Commit
	selected                      int
	list                          ui.ListState
	request                       *readRequest
}

func (a *Workspace) closeFileHistoryDialog() {
	if s := a.fileHistory; s != nil {
		stopRead(&s.request)
		a.fileHistory = nil
	}
}

func (a *Workspace) openFileHistory(target commandTarget) {
	a.closeFileHistoryDialog()
	s := &fileHistoryDialog{open: true, hasMore: true, selected: -1, root: a.Model.Active().Repository.Root, revision: a.blameRevision(target), path: target.file.Path}
	a.fileHistory = s
	a.loadFileHistoryPage(s)
}

func (a *Workspace) loadFileHistoryPage(s *fileHistoryDialog) {
	if s.loading || !s.hasMore || a.ReadRepositoryFileHistory == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	s.loading = true
	s.message = ""
	a.ReadRepositoryFileHistory(ctx, s.root, s.revision, s.path, len(s.commits), 100, func(page domain.FileHistoryPage, err error) {
		if a.fileHistory != s || s.request != request || ctx.Err() == context.Canceled {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		s.loading = false
		if err != nil {
			s.message = "Could not load file history · " + err.Error()
			return
		}
		if len(page.Commits) > 100 || len(page.Commits) == 0 && page.HasMore {
			s.message = "Git returned an invalid history page"
			return
		}
		seen := make(map[string]bool, len(s.commits))
		for _, commit := range s.commits {
			seen[commit.Hash] = true
		}
		for _, commit := range page.Commits {
			if seen[commit.Hash] {
				s.message = "History page repeated a commit; reopen the history query"
				return
			}
			seen[commit.Hash] = true
		}
		s.commits = append(s.commits, page.Commits...)
		s.hasMore = page.HasMore
		if s.selected < 0 && len(s.commits) > 0 {
			s.selected = 0
		}
	})
}

func (a *Workspace) fileHistoryDisabled(id commandID) string {
	s := a.fileHistory
	if s == nil {
		return "Open file history"
	}
	if id == closeFileHistory {
		return ""
	}
	if s.root != a.Model.Active().Repository.Root {
		return "Return to the history repository"
	}
	if id == loadMoreFileHistory {
		if s.loading {
			return "History is loading"
		}
		if !s.hasMore {
			return "All matching history is loaded"
		}
		if len(s.commits) >= 100000 {
			return "History display limit reached; use Git for older records"
		}
		return ""
	}
	if a.ReadRepositoryFile == nil || s.selected < 0 || s.selected >= len(s.commits) || len(s.commits[s.selected].Files) != 1 {
		return "Select a historical file"
	}
	if id == openFileHistoryRevision && s.commits[s.selected].Files[0].Status == "D" {
		return "File was deleted in this commit; show its diff instead"
	}
	return ""
}

func (a *Workspace) runFileHistory(id commandID) {
	s := a.fileHistory
	switch id {
	case closeFileHistory:
		a.closeFileHistoryDialog()
	case loadMoreFileHistory:
		a.loadFileHistoryPage(s)
	case openFileHistoryRevision, openFileHistoryDiff:
		commit := s.commits[s.selected]
		target := commandTarget{source: commit.Hash, file: commit.Files[0]}
		a.closeFileHistoryDialog()
		next := openRevision
		if id == openFileHistoryDiff {
			next = previewDiff
		}
		a.run(next, target)
	}
}

func (a *Workspace) fileHistoryView(c *ui.Context, p tokens) {
	s := a.fileHistory
	w, h := c.Size()
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 900)).Height(min(h-32, 660)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("File History dialog").Column().Children(func() {
			heading(c, p, "File History", "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.path).FillWidth().SingleLine().Ellipsis("…")
				ui.Text(c, s.root+" · "+s.revision).FillWidth().SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted)
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
				if s.loading {
					ui.Text(c, "Loading file history…")
				} else if len(s.commits) == 0 && s.message == "" {
					ui.Text(c, "No matching committed file history")
				}
			})
			s.list.Selected = &s.selected
			s.list.Key = func(i int) any { return s.commits[i].Hash }
			s.list.Label = func(i int) string { return s.commits[i].Subject }
			list := ui.List(c, &s.list, len(s.commits), func(i int) {
				commit := s.commits[i]
				row := ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Label(commit.Subject)
				row.ContextMenu(func(m *ui.Menu) { s.selected = i; a.fileHistoryMenu(m) })
				if i%2 == 1 && i != s.selected {
					row.Background(p.stripe)
				}
				row.Children(func() {
					ui.Text(c, commit.Hash[:min(10, len(commit.Hash))]).Width(82).TextColor(p.theme.TextMuted)
					ui.Text(c, commit.Subject).Grow(1).SingleLine().Ellipsis("…")
					ui.Text(c, commit.Author).Width(115).SingleLine().Ellipsis("…")
					ui.Text(c, commit.Timestamp.Format("2006-01-02")).Width(90).TextColor(p.theme.TextMuted)
				})
			}).Grow(1).FillWidth().Label("File History commits").ContextMenu(func(m *ui.Menu) {
				a.fileHistoryMenu(m)
			})
			if list.Submitted() {
				id := openFileHistoryRevision
				if a.disabledFor(id, commandTarget{}) != "" {
					id = openFileHistoryDiff
				}
				a.pending = func() { a.run(id, commandTarget{}) }
			}
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				if s.selected >= 0 && s.selected < len(s.commits) {
					commit := s.commits[s.selected]
					if len(commit.Files) == 1 {
						ui.Text(c, commit.Files[0].Status+" · "+commit.Files[0].Path).FillWidth().SingleLine().Ellipsis("…")
					}
				}
				ui.Row(c).FillWidth().Gap(gap).AlignItems(ui.Center).Children(func() {
					ui.Text(c, fmt.Sprintf("%d commits loaded", len(s.commits))).Grow(1)
					if len(s.commits) >= 100000 && s.hasMore {
						ui.Text(c, "History display limit reached")
					}
					ui.Button(c, "Load More").Label("Load More File History").Disabled(a.fileHistoryDisabled(loadMoreFileHistory) != "").OnClick(func() { a.run(loadMoreFileHistory, commandTarget{}) })
				})
			})
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Button(c, "Close").Label("Close File History").OnClick(func() { a.run(closeFileHistory, commandTarget{}) })
				ui.Button(c, "Show Diff").Label("Show Historical Diff").Disabled(a.fileHistoryDisabled(openFileHistoryDiff) != "").OnClick(func() { a.run(openFileHistoryDiff, commandTarget{}) })
				ui.PrimaryButton(c, "Open Version").Label("Open Historical Version").Disabled(a.fileHistoryDisabled(openFileHistoryRevision) != "").OnClick(func() { a.run(openFileHistoryRevision, commandTarget{}) })
			})
		})
	})
	if !s.open {
		a.closeFileHistoryDialog()
	}
}

func (a *Workspace) fileHistoryMenu(m *ui.Menu) {
	a.menuGroup(m, commandTarget{}, openFileHistoryDiff, openFileHistoryRevision)
	if s := a.fileHistory; s != nil && s.selected >= 0 && s.selected < len(s.commits) {
		m.Separator()
		a.menu(m, copyHash, commandTarget{source: s.commits[s.selected].Hash})
	}
}
