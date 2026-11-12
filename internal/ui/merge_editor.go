package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

type mergeEditorState struct {
	doc                       domain.MergeDocument
	current, result, incoming *editor.State
	blocks                    []domain.ConflictBlock
	incomplete                bool
	version                   uint64
	selected                  int
	leftSplit, rightSplit     float32
	focusResult, reload       bool
}

func newMergeEditor(doc domain.MergeDocument) *mergeEditorState {
	makeState := func(text string, readOnly bool) *editor.State {
		buffer := editor.NewBuffer(text)
		buffer.SetLanguage(editor.LanguageFor(doc.Conflict.Path))
		return &editor.State{Buffer: buffer, ReadOnly: readOnly}
	}
	d := &mergeEditorState{doc: doc, current: makeState(doc.Current, true), result: makeState(doc.Result, false),
		incoming: makeState(doc.Incoming, true), leftSplit: 32, rightSplit: 54, focusResult: true}
	d.parse()
	d.showBlock()
	return d
}

func (d *mergeEditorState) parse() {
	d.version = d.result.Buffer.Version()
	d.blocks, d.incomplete = domain.ConflictBlocks(d.result.Buffer.String(), d.doc.MarkerSize)
	d.selected = max(0, min(d.selected, len(d.blocks)-1))
}

func (d *mergeEditorState) showBlock() {
	if len(d.blocks) == 0 {
		return
	}
	block := d.blocks[d.selected]
	d.result.GoTo(d.result.Buffer.Pos(block.Start))
	d.result.Select(block.Start, block.End)
	for _, side := range []struct {
		state *editor.State
		text  string
	}{{d.current, block.Current}, {d.incoming, block.Incoming}} {
		if offset := strings.Index(side.state.Buffer.String(), side.text); side.text != "" && offset >= 0 {
			side.state.GoTo(side.state.Buffer.Pos(offset))
			side.state.Select(offset, offset+len(side.text))
		}
	}
}

func (a *Workspace) mergeEditorDisabled(id commandID, target commandTarget) string {
	repo, r := a.Model.Active().Repository, a.repo()
	if id == openMergeEditor {
		if r.mergeDrafts[target.file.Path] != nil {
			return ""
		}
		conflict, ok := findMergeConflict(repo, target.file.Path)
		if a.ReadMergeDocument == nil || !repo.ResolvingConflicts() || !ok || conflict.Submodule {
			return "Select an unresolved text file"
		}
		if a.repositoryWrite != "" {
			return "Wait for the Git operation to finish"
		}
		return ""
	}
	d := r.mergeDrafts[r.mergeActive]
	if d == nil {
		return "Open a merge editor"
	}
	if id == closeMergeEditor {
		return ""
	}
	if a.repositoryWrite != "" || a.repositoryOperation != nil || a.repositoryRead != nil {
		return "Wait for the Git operation to finish"
	}
	if d.version != d.result.Buffer.Version() {
		d.parse()
	}
	switch id {
	case saveMergeResult:
		if a.OperateRepository == nil || repo.IndexVersion == "" {
			return "Refresh the repository before applying"
		}
		conflict, ok := findMergeConflict(repo, d.doc.Conflict.Path)
		if !ok || conflict != d.doc.Conflict || repo.Head != d.doc.Head || !slices.Equal(repo.MergeHeads, d.doc.MergeHeads) ||
			repo.Rebase != d.doc.Rebase || repo.Sequencer != d.doc.Sequencer {
			return "The merge state changed. Reload the editor before applying"
		}
		if len(d.blocks) != 0 || d.incomplete {
			return "Resolve the remaining conflict markers"
		}
	case previousConflict:
		if d.selected <= 0 {
			return "No previous conflict"
		}
	case nextConflict:
		if d.selected+1 >= len(d.blocks) {
			return "No next conflict"
		}
	case applyCurrentBlock, applyIncomingBlock:
		if len(d.blocks) == 0 || d.incomplete {
			return "Select a complete conflict block"
		}
	}
	return ""
}

