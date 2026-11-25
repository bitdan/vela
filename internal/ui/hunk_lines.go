package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

const (
	toggleHunkLineMode commandID = "patch.toggle-line-mode"
	toggleHunkLine     commandID = "patch.toggle-changed-line"
	applyHunkSelection commandID = "patch.apply-hunk-selection"
	clearHunkLines     commandID = "patch.clear-changed-lines"
	selectAllHunkLines commandID = "patch.select-all-changed-lines"
)

func isHunkLineCommand(id commandID) bool {
	return id == toggleHunkLineMode || id == toggleHunkLine || id == applyHunkSelection || id == clearHunkLines || id == selectAllHunkLines
}

func (a *Workspace) closeIdleHunkDialog() {
	if s := a.patchDialog; s != nil && !s.busy {
		if s.cancel != nil {
			s.cancel()
		}
		a.patchDialog = nil
	}
}

func (s *patchDialog) changedLines() ([]application.ChangedPatchLine, []bool) {
	n := slices.Index(s.choices, s.choice)
	if n < 0 || n >= len(s.doc.Hunks) {
		return nil, nil
	}
	if s.lineRows == nil {
		s.lineRows = make(map[int][]application.ChangedPatchLine)
	}
	lines, found := s.lineRows[n]
	if !found {
		var err error
		lines, err = application.ChangedPatchLines(s.doc.Hunks[n])
		if err != nil {
			return nil, nil
		}
		s.lineRows[n] = lines
	}
	if s.lineChoices == nil {
		s.lineChoices = make(map[int][]bool)
	}
	chosen, ok := s.lineChoices[n]
	if !ok {
		chosen = make([]bool, len(lines))
		for i := range chosen {
			chosen[i] = true
		}
		s.lineChoices[n] = chosen
	}
	return lines, chosen
}

func (s *patchDialog) chosenLineOrdinals() []int {
	_, chosen := s.changedLines()
	var out []int
	for i, yes := range chosen {
		if yes {
			out = append(out, i)
		}
	}
	return out
}

func (a *Workspace) hunkLineDisabled(id commandID, t commandTarget) string {
	s := a.patchDialog
	if s == nil {
		return "No Git hunk selection"
	}
	if s.reading || s.busy || s.snapshot.Root != a.Model.Active().Repository.Root || a.repositoryWrite != "" || s.doc.Version == "" {
		return "Wait for a current Git hunk preview"
	}
	if id == applyHunkSelection {
		if s.id == discardHunk && !s.ack {
			return "Confirm permanent discard"
		}
		if s.lineMode {
			_, err := application.SelectPatchLines(s.doc, slices.Index(s.choices, s.choice), s.chosenLineOrdinals(), s.id == unstageHunk)
			if err != nil {
				return err.Error()
			}
		}
		return ""
	}
	if s.id == discardHunk {
		return "Individual lines are available for stage/unstage only"
	}
	if (id == clearHunkLines || id == selectAllHunkLines) && !s.lineMode {
		return "Choose individual changed lines first"
	}
	if id == toggleHunkLine {
		_, chosen := s.changedLines()
		if !s.lineMode || t.index < 0 || t.index >= len(chosen) {
			return "Choose a changed line"
		}
	}
	return ""
}

func (a *Workspace) runHunkLine(id commandID, t commandTarget) {
	s := a.patchDialog
	switch id {
	case applyHunkSelection:
		a.applySelectedHunk(s)
		return
	case toggleHunkLineMode:
		s.lineMode = !s.lineMode
	case toggleHunkLine:
		_, chosen := s.changedLines()
		chosen[t.index] = !chosen[t.index]
	case clearHunkLines, selectAllHunkLines:
		_, chosen := s.changedLines()
		for i := range chosen {
			chosen[i] = id == selectAllHunkLines
		}
	}
	s.shown = ""
	s.updatePreview()
}

func (a *Workspace) hunkLineControls(c *ui.Context, s *patchDialog) {
	if s.id == discardHunk {
		return
	}
	mode := s.lineMode
	reason := a.hunkLineDisabled(toggleHunkLineMode, commandTarget{})
	ui.Checkbox(c, &mode, "Choose individual changed lines").Disabled(reason != "").Tooltip(reason).OnChange(func() { a.run(toggleHunkLineMode, commandTarget{}) })
	if !s.lineMode {
		return
	}
	lines, chosen := s.changedLines()
	ui.Row(c).FillWidth().Gap(gap).Children(func() {
		ui.Text(c, fmt.Sprintf("%d / %d changed lines", len(s.chosenLineOrdinals()), len(lines))).Grow(1)
		for _, id := range []commandID{selectAllHunkLines, clearHunkLines} {
			ui.Button(c, commandLabel(id)).Disabled(s.busy).OnClick(func() { a.run(id, commandTarget{}) })
		}
	})
	s.lineList.Key = func(i int) any { return fmt.Sprintf("%s:%d", s.choice, lines[i].Ordinal) }
	ui.List(c, &s.lineList, len(lines), func(i int) {
		label := fmt.Sprintf("Changed line %d: %s", i+1, strings.TrimSuffix(strings.TrimSuffix(lines[i].Text, "\n"), "\r"))
		checked := chosen[i]
		ui.Checkbox(c.Key(i), &checked, label).Label(label).FillWidth().Disabled(s.busy).OnChange(func() { a.run(toggleHunkLine, commandTarget{index: i}) })
	}).FillWidth().Height(min(float32(len(lines))*rowHeight, 5*rowHeight)).Label("Changed Git lines")
	if s.lineError != "" {
		ui.Text(c, s.lineError).FillWidth()
	}
}
