package ui

import (
	"context"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type historyChangeSelection struct {
	seed             []string
	files            []domain.File
	chosen           []bool
	selected, parent int
	list             ui.ListState
	request          *readRequest
	message          string
}

func (a *Workspace) stopHistoryComparison() {
	if s := a.repositoryOperation; s != nil && s.historyChanges != nil {
		stopRead(&s.historyChanges.request)
	}
}

func historyComparisonDisabled(s *repositoryOperationDialog) bool {
	h := s.historyChanges
	return h != nil && (h.request != nil || h.parent < 1 || h.parent != slices.Index(s.mainlines, s.mainline) || len(s.op.Paths) == 0 || len(s.op.Paths) > 100 || h.message != "")
}

func (a *Workspace) loadHistoryComparison(s *repositoryOperationDialog) {
	if a.repositoryOperation != s || s.historyChanges == nil {
		return
	}
	h := s.historyChanges
	stopRead(&h.request)
	h.files, h.chosen, s.op.Paths = nil, nil, nil
	h.selected = -1
	h.message = ""
	h.parent = slices.Index(s.mainlines, s.mainline)
	if h.parent < 1 {
		return
	}
	parent := h.parent
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	h.request = request
	a.ReadRepositoryRevisionChanges(ctx, s.root, s.op.Source, parent, func(files []domain.File, err error) {
		if a.repositoryOperation != s || h.request != request || ctx.Err() == context.Canceled || a.Model.Active().Repository.Root != s.root {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&h.request)
		if err != nil {
			h.message = "Could not compare merge parent · " + err.Error()
			return
		}
		if len(files) > 10000 {
			h.message = "Merge comparison exceeds 10000 files"
			return
		}
		h.files = files
		h.chosen = make([]bool, len(files))
		for i, file := range files {
			h.chosen[i] = slices.Contains(h.seed, file.Path) || file.OldPath != "" && slices.Contains(h.seed, file.OldPath)
		}
		updateHistoryComparisonPaths(s)
	})
}

func updateHistoryComparisonPaths(s *repositoryOperationDialog) {
	s.op.Paths = nil
	for i, file := range s.historyChanges.files {
		if s.historyChanges.chosen[i] {
			for _, path := range []string{file.Path, file.OldPath} {
				if path != "" && !slices.Contains(s.op.Paths, path) {
					s.op.Paths = append(s.op.Paths, path)
				}
			}
		}
	}
}

func (a *Workspace) historyComparisonFields(c *ui.Context, s *repositoryOperationDialog) {
	h := s.historyChanges
	if h.parent < 1 {
		ui.Text(c, "Choose a parent, then select changes relative to that parent.").FillWidth()
		return
	}
	if h.request != nil {
		ui.Text(c, "Loading changes relative to the selected parent…")
		return
	}
	if h.message != "" {
		ui.Text(c, h.message).FillWidth()
		return
	}
	if len(h.files) == 0 {
		ui.Text(c, "No changes relative to this parent")
		return
	}
	ui.Text(c, "Choose whole-file changes (up to 100 paths including renames)").FillWidth()
	h.list.Selected = &h.selected
	h.list.Key = func(i int) any { return h.files[i].Path }
	h.list.Label = func(i int) string { return h.files[i].Path }
	ui.List(c, &h.list, len(h.files), func(i int) {
		file := h.files[i]
		ui.Row(c).FillWidth().Height(rowHeight).Children(func() {
			if ui.Checkbox(c, &h.chosen[i], file.Status+" · "+file.Path).Label("Historical change " + file.Path).Disabled(s.cancel != nil).Changed() {
				updateHistoryComparisonPaths(s)
			}
		})
	}).FillWidth().Height(140).Label("Historical parent changes")
}
