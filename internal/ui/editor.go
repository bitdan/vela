package ui

import (
	"maps"
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

func (a *Workspace) activate(id string) bool {
	return a.repo().activate(id)
}

func (r *repoView) activate(id string) bool {
	for i, existing := range r.tabs {
		if existing.id == id {
			r.active = i
			return true
		}
	}
	return false
}

func (a *Workspace) openDiff(source string, f domain.File) {
	r := a.repo()
	r.preview = nil
	if a.activate("diff:" + source + ":" + f.Path) {
		current := &r.tabs[r.active]
		if a.Model.Active().Repository.Root != "" && current.file != f {
			entry := diffTab(source, f)
			entry.pinned = current.pinned
			entry.diff.SetScrollY(current.diff.ScrollY())
			*current = entry
		}
		return
	}
	entry := diffTab(source, f)
	r.active = len(r.tabs)
	r.tabs = append(r.tabs, entry)
}

func diffTab(source string, f domain.File) tab {
	id := "diff:" + source + ":" + f.Path
	before, after := editor.NewBuffer(f.Old), editor.NewBuffer(f.New)
	lang := editor.LanguageFor(f.Path)
	before.SetLanguage(lang)
	after.SetLanguage(lang)
	context := "Diff · " + source
	if left, right, ok := domain.ParseComparisonSource(source); ok {
		label := "working snapshot"
		if right != "" {
			label = right[:10]
		}
		context = "Compare · " + left[:10] + " → " + label
	}
	return tab{id: id, title: path.Base(f.Path), path: f.Path, source: source, context: context, file: f,
		diff: &editor.DiffState{Old: before, New: after, Layout: editor.DiffSplit}}
}

func (a *Workspace) openCode(source string, f domain.File) {
	a.repo().preview = nil
	id := "code:" + source + ":" + f.Path
	if a.activate(id) {
		current := &a.repo().tabs[a.repo().active]
		if a.Model.Active().Repository.Root != "" && current.file != f {
			if current.code.Buffer.Modified() {
				return
			}
			buffer := current.code.Buffer
			cursor, scroll := current.code.Cursor(), current.code.ScrollY()
			buffer.Reset(f.New)
			buffer.SetLanguage(editor.LanguageFor(f.Path))
			if strings.HasPrefix(source, "blame:") {
				buffer.SetLanguage(nil)
			}
			current.code.ReadOnly = a.sourceReadOnly(source, f)
			cursor = min(cursor, len(f.New))
			current.code.Select(cursor, cursor)
			current.code.SetScrollY(scroll)
			current.file = f
			if current.document != nil {
				current.document.draftBase = nil
			}
			applyEditorFileType(current)
			if split := a.repo().split; split != nil && split.code != nil && split.code.Buffer == buffer {
				split.file = f
				split.code.ReadOnly = current.code.ReadOnly
			}
		}
		a.recoverPendingEditorDraft(current)
		return
	}
	buffer := editor.NewBuffer(f.New)
	buffer.SetLanguage(editor.LanguageFor(f.Path))
	if strings.HasPrefix(source, "blame:") {
		buffer.SetLanguage(nil)
	}
	r := a.repo()
	r.active = len(r.tabs)
	r.tabs = append(r.tabs, tab{id: id, title: path.Base(f.Path), path: f.Path, source: source, context: "Editor · " + source, file: f,
		document: &editorDocument{},
		code:     &editor.State{Buffer: buffer, ReadOnly: a.sourceReadOnly(source, f)}})
	if strings.HasPrefix(source, "blame:") {
		r.tabs[r.active].title = "Blame · " + path.Base(f.Path)
		r.tabs[r.active].context = "Blame · " + strings.TrimPrefix(source, "blame:")
	}
	a.recoverPendingEditorDraft(&r.tabs[r.active])
}

func (a *Workspace) splitTab(id string, vertical, move bool) {
	a.cancelSessionNavigation()
	r := a.repo()
	index := r.tabIndex(id)
	entry := r.tabs[index]
	if move {
		if r.active == index && len(r.tabs) > 1 {
			r.active = index + 1
			if r.active == len(r.tabs) {
				r.active = index - 1
			}
		}
	} else if entry.code != nil {
		entry.code = &editor.State{Buffer: entry.code.Buffer, ReadOnly: entry.code.ReadOnly}
	} else {
		entry = diffTab(entry.source, entry.file)
	}
	r.split, r.splitVertical, r.splitMoved, r.splitSize = &entry, vertical, move, 50
	r.preview = nil
}

func (a *Workspace) hasUnsavedTabs() bool {
	for _, repo := range a.repos {
		if repo.split != nil && repo.split.modified() {
			return true
		}
		for _, tab := range repo.tabs {
			if tab.code != nil && tab.code.Buffer.Modified() {
				return true
			}
		}
	}
	return a.hasUnsavedMergeDrafts()
}

func (a *Workspace) hasUnsavedMergeDrafts() bool {
	for _, repo := range a.repos {
		for _, draft := range repo.mergeDrafts {
			if draft.result.Buffer.Modified() {
				return true
			}
		}
	}
	return false
}

// RequestClose returns false while the workspace needs confirmation to exit.
func (a *Workspace) RequestClose() bool {
	if a.sessionSave != nil {
		return false
	}
	if s := a.repositoryOperation; s != nil {
		stopRead(&s.shelfHunkRead)
	}
	if a.Tasks.ActiveCount() > 0 {
		a.Tasks.CancelAll()
		a.status = "Waiting for task operations and refresh callbacks; close again after they finish"
		a.tasksOpen = true
		a.Layout.HideGit = false
		return false
	}
	a.stopBackgroundTasks()
	if a.backgroundInFlight > 0 {
		a.status = "Waiting for background Git tasks to stop; close again after they finish"
		return false
	}
	a.closeProjectFileDialog()
	a.closeBranchComparisonDialog()
	a.stopComparisonTabReads()
	a.stopHistoryComparison()
	a.closeRevisionBrowser()
	a.changelistDialog = nil
	a.closeProjectSearchDialog()
	a.fileTypeDialog = nil
	stopRead(&a.logRead)
	if a.repositoryWrite != "" {
		a.status = "Wait for the Git operation to finish"
		return false
	}
	stopRead(&a.repositoryRead)
	stopRead(&a.sessionRestore)
	stopRead(&a.repositoryFileRead)
	a.stopSourceRefreshes()
	a.closeFileHistoryDialog()
	a.stopPatchExports()
	a.stopSourceDecision()
	if s := a.patchDialog; s != nil && s.cancel != nil {
		s.cancel()
		a.patchDialog = nil
	}
	if s := a.repositoryInit; s != nil && s.cancel != nil {
		s.cancel()
		return false
	}
	if a.exitApproved || !a.hasUnsavedTabs() {
		return true
	}
	a.confirmExit = true
	return false
}

func (a *Workspace) editor(c *ui.Context, p tokens) {
	r := a.repo()
	if draft := r.mergeDrafts[r.mergeActive]; draft != nil {
		a.mergeEditor(c, p, draft)
		return
	}
	if len(r.tabs) == 0 && r.preview == nil && len(r.mergeDrafts) == 0 {
		a.emptyEditor(c, p)
		return
	}
	ui.Column(c).Fill().Background(p.editor).Children(func() {
		ui.Row(c).FillWidth().Height(headerHeight+4).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Background(p.chrome).Clip().Children(func() {
			ui.ScrollHorizontal(c).Grow(1).FillHeight().Children(func() {
				ui.Row(c).FillHeight().Gap(gap).AlignItems(ui.Center).Children(func() {
					if len(r.mergeDrafts) != 0 {
						ui.Button(c, "Merge Drafts").Menu(func(m *ui.Menu) {
							for _, name := range slices.Sorted(maps.Keys(r.mergeDrafts)) {
								a.menuNamed(m, openMergeEditor, commandTarget{file: domain.File{Path: name}}, name, "")
							}
						})
					}
					for i, entry := range r.tabs {
						if r.split != nil && r.splitMoved && r.split.id == entry.id {
							continue
						}
						label := entry.title
						if entry.code != nil && entry.code.Buffer.Modified() {
							label += " •"
						}
						if entry.pinned {
							label = "◆ " + label
						}
						tabRow := ui.Row(c.Key(entry.id)).Height(controlHeight).AlignItems(ui.Center).Radius(4)
						if i == r.active && r.preview == nil {
							toolUnderline(tabRow, p)
						}
						tabRow.Children(func() {
							ui.ButtonBase(c).Height(controlHeight).Padding(0, gap).Center().Label(entry.title).Tooltip(entry.id).
								ContextMenu(func(m *ui.Menu) { a.tabMenu(m, entry, false) }).
								Children(func() { ui.Text(c, label).SingleLine() }).
								OnClick(func() { a.cancelSessionNavigation(); r.preview = nil; r.active = i })
							target := commandTarget{tabID: entry.id}
							reason := a.disabledFor(closeTab, target)
							tip := "Close"
							if keys, _ := application.EffectiveShortcut(a.Layout.Keymap, string(closeTab)); keys != "" {
								tip += " (" + keys + ")"
							}
							if reason != "" {
								tip = reason
							}
							ui.ButtonBase(c).Size(20, 20).Center().Label("Close " + entry.title).Tooltip(tip).
								Disabled(reason != "").Children(func() { ui.Text(c, "×") }).
								OnClick(func() { a.run(closeTab, target) })
						})
					}
					if r.preview != nil {
						a.previewTab(c, p, *r.preview)
					}
				})
			})
		})
		divider(c, p)
		if r.preview != nil {
			a.diffToolbar(c, p, r.preview)
			r.preview.diff.Font, r.preview.diff.TabSize = a.editorPreferences()
			editor.DiffView(c.Key("preview:"+r.preview.id), r.preview.diff).Grow(1).FillWidth()
			return
		}
		if len(r.tabs) == 0 {
			a.emptyEditor(c, p)
			return
		}
		entry := r.tabs[r.active]
		if r.split == nil {
			a.tabContent(c, p, entry)
			return
		}
		first := func() {
			ui.Column(c).Fill().Children(func() {
				if r.splitMoved && r.split.id == entry.id {
					ui.Text(c, "Editor Workspace").Padding(inset)
					return
				}
				a.tabContent(c, p, entry)
			})
		}
		second := func() {
			ui.Column(c).Fill().Children(func() {
				ui.Row(c).FillWidth().Padding(0, gap).Gap(gap).Children(func() {
					ui.Button(c, r.split.title).Label("Split tab").ContextMenu(func(m *ui.Menu) { a.tabMenu(m, *r.split, false) })
					a.panelAction(c, closeSplit, "×")
				})
				a.tabContent(c, p, *r.split)
			})
		}
		split := ui.SplitPercent
		if r.splitVertical {
			split = ui.SplitVerticalPercent
		}
		split(c, &r.splitSize, first, second).Grow(1).FillWidth()
	})
}

func (a *Workspace) emptyEditor(c *ui.Context, p tokens) {
	panel := ui.Column(c).Fill().Center().Background(p.panel).Label("Empty editor").TabStop(false)
	if a.repo().emptyEditorFocus {
		panel.Focus()
		a.repo().emptyEditorFocus = false
	}
	panel.Children(func() {
		ui.Column(c).AlignItems(ui.Start).Children(func() {
			for _, item := range []struct {
				id   commandID
				keys string
			}{
				{showSearch, "Shift Shift"},
				{goToFile, "Ctrl+Shift+N"},
				{showRecentFiles, "Ctrl+E"},
				{focusProject, "Alt+Home"},
			} {
				label := commandLabel(item.id)
				if keys, known := application.EffectiveShortcut(a.Layout.Keymap, string(item.id)); known {
					item.keys = keys
				}
				ui.ButtonBase(c.Key(string(item.id))).Height(headerHeight+4).Padding(0, gap).
					Row().Gap(gap).AlignItems(ui.Center).Label(label).Children(func() {
					ui.Text(c, label).FontSize(p.textSize(emptyEditorText)).TextColor(p.theme.TextMuted)
					ui.Text(c, item.keys).FontSize(p.textSize(emptyEditorText))
				}).OnClick(func() { a.run(item.id, commandTarget{}) })
			}
		})
	})
}

func (a *Workspace) tabContent(c *ui.Context, p tokens, entry tab) {
	if entry.comparison != nil {
		a.twoWayComparisonView(c, p, entry.comparison)
		return
	}
	heading(c, p, entry.path, entry.context)
	if entry.code != nil {
		a.prepareEditorMetadata(&entry, p)
		if !entry.code.ReadOnly && a.Model.Active().Repository.Root != "" {
			ui.Row(c).FillWidth().Height(headerHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Children(func() {
				reason := a.disabledFor(saveSourceFile, commandTarget{tabID: entry.id})
				ui.Button(c, "Save").Label("Save Source File").Height(controlHeight).Disabled(reason != "").Tooltip(reason).OnClick(func() { a.run(saveSourceFile, commandTarget{tabID: entry.id}) })
				ui.Text(c, "Ctrl+S").TextColor(p.theme.TextMuted).FontSize(p.textSize(smallText))
			})
		}
		entry.code.Font, entry.code.TabSize = a.editorPreferences()
		entry.code.ContextMenu = func(m *ui.Menu) {
			target := commandTarget{tabID: entry.id, editorState: entry.code}
			a.editorBookmarkMenu(m, target)
			a.menu(m, fileType, target)
			m.Separator()
			if a.Model.Active().Repository.Root != "" {
				a.menu(m, saveSourceFile, commandTarget{tabID: entry.id})
				a.menu(m, reloadProjectFile, commandTarget{tabID: entry.id})
			}
			m.Submenu("Git", func(g *ui.Menu) { a.editorGitMenu(g, commandTarget{source: entry.source, file: entry.file}) })
		}
		view := editor.View(c.Key(entry.id), entry.code).Label("Source editor").Grow(1).FillWidth()
		if a.searchFocus == entry.code {
			view.Focus()
			a.searchFocus = nil
		}
		if view.FocusWithin() {
			a.focusedEditor = entry.code
		}
		if view.Shortcut(ui.Cmd, ui.KeyS) {
			a.run(saveSourceFile, commandTarget{tabID: entry.id})
		}
		target := commandTarget{tabID: entry.id, editorState: entry.code}
		if view.Shortcut(0, ui.KeyF11) {
			a.run(toggleBookmark, target)
		}
		if view.Shortcut(0, ui.KeyF2) {
			a.run(nextBookmark, target)
		}
		if view.Shortcut(ui.Shift, ui.KeyF2) {
			a.run(previousBookmark, target)
		}
	} else {
		entry.diff.Font, entry.diff.TabSize = a.editorPreferences()
		editor.DiffView(c.Key(entry.id), entry.diff).Grow(1).FillWidth()
	}
}
