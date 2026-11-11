package ui

import (
	"slices"

	"github.com/egoist/mygo/ui"
)

const (
	closeOthers     commandID = "editor.close-other-tabs"
	closeAllTabs    commandID = "editor.close-all-tabs"
	closeUnmodified commandID = "editor.close-unmodified-tabs"
	closeLeft       commandID = "editor.close-tabs-left"
	closeRight      commandID = "editor.close-tabs-right"
	closeReadOnly   commandID = "editor.close-read-only-tabs"
	pinTab          commandID = "editor.pin-tab"
	reopenTab       commandID = "editor.reopen-tab"
	splitRight      commandID = "editor.split-right"
	splitDown       commandID = "editor.split-down"
	moveRight       commandID = "editor.move-right"
	moveDown        commandID = "editor.move-down"
	closeSplit      commandID = "editor.close-split"
	newTabWindow    commandID = "editor.open-tab-window"
	configureTabs   commandID = "editor.configure-tabs"
	tabBookmarks    commandID = "editor.bookmarks"
	fileType        commandID = "editor.override-file-type"
	openInExplorer  commandID = "editor.open-in-explorer"
	localHistory    commandID = "editor.local-history"
	renameFile      commandID = "file.rename"
)

func (t tab) modified() bool { return t.code != nil && t.code.Buffer.Modified() }
func (t tab) readOnly() bool { return t.code == nil || t.code.ReadOnly }

func (r *repoView) tabIndex(id string) int {
	if len(r.tabs) == 0 {
		return -1
	}
	if id == "" {
		return r.active
	}
	return slices.IndexFunc(r.tabs, func(entry tab) bool { return entry.id == id })
}

func (r *repoView) tabTargetIndex(t commandTarget) int {
	if t.preview {
		if r.preview != nil && r.preview.id == t.tabID {
			return len(r.tabs)
		}
		return -1
	}
	return r.tabIndex(t.tabID)
}

func (a *Workspace) promotePreview() string {
	r := a.repo()
	entry := *r.preview
	r.preview = nil
	if !a.activate(entry.id) {
		r.active = len(r.tabs)
		r.tabs = append(r.tabs, entry)
	} else {
		r.tabs[r.active].diff = entry.diff
	}
	return entry.id
}

func shouldClose(id commandID, index, target int, entry tab) bool {
	if entry.modified() {
		return false
	}
	if id == closeTab {
		return index == target
	}
	if entry.pinned {
		return false
	}
	switch id {
	case closeOthers:
		return index != target
	case closeLeft:
		return index < target
	case closeRight:
		return index > target
	case closeReadOnly:
		return entry.readOnly()
	case closeAllTabs, closeUnmodified:
		return true
	}
	return false
}

func (a *Workspace) tabDisabled(id commandID, t commandTarget) string {
	r := a.repo()
	if id == reopenTab {
		if len(r.closed) == 0 {
			return "No recently closed tabs"
		}
		return ""
	}
	index := r.tabTargetIndex(t)
	if index < 0 {
		return "Select an editor tab"
	}
	switch id {
	case pinTab, splitRight, splitDown, moveRight, moveDown:
		return ""
	case closeTab:
		if !t.preview && r.tabs[index].modified() {
			return "This tab has unsaved changes"
		}
		return ""
	}
	for i, entry := range r.tabs {
		if shouldClose(id, i, index, entry) {
			return ""
		}
	}
	if r.preview != nil && shouldClose(id, len(r.tabs), index, *r.preview) {
		return ""
	}
	return "No matching tabs that can be closed"
}

func (a *Workspace) closeTabs(id commandID, target commandTarget) {
	r := a.repo()
	index := r.tabTargetIndex(target)
	active := ""
	if len(r.tabs) > 0 {
		active = r.tabs[r.active].id
	}
	if r.preview != nil && shouldClose(id, len(r.tabs), index, *r.preview) {
		r.preview = nil
	}
	kept := r.tabs[:0]
	next := 0
	for i, entry := range r.tabs {
		if shouldClose(id, i, index, entry) {
			r.closed = append(r.closed, entry)
			if r.split != nil && r.split.id == entry.id {
				r.split = nil
			}
		} else {
			if i < r.active {
				next++
			}
			kept = append(kept, entry)
		}
	}
	clear(r.tabs[len(kept):])
	r.tabs = kept
	if len(r.tabs) == 0 && r.preview == nil {
		r.emptyEditorFocus = true
	}
	r.active = max(0, min(next, len(kept)-1))
	a.activate(active)
	if len(r.closed) > 20 {
		copy(r.closed, r.closed[len(r.closed)-20:])
		clear(r.closed[20:])
		r.closed = r.closed[:20]
	}
}

func (a *Workspace) reopenClosedTab() {
	a.reopenClosedTabAt(len(a.repo().closed) - 1)
}

func (a *Workspace) reopenClosedTabAt(index int) {
	r := a.repo()
	entry := r.closed[index]
	r.closed = slices.Delete(r.closed, index, index+1)
	r.preview = nil
	if !a.activate(entry.id) {
		r.active = len(r.tabs)
		r.tabs = append(r.tabs, entry)
	}
}

func (a *Workspace) tabMenu(m *ui.Menu, entry tab, preview bool) {
	t := commandTarget{source: entry.source, file: entry.file, tabID: entry.id, preview: preview}
	a.menuNamed(m, closeTab, t, "Close", "")
	a.menuGroup(m, t, closeOthers, closeAllTabs, closeUnmodified, closeLeft, closeRight, closeReadOnly)
	m.Separator()
	a.menu(m, copyPath, t)
	m.Separator()
	a.menuGroup(m, t, splitRight, moveRight, splitDown, moveDown)
	m.Separator()
	label := "Pin Tab"
	if entry.pinned {
		label = "Unpin Tab"
	}
	a.menuNamed(m, pinTab, t, label, "")
	a.menuGroup(m, t, newTabWindow, configureTabs)
	m.Separator()
	a.menu(m, reopenTab, t)
	m.Separator()
	a.menuGroup(m, t, tabBookmarks, fileType, openInExplorer, localHistory)
	m.Submenu("Git", func(g *ui.Menu) { a.editorGitMenu(g, t) })
	a.menu(m, renameFile, t)
}
