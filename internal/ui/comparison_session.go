package ui

import (
	"slices"
	"strings"

	"vela/internal/application"
	"vela/internal/domain"
)

func comparisonSnapshot(s *branchComparisonDialog) *application.ComparisonSnapshot {
	saved := &application.ComparisonSnapshot{Ref: s.ref, Label: s.label, RightLabel: s.rightLabel, Left: s.comparison.Left, Right: s.comparison.Right, Boundary: s.comparison.AncestryBoundary, Split: s.split}
	for i, side := range s.sides {
		if side == nil {
			saved.Sides[i].Filter = s.filters[i]
			saved.Sides[i].Draft = s.filters[i]
			continue
		}
		entry := application.ComparisonSideSnapshot{Filter: side.comparison.Filter, Draft: side.draft, Selected: selectedComparisonCommits(side), Loaded: len(side.commits), Split: side.split, DetailSplit: side.detailSplit, ClosedGroups: slices.Clone(side.closedGroups)}
		entry.File, entry.FileSource = side.restoreFile, side.restoreSource
		if side.selected >= 0 && side.selected < len(side.commits) {
			entry.ActiveCommit = side.commits[side.selected].Hash
		}
		if side.file >= 0 && side.file < len(side.files) {
			entry.File = side.files[side.file].Path
			if side.file < len(side.fileSources) {
				entry.FileSource = side.fileSources[side.file]
			}
		}
		// A second exit during partial restoration must retain unread identities.
		if side.restore != nil {
			entry.Selected = slices.Clone(side.restore.Selected)
			entry.ActiveCommit = side.restore.ActiveCommit
			entry.Loaded = side.restore.Loaded
		}
		saved.Sides[i] = entry
	}
	return saved
}

func cloneComparisonSnapshot(value *application.ComparisonSnapshot) *application.ComparisonSnapshot {
	if value == nil {
		return nil
	}
	out := *value
	for i := range out.Sides {
		out.Sides[i].Selected = slices.Clone(out.Sides[i].Selected)
		out.Sides[i].ClosedGroups = slices.Clone(out.Sides[i].ClosedGroups)
	}
	return &out
}

func (a *Workspace) restoreComparisonTab(entry application.TabSnapshot) bool {
	value := cloneComparisonSnapshot(entry.Comparison)
	if value == nil || !fixedObjectID(value.Left) || !fixedObjectID(value.Right) || !strings.HasPrefix(entry.Source, "branch-compare:") || len(entry.Source) > 256 || a.ReadRepositoryComparisonCommits == nil {
		return false
	}
	for _, side := range value.Sides {
		if application.ValidateComparisonLogFilter(side.Filter) != nil || side.Loaded < 0 || side.Loaded > 100000 || len(side.Selected) > 1000 {
			return false
		}
		for _, id := range side.Selected {
			if !fixedObjectID(id) {
				return false
			}
		}
	}
	r := a.repo()
	if r.tabIndex(entry.Source) >= 0 {
		return true
	}
	s := &branchComparisonDialog{open: true, ready: true, mode: "committed", root: a.Model.Active().Repository.Root, ref: value.Ref, label: value.Label, rightLabel: value.RightLabel, snapshot: a.Model.Active().Repository, tabID: entry.Source, split: comparisonSplit(value.Split, 50), restore: value, comparison: domain.RevisionComparison{Left: value.Left, Right: value.Right, AncestryBoundary: value.Boundary}}
	for i := range s.filters {
		s.filters[i] = value.Sides[i].Filter
	}
	r.tabs = append(r.tabs, tab{id: s.tabID, title: comparisonTabTitle(s), comparison: s, pinned: entry.Pinned})
	a.startTwoWayComparison(s)
	s.restore = nil
	return true
}

func comparisonSplit(value, fallback float32) float32 {
	if !(value >= 10 && value <= 90) {
		return fallback
	}
	return value
}

func (a *Workspace) prepareComparisonSideRestore(side *comparisonSide, value application.ComparisonSideSnapshot) {
	side.split, side.detailSplit = comparisonSplit(value.Split, 64), comparisonSplit(value.DetailSplit, 55)
	side.draft = value.Draft
	side.restore = &value
	side.closedGroups = slices.Clone(value.ClosedGroups)
	side.restoreFile, side.restoreSource = value.File, value.FileSource
}

func (a *Workspace) continueComparisonSideRestore(s *branchComparisonDialog, side *comparisonSide) {
	saved := side.restore
	if side.more && len(side.commits) < saved.Loaded {
		a.loadComparisonSide(s, side)
		return
	}
	side.restore = nil
	side.chosen.Clear()
	for i, c := range side.commits {
		if c.Hash == saved.ActiveCommit {
			side.selected = i
		}
		if slices.Contains(saved.Selected, c.Hash) {
			side.chosen.Add(c.Hash)
		}
	}
	if side.selected < 0 && len(side.commits) > 0 {
		side.selected = 0
	}
	if side.chosen.Len() == 0 && side.selected >= 0 && saved.Loaded == 0 {
		side.chosen.Add(side.commits[side.selected].Hash)
	}
	if side.selected >= 0 {
		side.list.ScrollIntoView(side.selected)
		a.loadComparisonDetails(s, side, 0)
	}
}
