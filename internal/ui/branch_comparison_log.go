package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

// Each direction owns its paging, selection, graph and detail request. Neither
// comparison log shares selection or pagination with the main repository log.
type comparisonSide struct {
	comparison                       domain.RevisionComparison
	commits                          []domain.Commit
	graph                            []graphRow
	files                            []domain.File
	list, fileList                   ui.ListState
	selected, file, parent           int
	revision, message, detailMessage string
	more, filesReady                 bool
	request, details                 *readRequest
	split, detailSplit               float32
	graphWidth                       float32
}

func (a *Workspace) startTwoWayComparison(s *branchComparisonDialog) {
	for i := range s.sides {
		comparison := s.comparison
		if i == 1 {
			comparison.Left, comparison.Right = comparison.Right, comparison.Left
		}
		s.sides[i] = &comparisonSide{comparison: comparison, selected: -1, file: -1, more: true, split: 64, detailSplit: 55}
		a.loadComparisonSide(s, s.sides[i])
	}
	if s.split == 0 {
		s.split = 50
	}
}

func (a *Workspace) comparisonSideAlive(s *branchComparisonDialog, side *comparisonSide) bool {
	return a.comparisonAlive(s) && (s.sides[0] == side || s.sides[1] == side)
}

func (a *Workspace) loadComparisonSide(s *branchComparisonDialog, side *comparisonSide) {
	if side.request != nil || !side.more {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	side.request, side.message = request, ""
	offset := len(side.commits)
	a.ReadRepositoryComparisonCommits(ctx, s.root, side.comparison, offset, 100, func(page domain.CommitLogPage, err error) {
		if !a.comparisonSideAlive(s, side) || side.request != request {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&side.request)
		if err != nil {
			side.message = "Could not load commits · " + err.Error()
			return
		}
		if len(page.Commits) > 100 || len(page.Commits) == 0 && page.HasMore || len(side.commits) != offset {
			side.message = "Invalid comparison page"
			return
		}
		seen := make(map[string]bool, offset+len(page.Commits))
		for _, entry := range side.commits {
			seen[entry.Hash] = true
		}
		for _, entry := range page.Commits {
			if !fixedObjectID(entry.Hash) || seen[entry.Hash] {
				side.message = "Repeated or invalid comparison commit"
				return
			}
			seen[entry.Hash] = true
		}
		for i := range page.Commits {
			if page.Commits[i].Hash == side.comparison.Left {
				label := s.label
				if side == s.sides[1] {
					label = s.rightLabel
				}
				page.Commits[i].Refs = []string{label}
			}
		}
		side.commits = append(slices.Clone(side.commits), page.Commits...)
		domain.ReconnectCommitLog(side.commits)
		side.graph = buildGraph(side.commits)
		side.graphWidth = 36
		for _, entry := range side.commits {
			side.graphWidth = max(side.graphWidth, 36+float32(min(entry.Lane, lastGraphLane))*graphStep)
		}
		side.more = page.HasMore
		if side.selected == -1 && len(side.commits) > 0 {
			side.selected = 0
			a.loadComparisonDetails(s, side, 0)
		}
	})
}

func (a *Workspace) loadComparisonDetails(s *branchComparisonDialog, side *comparisonSide, parent int) {
	stopRead(&side.details)
	side.files, side.filesReady, side.file, side.detailMessage = nil, false, -1, ""
	if side.selected < 0 || side.selected >= len(side.commits) {
		side.revision = ""
		return
	}
	entry := side.commits[side.selected]
	side.revision, side.parent = entry.Hash, parent
	if len(entry.ParentHashes) > 1 && parent == 0 {
		side.parent = 1
	}
	if a.ReadRepositoryRevisionChanges == nil {
		side.detailMessage = "Changed-file reader unavailable"
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	side.details = request
	a.ReadRepositoryRevisionChanges(ctx, s.root, entry.Hash, side.parent, func(files []domain.File, err error) {
		if !a.comparisonSideAlive(s, side) || side.details != request {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&side.details)
		if err != nil {
			side.detailMessage = "Could not read changed files · " + err.Error()
			return
		}
		if len(files) > 10000 {
			side.detailMessage = "Too many changed files"
			return
		}
		for _, file := range files {
			if !application.ValidProjectSearchPath(file.Path) || file.OldPath != "" && !application.ValidProjectSearchPath(file.OldPath) {
				side.detailMessage = "Invalid historical path"
				return
			}
		}
		side.files, side.filesReady = files, true
	})
}

func comparisonSideIndex(t commandTarget) int {
	if t.source == "1" {
		return 1
	}
	return 0
}

func (a *Workspace) twoWayComparisonDisabled(id commandID, t commandTarget) string {
	s := a.comparisonTarget(t)
	if id == closeBranchComparison || id == reloadBranchComparison {
		return ""
	}
	if id == cancelBranchComparison {
		if s.request != nil {
			return ""
		}
		for _, side := range s.sides {
			if side != nil && (side.request != nil || side.details != nil) {
				return ""
			}
		}
		return "No query is running"
	}
	side := s.sides[comparisonSideIndex(t)]
	if !s.ready || side == nil {
		return "Wait for branch identities"
	}
	if id == loadMoreComparisonCommits {
		if side.request != nil || !side.more {
			return "No more commits available"
		}
		return ""
	}
	if side.selected < 0 || side.selected >= len(side.commits) {
		return "Select a commit"
	}
	if id == selectComparisonCommit {
		return ""
	}
	if side.revision != side.commits[side.selected].Hash {
		return "Wait for commit selection"
	}
	if id == browseComparisonCommit {
		if a.ReadRepositoryRevisionTree == nil {
			return "Historical repository browser unavailable"
		}
		return ""
	}
	if id == comparisonParent {
		if len(side.commits[side.selected].ParentHashes) < 2 {
			return "Select a merge commit"
		}
		return ""
	}
	if side.details != nil || !side.filesReady || side.file < 0 || side.file >= len(side.files) {
		return "Select a loaded changed file"
	}
	if a.ReadRepositoryFile == nil {
		return "File reader unavailable"
	}
	return ""
}

func (a *Workspace) runTwoWayComparison(id commandID, t commandTarget) {
	s := a.comparisonTarget(t)
	switch id {
	case closeBranchComparison:
		a.closeTabs(closeTab, commandTarget{tabID: s.tabID})
		return
	case reloadBranchComparison:
		s.snapshot = a.Model.Active().Repository
		s.rightLabel = s.snapshot.CurrentBranch
		if i := a.repo().tabIndex(s.tabID); i >= 0 {
			a.repo().tabs[i].title = comparisonTabTitle(s)
		}
		a.loadBranchComparison(s)
		return
	case cancelBranchComparison:
		stopRead(&s.request)
		for _, side := range s.sides {
			if side == nil {
				continue
			}
			if side.request != nil {
				stopRead(&side.request)
				side.message = "Query cancelled. Load More to retry."
			}
			if side.details != nil {
				stopRead(&side.details)
				side.detailMessage = "Query cancelled. Select the commit again to retry."
			}
		}
		s.message = "Queries cancelled. Reload to capture branch identities again."
		return
	}
	side := s.sides[comparisonSideIndex(t)]
	switch id {
	case loadMoreComparisonCommits:
		a.loadComparisonSide(s, side)
	case selectComparisonCommit:
		a.loadComparisonDetails(s, side, 0)
	case comparisonParent:
		next := side.parent%len(side.commits[side.selected].ParentHashes) + 1
		a.loadComparisonDetails(s, side, next)
	case browseComparisonCommit:
		revision := side.revision
		a.openRevisionBrowser(revision)
	case openComparisonEntry:
		entry := side.commits[side.selected]
		source := entry.Hash
		if len(entry.ParentHashes) > 0 {
			parent := max(side.parent, 1) - 1
			source = (domain.RevisionComparison{Left: entry.ParentHashes[parent], Right: entry.Hash}).Source()
		}
		file := side.files[side.file]
		a.run(openDiff, commandTarget{source: source, file: file})
	}
}

func (a *Workspace) twoWayComparisonView(c *ui.Context, p tokens, s *branchComparisonDialog) {
	ui.Column(c.Key(s.tabID)).Fill().Background(p.panel).Label("Branch Comparison workspace").Children(func() {
		ui.Row(c).FillWidth().Padding(gap).Gap(gap).Children(func() {
			ui.Text(c, s.root).Grow(1).SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted)
			for _, button := range []struct {
				id    commandID
				label string
			}{{cancelBranchComparison, "Cancel Query"}, {reloadBranchComparison, "Reload"}} {
				t := commandTarget{tabID: s.tabID}
				ui.Button(c, button.label).Disabled(a.branchComparisonDisabled(button.id, t) != "").OnClick(func() { a.run(button.id, t) })
			}
		})
		if s.request != nil {
			ui.Text(c, "Reading branch identities…").Padding(gap)
		}
		if s.message != "" {
			ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
		}
		ui.SplitVerticalPercent(c, &s.split, func() { a.comparisonSideView(c.Key("top"), p, s, 0) }, func() { a.comparisonSideView(c.Key("bottom"), p, s, 1) }).SplitReset(50).Grow(1).FillWidth()
	})
}

func (a *Workspace) comparisonSideView(c *ui.Context, p tokens, s *branchComparisonDialog, index int) {
	side := s.sides[index]
	if side == nil {
		ui.Column(c).Fill()
		return
	}
	t := commandTarget{source: fmt.Sprint(index), tabID: s.tabID}
	left, right := s.label, s.rightLabel
	if index == 1 {
		left, right = right, left
	}
	ui.SplitPercent(c, &side.split, func() {
		ui.Column(c).Fill().Children(func() {
			heading(c, p, "Commits in "+left+" missing from "+right, "")
			if side.message != "" {
				ui.Text(c, side.message).FillWidth().TextColor(p.theme.Danger)
			}
			if side.request != nil {
				ui.Text(c, "Reading commits…")
			}
			side.list.Selected = &side.selected
			side.list.Key = func(i int) any { return side.commits[i].Hash }
			side.list.Label = func(i int) string { return side.commits[i].Subject }
			list := ui.List(c, &side.list, len(side.commits), func(i int) {
				entry := side.commits[i]
				ui.Row(c).FillWidth().Height(rowHeight).Gap(gap).AlignItems(ui.Center).Children(func() {
					ui.Text(c, entry.Author).Width(88).SingleLine().Ellipsis("…")
					ui.Box(c).Width(side.graphWidth).FillHeight().Draw(func(painter *ui.Painter, box ui.Rect) { drawCommit(painter, box, p, entry, side.graph[i]) })
					ui.Text(c, entry.Subject).Grow(1).SingleLine().Ellipsis("…")
					for _, ref := range entry.Refs {
						ui.Text(c, ref).MaxWidth(110).SingleLine().Ellipsis("…").TextColor(p.graph[0])
					}
					ui.Text(c, entry.Hash[:8]).Width(64).TextColor(p.theme.TextMuted)
					ui.Text(c, entry.Timestamp.Format("2006/01/02 15:04")).Width(125).TextColor(p.theme.TextMuted)
				})
			}).Grow(1).FillWidth().Label("Comparison commits " + left)
			if len(side.commits) == 0 && side.request == nil && side.message == "" {
				list.Children(func() { ui.Text(c, "No commits in this direction").Padding(inset) })
			}
			if list.Changed() || list.Submitted() {
				a.pending = func() { a.run(selectComparisonCommit, t) }
			}
			ui.Row(c).FillWidth().Padding(gap).Gap(gap).Children(func() {
				ui.Text(c, fmt.Sprintf("%d commits loaded", len(side.commits))).Grow(1)
				ui.Button(c, "Load More").Label(commandLabel(loadMoreComparisonCommits) + " " + left).Disabled(a.branchComparisonDisabled(loadMoreComparisonCommits, t) != "").OnClick(func() { a.run(loadMoreComparisonCommits, t) })
			})
		})
	}, func() {
		ui.SplitVerticalPercent(c, &side.detailSplit, func() {
			ui.Column(c).Fill().Children(func() {
				heading(c, p, "Changed Files", "")
				if side.selected >= 0 && side.selected < len(side.commits) && len(side.commits[side.selected].ParentHashes) > 1 {
					ui.Button(c, fmt.Sprintf("Relative to parent %d · Next Parent", side.parent)).OnClick(func() { a.run(comparisonParent, t) })
				}
				if side.details != nil {
					ui.Text(c, "Reading changed files…")
				}
				if side.detailMessage != "" {
					ui.Text(c, side.detailMessage).FillWidth().TextColor(p.theme.Danger)
				}
				side.fileList.Selected = &side.file
				side.fileList.Key = func(i int) any { return side.files[i].Path }
				side.fileList.Label = func(i int) string { return side.files[i].Path }
				list := ui.List(c, &side.fileList, len(side.files), func(i int) {
					file := side.files[i]
					name := file.Path
					if file.OldPath != "" {
						name = file.OldPath + " → " + name
					}
					ui.Text(c, file.Status+"  "+name).Height(rowHeight).FillWidth().SingleLine().Ellipsis("…")
				}).Grow(1).FillWidth().Label("Comparison changed files " + left)
				if side.filesReady && len(side.files) == 0 {
					list.Children(func() { ui.Text(c, "No changed files").Padding(gap) })
				}
				if list.Submitted() {
					a.pending = func() { a.run(openComparisonEntry, t) }
				}
				ui.Row(c).FillWidth().Gap(gap).Children(func() {
					ui.Button(c, "Browse Commit").Label(commandLabel(browseComparisonCommit) + " " + left).Disabled(a.branchComparisonDisabled(browseComparisonCommit, t) != "").OnClick(func() { a.run(browseComparisonCommit, t) })
					ui.Button(c, "Show Diff").Label(commandLabel(openComparisonEntry) + " " + left).Disabled(a.branchComparisonDisabled(openComparisonEntry, t) != "").OnClick(func() { a.run(openComparisonEntry, t) })
				})
			})
		}, func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
					if side.selected < 0 || side.selected >= len(side.commits) {
						ui.Text(c, "Select a commit")
						return
					}
					entry := side.commits[side.selected]
					ui.Text(c, entry.Subject).FillWidth().Bold().Selectable()
					if entry.Body != "" {
						ui.Text(c, entry.Body).FillWidth().Selectable()
					}
					commitMetadata(c, p, entry, func() { a.run(copyHash, commandTarget{source: entry.Hash}) })
					commitReferenceLabels(c, p, entry.Refs, s.snapshot)
					ui.Text(c, entry.Hash).FillWidth().Selectable().TextColor(p.theme.TextMuted)
				})
			})
		}).SplitReset(55).Fill()
	}).SplitReset(64).Fill()
}
