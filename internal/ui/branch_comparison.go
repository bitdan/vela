package ui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	closeBranchComparison     commandID = "branches.close-comparison"
	reloadBranchComparison    commandID = "branches.reload-comparison"
	loadMoreComparisonCommits commandID = "branches.load-more-missing"
	cancelBranchComparison    commandID = "branches.cancel-comparison"
	openComparisonEntry       commandID = "branches.open-comparison-diff"
	browseComparisonCommit    commandID = "branches.browse-missing-commit"
)

type branchComparisonDialog struct {
	open, ready, more                           bool
	root, ref, label, rightLabel, mode, message string
	snapshot                                    domain.Repository
	comparison                                  domain.RevisionComparison
	commits                                     []domain.Commit
	selected                                    int
	list                                        ui.ListState
	request                                     *readRequest
}

func isBranchComparisonEntry(id commandID) bool {
	return id == branchDiff || id == compareCurrentBranch || id == missingBranchCommits
}
func isBranchComparisonCommand(id commandID) bool {
	return isBranchComparisonEntry(id) || id == closeBranchComparison || id == reloadBranchComparison || id == loadMoreComparisonCommits || id == cancelBranchComparison || id == openComparisonEntry || id == browseComparisonCommit
}
func (a *Workspace) closeBranchComparisonDialog() {
	if s := a.branchComparison; s != nil {
		stopRead(&s.request)
		a.branchComparison = nil
	}
}

func (a *Workspace) branchComparisonDisabled(id commandID, t commandTarget) string {
	if isBranchComparisonEntry(id) {
		r := a.Model.Active().Repository
		if a.ReadRepositoryComparison == nil {
			return "Branch comparison reader is unavailable"
		}
		if a.repositoryRead != nil || a.repositoryWrite != "" || a.repositoryOperation != nil {
			return "Wait for the current repository operation"
		}
		if r.Head == "" || r.IndexVersion == "" || operationRef(r, t.source) == "" {
			return "Select a committed local branch, remote branch or tag"
		}
		if id == missingBranchCommits && a.ReadRepositoryComparisonCommits == nil {
			return "Missing-commit reader is unavailable"
		}
		return ""
	}
	s := a.branchComparison
	if s == nil {
		return "Open a branch comparison"
	}
	if id == closeBranchComparison || id == reloadBranchComparison {
		return ""
	}
	if id == cancelBranchComparison {
		if s.request == nil {
			return "No query is running"
		}
		return ""
	}
	if s.request != nil || !s.ready {
		return "Wait for the branch comparison"
	}
	if id == loadMoreComparisonCommits {
		if s.mode != "missing" || !s.more {
			return "All missing commits are loaded"
		}
		return ""
	}
	if s.mode == "missing" {
		if s.selected < 0 || s.selected >= len(s.commits) {
			return "Select a missing commit"
		}
		if a.ReadRepositoryRevisionTree == nil {
			return "Historical repository browser unavailable"
		}
	} else {
		if s.selected < 0 || s.selected >= len(s.comparison.Files) {
			return "Select a changed file"
		}
		if a.ReadRepositoryFile == nil {
			return "File reader unavailable"
		}
	}
	return ""
}

func (a *Workspace) runBranchComparison(id commandID, t commandTarget) {
	if isBranchComparisonEntry(id) {
		a.closeBranchComparisonDialog()
		r := a.Model.Active().Repository
		mode := "committed"
		if id == branchDiff {
			mode = "working"
		}
		if id == missingBranchCommits {
			mode = "missing"
		}
		s := &branchComparisonDialog{open: true, root: r.Root, ref: operationRef(r, t.source), label: t.source, rightLabel: r.CurrentBranch, mode: mode, snapshot: r, selected: -1}
		a.branchComparison = s
		a.loadBranchComparison(s)
		return
	}
	s := a.branchComparison
	switch id {
	case closeBranchComparison:
		a.closeBranchComparisonDialog()
	case reloadBranchComparison:
		a.loadBranchComparison(s)
	case cancelBranchComparison:
		stopRead(&s.request)
		s.message = "Query cancelled. Reload or load more to retry."
	case loadMoreComparisonCommits:
		a.loadComparisonCommits(s)
	case browseComparisonCommit:
		oid := s.commits[s.selected].Hash
		a.closeBranchComparisonDialog()
		a.run(showRepositoryRevision, commandTarget{source: oid})
	case openComparisonEntry:
		target := commandTarget{source: s.comparison.Source(), file: s.comparison.Files[s.selected]}
		a.closeBranchComparisonDialog()
		a.run(openDiff, target)
	}
}

