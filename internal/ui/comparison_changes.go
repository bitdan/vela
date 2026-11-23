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

func selectedComparisonCommits(side *comparisonSide) []string {
	var ids []string
	for _, commit := range side.commits {
		if side.chosen.Has(commit.Hash) {
			ids = append(ids, commit.Hash)
		}
	}
	return ids
}

func (a *Workspace) loadComparisonSelectionDetails(s *branchComparisonDialog, side *comparisonSide) {
	stopRead(&side.details)
	side.files, side.groups, side.fileSources = nil, nil, nil
	side.filesReady, side.file, side.groupRow, side.detailMessage = false, -1, -1, ""
	ids := selectedComparisonCommits(side)
	if len(ids) == 0 {
		side.revision = ""
		return
	}
	if len(ids) > 1000 {
		side.detailMessage = "Select at most 1000 commits"
		return
	}
	if side.selected >= 0 && side.selected < len(side.commits) {
		side.revision = side.commits[side.selected].Hash
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	side.details = request
	a.ReadRepositoryComparisonChanges(ctx, s.root, side.comparison, ids, func(groups []domain.ComparisonChangeGroup, err error) {
		if !a.comparisonSideAlive(s, side) || side.details != request {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&side.details)
		if err != nil {
			side.detailMessage = "Could not read selected changes · " + err.Error()
			return
		}
		var files []domain.File
		var sources []string
		for _, group := range groups {
			_, _, valid := domain.ParseComparisonSource(group.Source)
			if !valid && !fixedObjectID(group.Source) {
				side.detailMessage = "Invalid comparison group source"
				return
			}
			for _, file := range group.Files {
				if !application.ValidProjectSearchPath(file.Path) || file.OldPath != "" && !application.ValidProjectSearchPath(file.OldPath) {
					side.detailMessage = "Invalid historical path"
					return
				}
				files = append(files, file)
				sources = append(sources, group.Source)
			}
		}
		if len(files) > 10000 || len(groups) > 10000 {
			side.detailMessage = "Too many comparison changes"
			return
		}
		side.groups, side.files, side.fileSources, side.filesReady = groups, files, sources, true
		side.groupOutline = ui.OutlineState[int]{}
		for i, g := range groups {
			if !slices.Contains(side.closedGroups, g.Source) {
				side.groupOutline.Open.Add(-i - 1)
			}
		}
		if side.restoreFile != "" {
			for i, f := range files {
				if f.Path == side.restoreFile && sources[i] == side.restoreSource {
					side.file = i
					break
				}
			}
			side.restoreFile, side.restoreSource = "", ""
		}
	})
}

func (a *Workspace) comparisonChangedFilesView(c *ui.Context, p tokens, s *branchComparisonDialog, side *comparisonSide, target commandTarget, label string) {
	if a.ReadRepositoryComparisonChanges == nil {
		side.fileList.Selected = &side.file
		side.fileList.Key = func(i int) any { return side.files[i].Path }
		side.fileList.Label = func(i int) string { return side.files[i].Path }
		list := ui.List(c, &side.fileList, len(side.files), func(i int) { comparisonFileLabel(c, side.files[i], label) }).Grow(1).FillWidth().Label("Comparison changed files " + label)
		if list.Submitted() {
			a.pending = func() { a.run(openComparisonEntry, target) }
		}
		if side.filesReady && len(side.files) == 0 {
			list.Children(func() { ui.Text(c, "No changed files").Padding(gap) })
		}
		return
	}
	var roots []int
	children := make(map[int][]int, len(side.groups))
	offset := 0
	for i, group := range side.groups {
		key := -i - 1
		roots = append(roots, key)
		children[key] = []int{}
		for range group.Files {
			children[key] = append(children[key], offset)
			offset++
		}
	}
	state := &side.groupOutline
	state.List.Selected = &side.groupRow
	state.List.Label = func(row int) string {
		key := state.Item(row)
		if key < 0 {
			return side.groups[-key-1].Title
		}
		return side.files[key].Path
	}
	list := ui.Outline(c, state, roots, func(key int) []int { return children[key] }, func(key int) {
		if key < 0 {
			group := side.groups[-key-1]
			ui.Text(c, fmt.Sprintf("%s (%d)", group.Title, len(group.Files))).Label("Comparison group " + label + " " + group.Source).Bold().FillWidth().SingleLine().Ellipsis("…").Height(rowHeight)
		} else {
			comparisonFileLabel(c, side.files[key], label)
		}
	}).Grow(1).FillWidth().Label("Comparison changed files " + label)
	if side.groupRow < 0 && side.file >= 0 {
		for row := 0; row < state.Rows(); row++ {
			if state.Item(row) == side.file {
				side.groupRow = row
				state.List.ScrollIntoView(row)
				break
			}
		}
	}
	// Preserve collapse state by diff identity, rather than by row index.
	if side.filesReady {
		side.closedGroups = nil
		for i, g := range side.groups {
			if !state.Open.Has(-i - 1) {
				side.closedGroups = append(side.closedGroups, g.Source)
			}
		}
	}
	if list.Changed() && side.groupRow >= 0 && side.groupRow < state.Rows() {
		side.file = state.Item(side.groupRow)
	}
	if list.Submitted() && side.file >= 0 {
		a.pending = func() { a.run(openComparisonEntry, target) }
	}
	if side.filesReady && len(side.files) == 0 {
		ui.Text(c, "No changed files").Padding(gap)
	}
}

func comparisonFileLabel(c *ui.Context, file domain.File, label string) {
	name := file.Path
	if file.OldPath != "" {
		name = file.OldPath + " → " + name
	}
	ui.Text(c, file.Status+"  "+name).Label("Comparison file " + label + " " + file.Path).Height(rowHeight).FillWidth().SingleLine().Ellipsis("…")
}
