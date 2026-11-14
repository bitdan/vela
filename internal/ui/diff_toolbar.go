package ui

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

func (a *Workspace) previewTab(c *ui.Context, p tokens, entry tab) {
	label := "Diff: " + entry.title
	if entry.source == "working" || entry.source == "staged" {
		label = "Commit: " + entry.title
	} else if entry.source == "merge-conflict" {
		label = "Conflict: " + entry.title
	}
	row := ui.Row(c.Key("temporary-diff")).Height(controlHeight).AlignItems(ui.Center).Label("Diff Preview").
		ContextMenu(func(m *ui.Menu) { a.tabMenu(m, entry, true) })
	toolUnderline(row, p)
	row.Children(func() {
		ui.Text(c, label).Padding(0, gap).SingleLine().Tooltip(entry.path)
		ui.ButtonBase(c).Size(20, 20).Center().Label("Close Preview").Tooltip("Close Preview (Esc)").
			Children(func() { ui.Text(c, "×") }).OnClick(func() { a.run(closePreview, commandTarget{}) })
	})
}

func (a *Workspace) previewFiles(source string) []domain.File {
	s := a.Model.Active()
	switch source {
	case "working":
		if a.repo().showIgnored {
			return appendIgnored(s.Repository.Working)
		}
		return s.Repository.Working
	case "staged":
		return s.Repository.Staged
	}
	return nil
}

func (a *Workspace) diffToolbar(c *ui.Context, p tokens, entry *tab) {
	diff := entry.diff
	real := a.Model.Active().Repository.Root != ""
	files := a.previewFiles(entry.source)
	index := slices.IndexFunc(files, func(file domain.File) bool { return file.Path == entry.path })
	navigate := func(next int) {
		file := files[next]
		s := a.Model.Active()
		if entry.source == "working" {
			s.Working = file.Path
		} else {
			s.Staged = file.Path
		}
		a.repo().commitSource = entry.source
		a.run(previewDiff, commandTarget{source: entry.source, file: file})
		if !real {
			a.repo().preview.diff.Layout = diff.Layout
		}
	}
	ui.Row(c).Height(headerHeight).FillWidth().Padding(0, gap).Gap(2).AlignItems(ui.Center).Background(p.chrome).Children(func() {
		button := func(glyph, label string, disabled bool, action func()) {
			ui.ButtonBase(c.Key(label)).Size(24, 24).Center().Label(label).Tooltip(label).Disabled(disabled).
				Children(func() { ui.Text(c, glyph) }).OnClick(action)
		}
		hunks := diff.Hunks()
		step := func(delta int) {
			current, inside := diff.CurrentHunk()
			if !inside && delta > 0 {
				current--
			}
			diff.GoToHunk(max(0, min(current+delta, len(hunks)-1)))
		}
		button("↑", "Previous difference", len(hunks) == 0, func() { step(-1) })
		button("↓", "Next difference", len(hunks) == 0, func() { step(1) })
		button("✎", "Open source", false, func() { a.run(openCode, commandTarget{source: entry.source, file: entry.file}) })
		if index >= 0 {
			button("←", "Previous changed file", index == 0, func() { navigate(index - 1) })
			ui.Text(c, fmt.Sprintf("%d/%d files", index+1, len(files))).FontSize(p.textSize(smallText))
			button("→", "Next changed file", index == len(files)-1, func() { navigate(index + 1) })
		}
		button("＋", "Keep Diff in New Tab", false, func() { a.run(openDiff, commandTarget{source: entry.source, file: entry.file}) })
		if real && (entry.source == "working" || entry.source == "staged") {
			target := commandTarget{source: entry.source, file: entry.file}
			id := stageHunk
			if entry.source == "staged" {
				id = unstageHunk
			}
			button("±", commandLabel(id), a.disabledFor(id, target) != "", func() { a.run(id, target) })
			if entry.source == "working" {
				button("↶", commandLabel(discardHunk), a.disabledFor(discardHunk, target) != "", func() { a.run(discardHunk, target) })
			}
		}
		ui.Box(c).Grow(1)
		ui.Text(c, fmt.Sprintf("%d differences", len(hunks))).FontSize(p.textSize(smallText))
		button("◫", "Side-by-side Diff", diff.Layout == editor.DiffSplit, func() { diff.Layout = editor.DiffSplit })
		button("▤", "Unified Diff", diff.Layout == editor.DiffUnified, func() { diff.Layout = editor.DiffUnified })
	})
	divider(c, p)
	ui.Row(c).FillWidth().Height(rowHeight).Padding(0, inset).Gap(gap).AlignItems(ui.Center).Children(func() {
		old, next := "Before", "Current version"
		if entry.source == "working" {
			old = "Base version"
		}
		if entry.source == "staged" {
			old, next = "Base version", "Staged version"
		}
		if real {
			switch entry.source {
			case "working":
				old, next = "Index", "Working Tree"
			case "staged":
				old, next = "HEAD", "Index"
			case "merge-conflict":
				repo := a.Model.Active().Repository
				old, next = conflictPaneLabels(repo.Rebase, repo.Sequencer)
				if conflict, ok := findMergeConflict(a.Model.Active().Repository, entry.path); ok {
					if conflict.Current == "" {
						old += " (deleted)"
					}
					if conflict.Incoming == "" {
						next += " (deleted)"
					}
				}
			}
		}
		ui.Text(c, old+"  ·  "+entry.path).Grow(1).SingleLine().Ellipsis("…").FontSize(p.textSize(smallText))
		if diff.Layout == editor.DiffSplit {
			ui.Text(c, next).Grow(1).FontSize(p.textSize(smallText))
		}
	})
	divider(c, p)
}