func (a *Workspace) loadMergeEditor(path string) {
	stopRead(&a.repositoryFileRead)
	r := a.repo()
	if r.mergeDrafts[path] != nil {
		r.mergeActive = path
		r.mergeDrafts[path].focusResult = true
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	request := &readRequest{cancel: cancel}
	a.repositoryFileRead = request
	a.status = "Loading conflict · " + path
	a.ReadMergeDocument(ctx, a.Model.Active().Repository, path, func(doc domain.MergeDocument, err error) {
		if a.repositoryFileRead != request {
			return
		}
		stopRead(&a.repositoryFileRead)
		if err != nil {
			a.record("Could not open merge editor · " + err.Error())
			return
		}
		r := a.repo()
		if r.mergeDrafts == nil {
			r.mergeDrafts = make(map[string]*mergeEditorState)
		}
		r.mergeDrafts[path] = newMergeEditor(doc)
		r.mergeActive, r.preview = path, nil
		a.status = "Resolving " + path
	})
}

func (a *Workspace) runMergeEditor(id commandID) {
	r := a.repo()
	d := r.mergeDrafts[r.mergeActive]
	switch id {
	case closeMergeEditor:
		r.mergeActive = ""
		if !d.result.Buffer.Modified() {
			delete(r.mergeDrafts, d.doc.Conflict.Path)
		}
	case previousConflict:
		d.selected--
		d.showBlock()
	case nextConflict:
		d.selected++
		d.showBlock()
	case applyCurrentBlock, applyIncomingBlock:
		block := d.blocks[d.selected]
		text := block.Current
		if id == applyIncomingBlock {
			text = block.Incoming
		}
		d.result.Select(block.Start, block.End)
		d.result.Insert(text)
		d.parse()
		d.showBlock()
	case saveMergeResult:
		a.saveMergeDraft(d)
	}
}

func (a *Workspace) saveMergeDraft(d *mergeEditorState) {
	repo := a.Model.Active().Repository
	path := d.doc.Conflict.Path
	text := d.result.Buffer.String()
	if d.result.Buffer.LineEnding() == "\r\n" {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	op := domain.RepositoryOperation{Kind: domain.SaveMergeResult, Source: path, Result: text,
		WorktreeVersion: d.doc.WorktreeVersion, Conflict: d.doc.Conflict}
	ctx, cancel := context.WithCancel(context.Background())
	a.beginRepositoryWrite(repo.Root, "Applying merge result…")
	a.OperateRepository(ctx, repo, op, func(updated domain.Repository, err, refreshErr error) {
		cancel()
		message := "Staged merge result · " + path
		if err != nil {
			message = "Could not apply merge result · " + err.Error()
		} else {
			for i, session := range a.Model.Sessions {
				if session.Repository.Root == repo.Root {
					r := &a.repos[i]
					delete(r.mergeDrafts, path)
					if r.mergeActive == path {
						r.mergeActive = ""
					}
					break
				}
			}
		}
		a.finishRepositoryWrite(repo.Root, updated, refreshErr, message)
	})
}

func (a *Workspace) mergeEditor(c *ui.Context, p tokens, d *mergeEditorState) {
	busy := a.repositoryWrite != ""
	d.result.ReadOnly = busy
	button := func(id commandID, label string) {
		reason := a.disabledFor(id, commandTarget{})
		name := commandLabel(id)
		if d.doc.Rebase.Active() || d.doc.Sequencer.Active() {
			left, right := conflictPaneLabels(d.doc.Rebase, d.doc.Sequencer)
			switch id {
			case applyCurrentBlock:
				name = "Apply " + left + " Block"
			case applyIncomingBlock:
				name = "Apply " + right + " Block"
			}
		}
		tip := reason
		if tip == "" {
			tip = name
		}
		ui.Button(c.Key(string(id)), label).Label(name).Tooltip(tip).
			Disabled(reason != "").OnClick(func() { a.run(id, commandTarget{}) })
	}
	pane := func(label string, state *editor.State, action commandID, glyph string) {
		ui.Column(c.Key(label)).Fill().Background(p.editor).Children(func() {
			ui.Row(c).FillWidth().Height(headerHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Background(p.chrome).Children(func() {
				ui.Text(c, label).Bold().Grow(1)
				if action != "" {
					button(action, glyph)
				}
			})
			divider(c, p)
			state.Font, state.TabSize = a.editorPreferences()
			view := editor.View(c, state).Label(label + " merge editor").FillWidth().Grow(1)
			if state == d.result && d.focusResult {
				view.Focus()
				d.focusResult = false
			}
		})
	}
	ui.Column(c.Key("merge:" + d.doc.Conflict.Path)).Fill().Background(p.editor).Children(func() {
		title := "Merge"
		if d.doc.Rebase.Active() {
			title = "Rebase"
		} else if d.doc.Sequencer.Active() {
			title = sequenceTitle(d.doc.Sequencer)
		}
		heading(c, p, title+" · "+d.doc.Conflict.Path, "")
		if d.version != d.result.Buffer.Version() {
			d.parse()
		}
		d.result.LineBackgrounds = d.result.LineBackgrounds[:0]
		for i, block := range d.blocks {
			alpha := float32(.10)
			if i == d.selected {
				alpha = .18
			}
			d.result.LineBackgrounds = append(d.result.LineBackgrounds, editor.LineBackground{
				First: d.result.Buffer.Pos(block.Start).Line,
				Last:  d.result.Buffer.Pos(block.End - 1).Line,
				Color: p.theme.Danger.Alpha(alpha),
			})
		}
		ui.Row(c).FillWidth().Padding(gap).Gap(gap).AlignItems(ui.Center).Wrap().Children(func() {
			button(previousConflict, "↑")
			button(nextConflict, "↓")
			label := "No conflict markers"
			if d.incomplete {
				label = "Incomplete conflict markers"
			} else if len(d.blocks) != 0 {
				label = fmt.Sprintf("Conflict %d of %d", d.selected+1, len(d.blocks))
			}
			ui.Text(c, label).TextColor(p.theme.TextMuted).Grow(1)
			button(saveMergeResult, commandLabel(saveMergeResult))
			ui.Button(c, "Reload").Disabled(busy).OnClick(func() { d.reload = true })
			button(closeMergeEditor, "×")
		})
		divider(c, p)
		left, right := conflictPaneLabels(d.doc.Rebase, d.doc.Sequencer)
		ui.SplitPercent(c, &d.leftSplit, func() {
			pane(left, d.current, applyCurrentBlock, "→")
		}, func() {
			ui.SplitPercent(c, &d.rightSplit, func() {
				pane("Result", d.result, "", "")
			}, func() {
				pane(right, d.incoming, applyIncomingBlock, "←")
			}).Fill()
		}).Grow(1).FillWidth()
	})
	if d.reload && ui.AlertDialog(c, &d.reload, "Reload merge result?", "Reload the file from disk and discard the editor draft.", "Cancel", "Reload File") == 1 {
		path := d.doc.Conflict.Path
		delete(a.repo().mergeDrafts, path)
		a.repo().mergeActive = ""
		a.run(openMergeEditor, commandTarget{file: domain.File{Path: path}})
	}
	for _, key := range []struct {
		mods    ui.Modifiers
		key     ui.Key
		command commandID
	}{
		{0, ui.KeyF7, nextConflict}, {ui.Shift, ui.KeyF7, previousConflict},
		{ui.Cmd, ui.KeyS, saveMergeResult}, {0, ui.KeyEscape, closeMergeEditor},
	} {
		if c.Shortcut(key.mods, key.key) {
			a.run(key.command, commandTarget{})
		}
	}
}
