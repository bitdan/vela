package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	previewShelfHunks commandID = "shelf.preview-hunks"
	toggleShelfHunk   commandID = "shelf.toggle-hunk"
	wholeShelfFile    commandID = "shelf.use-whole-file"
)

func (a *Workspace) loadShelfHunks(s *repositoryOperationDialog, path string) {
	if _, ok := s.shelfHunkDocs[path]; ok {
		s.shelfHunkPath = path
		s.message = ""
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	s.shelfHunkRead = request
	if s.shelfHunkDocs == nil {
		s.shelfHunkDocs = map[string]domain.PatchDocument{}
		s.shelfHunkChosen = map[string][]bool{}
	}
	delete(s.shelfHunkDocs, path)
	delete(s.shelfHunkChosen, path)
	s.shelfChosen[path] = false
	s.shelfHunkPath = ""
	a.rebuildShelfSelection(s)
	source := "shelf:" + s.op.Target
	a.ReadRepositoryPatch(ctx, s.snapshot, source, path, func(doc domain.PatchDocument, err error) {
		if a.repositoryOperation != s || s.shelfHunkRead != request || a.Model.Active().Repository.Root != s.root {
			if s.shelfHunkRead == request {
				s.shelfHunkRead = nil
			}
			cancel()
			return
		}
		s.shelfHunkRead = nil
		if err == nil {
			err = ctx.Err()
		}
		cancel()
		if err != nil {
			s.message = "Could not preview Shelf hunks · " + err.Error()
			return
		}
		if doc.Path != path || doc.Source != source || doc.Version == "" || len(doc.Hunks) == 0 || len(doc.Hunks) > application.MaxImportedPatchHunks {
			s.message = "Invalid Shelf hunk preview; reopen the selection"
			return
		}
		size := len(doc.Header)
		for _, h := range doc.Hunks {
			size += len(h.Text)
		}
		for _, loaded := range s.shelfHunkDocs {
			size += len(loaded.Header)
			for _, h := range loaded.Hunks {
				size += len(h.Text)
			}
		}
		if size > application.MaxImportedPatchBytes {
			s.message = "Shelf hunk previews exceed 2 MiB; use smaller selections"
			return
		}
		s.shelfHunkDocs[path] = doc
		s.shelfHunkChosen[path] = make([]bool, len(doc.Hunks))
		for i := range doc.Hunks {
			s.shelfHunkChosen[path][i] = true
		}
		s.shelfHunkPath = path
		a.rebuildShelfSelection(s)
	})
}

func (a *Workspace) shelfHunkFields(c *ui.Context, p tokens, s *repositoryOperationDialog, busy bool) {
	if s.shelfHunkRead != nil {
		ui.Text(c, "Loading Shelf Git hunks…").TextColor(p.theme.TextMuted)
		return
	}
	path := s.shelfHunkPath
	doc, ok := s.shelfHunkDocs[path]
	if !ok {
		return
	}
	ui.Row(c).FillWidth().Gap(gap).Children(func() {
		ui.Text(c, "Git hunks · "+path).Grow(1).SingleLine().Ellipsis("…")
		ui.Button(c, "Whole File").Label("Use whole Shelf file " + path).Disabled(busy).OnClick(func() { a.run(wholeShelfFile, commandTarget{source: path}) })
	})
	s.shelfHunkList.Key = func(i int) any { return i }
	ui.List(c, &s.shelfHunkList, len(doc.Hunks), func(i int) {
		h := doc.Hunks[i]
		checked := s.shelfHunkChosen[path][i]
		label := strings.SplitN(h.Text, "\n", 2)[0]
		ui.Column(c).FillWidth().Height(2 * rowHeight).Children(func() {
			ui.Checkbox(c, &checked, label).Label(fmt.Sprintf("Unshelve hunk %s #%d", path, i+1)).FillWidth().Tooltip(shelfHunkExcerpt(h.Text, 2000)).Disabled(busy).OnChange(func() { a.run(toggleShelfHunk, commandTarget{source: path, index: i}) })
			ui.Text(c, shelfHunkBrief(h.Text)).FillWidth().SingleLine().Ellipsis("…").TextColor(p.theme.TextMuted)
		})
	}).FillWidth().Height(min(8*rowHeight, 2*float32(len(doc.Hunks))*rowHeight)).Label("Unshelve Git hunks")
	ui.Text(c, "Hunk selections retain the full Shelf and do not restore saved staged state. Git can report conflicts in the worktree and Index.").FillWidth().TextColor(p.theme.TextMuted)
}

func shelfHunkExcerpt(text string, limit int) string {
	r := []rune(text)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return text
}
func shelfHunkBrief(text string) string {
	old, next := "", ""
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "-") && old == "" {
			old = shelfHunkExcerpt(line, 140)
		}
		if strings.HasPrefix(line, "+") && next == "" {
			next = shelfHunkExcerpt(line, 140)
		}
		if old != "" && next != "" {
			break
		}
	}
	if old != "" && next != "" {
		return old + " → " + next
	}
	return old + next
}