func (a *Workspace) loadBranchComparison(s *branchComparisonDialog) {
	stopRead(&s.request)
	s.ready = false
	s.more = false
	s.message = ""
	s.selected = -1
	s.commits = nil
	s.comparison = domain.RevisionComparison{}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	a.ReadRepositoryComparison(ctx, s.snapshot, s.ref, s.mode, func(result domain.RevisionComparison, err error) {
		if a.branchComparison != s || s.request != request || a.Model.Active().Repository.Root != s.root {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		if err != nil {
			s.message = "Could not compare branches · " + err.Error()
			return
		}
		if !fixedObjectID(result.Left) || s.mode != "working" && !fixedObjectID(result.Right) || len(result.Files) > 10000 {
			s.message = "Invalid branch comparison response"
			return
		}
		s.comparison = result
		s.ready = true
		if s.mode == "missing" {
			s.more = true
			a.loadComparisonCommits(s)
		}
	})
}

func (a *Workspace) loadComparisonCommits(s *branchComparisonDialog) {
	if s.request != nil || !s.more {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.request = request
	s.message = ""
	offset := len(s.commits)
	a.ReadRepositoryComparisonCommits(ctx, s.root, s.comparison, offset, 100, func(page domain.CommitLogPage, err error) {
		if a.branchComparison != s || s.request != request || a.Model.Active().Repository.Root != s.root {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&s.request)
		if err != nil {
			s.message = "Could not read missing commits · " + err.Error()
			return
		}
		if len(page.Commits) > 100 || len(page.Commits) == 0 && page.HasMore || len(s.commits) != offset {
			s.message = "Invalid missing-commit page"
			return
		}
		seen := map[string]bool{}
		for _, commit := range s.commits {
			seen[commit.Hash] = true
		}
		for _, commit := range page.Commits {
			if !fixedObjectID(commit.Hash) || seen[commit.Hash] {
				s.message = "Repeated or invalid missing commit"
				return
			}
			seen[commit.Hash] = true
		}
		s.commits = append(slices.Clone(s.commits), page.Commits...)
		s.more = page.HasMore
	})
}

func (a *Workspace) branchComparisonView(c *ui.Context, p tokens) {
	s := a.branchComparison
	w, h := c.Size()
	title := "Compare Branches"
	if s.mode == "working" {
		title = "Branch vs Working Tree"
	}
	if s.mode == "missing" {
		title = "Missing Commits"
	}
	ui.DialogBase(c, &s.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(w-32, 920)).Height(min(h-32, 650)).Background(p.panel).Border(1, p.theme.Border).Radius(6).Label("Branch Comparison dialog").Column().Children(func() {
			heading(c, p, title, "")
			ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
				ui.Text(c, s.root).SingleLine().Ellipsis("…").FillWidth()
				direction := s.label + " → " + s.rightLabel
				if s.mode == "working" {
					direction = s.label + " → tracked working tree snapshot"
				}
				if s.mode == "missing" {
					direction = "Commits in " + s.label + " missing from " + s.rightLabel
				}
				ui.Text(c, direction).FillWidth()
				if s.ready {
					right := s.comparison.Right
					if right == "" {
						right = "working snapshot"
					}
					ui.Text(c, s.comparison.Left+" → "+right).FillWidth().SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted)
				}
				if s.request != nil {
					ui.Text(c, "Reading branch comparison…")
				}
				if s.message != "" {
					ui.Text(c, s.message).FillWidth().TextColor(p.theme.Danger)
				}
			})
			count := len(s.comparison.Files)
			if s.mode == "missing" {
				count = len(s.commits)
			}
			s.list.Selected = &s.selected
			s.list.Key = func(i int) any {
				if s.mode == "missing" {
					return s.commits[i].Hash
				}
				return s.comparison.Files[i].Path
			}
			s.list.Label = func(i int) string {
				if s.mode == "missing" {
					return s.commits[i].Subject
				}
				return s.comparison.Files[i].Path
			}
			list := ui.List(c, &s.list, count, func(i int) {
				ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Children(func() {
					if s.mode == "missing" {
						entry := s.commits[i]
						ui.Text(c, entry.Hash[:min(10, len(entry.Hash))]).Width(88).TextColor(p.theme.TextMuted)
						ui.Text(c, entry.Subject).Grow(1).SingleLine().Ellipsis("…")
						ui.Text(c, entry.Author).Width(120).SingleLine().Ellipsis("…")
					} else {
						file := s.comparison.Files[i]
						ui.Text(c, file.Status).Width(24)
						name := file.Path
						if file.OldPath != "" {
							name = file.OldPath + " → " + file.Path
						}
						ui.Text(c, name).Grow(1).SingleLine().Ellipsis("…")
					}
				})
			}).Grow(1).FillWidth().Label("Branch Comparison results")
			if count == 0 && s.ready && s.request == nil && s.message == "" {
				list.Children(func() {
					text := "No changed tracked files"
					if s.mode == "missing" {
						text = "No commits missing from the current branch"
					}
					ui.Text(c, text).Padding(inset).TextColor(p.theme.TextMuted)
				})
			}
			if list.Submitted() {
				id := openComparisonEntry
				if s.mode == "missing" {
					id = browseComparisonCommit
				}
				a.pending = func() { a.run(id, commandTarget{}) }
			}
			divider(c, p)
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				ui.Text(c, fmt.Sprintf("%d results loaded", count)).Grow(1)
				ui.Button(c, "Cancel Query").Label(commandLabel(cancelBranchComparison)).Disabled(a.branchComparisonDisabled(cancelBranchComparison, commandTarget{}) != "").OnClick(func() { a.run(cancelBranchComparison, commandTarget{}) })
				ui.Button(c, "Reload").OnClick(func() { a.run(reloadBranchComparison, commandTarget{}) })
				if s.mode == "missing" {
					ui.Button(c, "Load More").Label(commandLabel(loadMoreComparisonCommits)).Disabled(a.branchComparisonDisabled(loadMoreComparisonCommits, commandTarget{}) != "").OnClick(func() { a.run(loadMoreComparisonCommits, commandTarget{}) })
				}
				ui.Button(c, "Close").Label(commandLabel(closeBranchComparison)).OnClick(func() { a.run(closeBranchComparison, commandTarget{}) })
				id := openComparisonEntry
				label := "Show Diff"
				if s.mode == "missing" {
					id = browseComparisonCommit
					label = "Browse Commit"
				}
				ui.PrimaryButton(c, label).Label(commandLabel(id)).Disabled(a.branchComparisonDisabled(id, commandTarget{}) != "").OnClick(func() { a.run(id, commandTarget{}) })
			})
		})
	})
	if !s.open {
		a.closeBranchComparisonDialog()
	}
}
