package ui

import (
	"maps"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

const shortcutChordTimeout = 1500 * time.Millisecond

type shortcutChordState struct {
	first    application.Shortcut
	scope    application.KeymapScope
	deadline time.Time
}

func layoutKeymap(layout application.Layout) map[string]string {
	if layout.Keymap == nil {
		return nil
	}
	return layout.Keymap.Bindings
}

func layoutLocalKeymap(layout application.Layout) map[application.KeymapScope]map[string]string {
	if layout.Keymap == nil {
		return nil
	}
	return layout.Keymap.LocalBindings
}

func cloneLocalKeymap(source map[application.KeymapScope]map[string]string) map[application.KeymapScope]map[string]string {
	if source == nil {
		return nil
	}
	cloned := make(map[application.KeymapScope]map[string]string, len(source))
	for scope, bindings := range source {
		cloned[scope] = maps.Clone(bindings)
	}
	return cloned
}

func equalLocalKeymap(left, right map[application.KeymapScope]map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for scope, bindings := range left {
		if !maps.Equal(bindings, right[scope]) {
			return false
		}
	}
	return true
}

func nativeShortcut(shortcut application.Shortcut) (ui.Modifiers, ui.Key) {
	var mods ui.Modifiers
	if shortcut.Ctrl {
		mods |= ui.Ctrl
	}
	if shortcut.Alt {
		mods |= ui.Alt
	}
	if shortcut.Shift {
		mods |= ui.Shift
	}
	key := ui.KeyUnknown
	if len(shortcut.Key) == 1 && shortcut.Key[0] >= 'A' && shortcut.Key[0] <= 'Z' {
		letters := [...]ui.Key{ui.KeyA, ui.KeyB, ui.KeyC, ui.KeyD, ui.KeyE, ui.KeyF, ui.KeyG, ui.KeyH, ui.KeyI, ui.KeyJ, ui.KeyK, ui.KeyL, ui.KeyM, ui.KeyN, ui.KeyO, ui.KeyP, ui.KeyQ, ui.KeyR, ui.KeyS, ui.KeyT, ui.KeyU, ui.KeyV, ui.KeyW, ui.KeyX, ui.KeyY, ui.KeyZ}
		key = letters[shortcut.Key[0]-'A']
	} else if len(shortcut.Key) == 1 && shortcut.Key[0] >= '0' && shortcut.Key[0] <= '9' {
		digits := [...]ui.Key{ui.Key0, ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5, ui.Key6, ui.Key7, ui.Key8, ui.Key9}
		key = digits[shortcut.Key[0]-'0']
	} else if shortcut.Key == "Home" {
		key = ui.KeyHome
	} else if shortcut.Key == "End" {
		key = ui.KeyEnd
	} else if len(shortcut.Key) > 1 && shortcut.Key[0] == 'F' {
		n, _ := strconv.Atoi(shortcut.Key[1:])
		keys := [...]ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12}
		key = keys[n-1]
	}
	return mods, key
}

func (a *Workspace) nativeWorkspaceShortcut(id commandID) (ui.Modifiers, ui.Key, bool) {
	value, known := application.EffectiveShortcut(a.Layout.Keymap, string(id))
	if !known || value == "" {
		return 0, ui.KeyUnknown, known
	}
	sequence, err := application.ParseKeySequence(value)
	if err != nil || sequence.Chord() {
		return 0, ui.KeyUnknown, true
	}
	mods, key := nativeShortcut(sequence.First)
	return mods, key, true
}

func (a *Workspace) runWorkspaceShortcut(id commandID) {
	if id == closeTab {
		if a.repo().mergeActive != "" {
			id = closeMergeEditor
		} else if a.repo().preview != nil {
			id = closePreview
		}
	}
	a.run(id, commandTarget{})
}

func (a *Workspace) markShortcutScope(scope application.KeymapScope) {
	if scope == application.KeymapEditor || a.viewShortcutScope == application.KeymapGlobal ||
		a.viewShortcutScope == application.KeymapToolWindow && scope == application.KeymapList {
		a.viewShortcutScope = scope
	}
}

func (a *Workspace) workspaceShortcutValue(id commandID, scope application.KeymapScope, local bool) (string, bool) {
	if local {
		if scope == application.KeymapGlobal || a.Layout.Keymap == nil {
			return "", false
		}
		value, ok := a.Layout.Keymap.LocalBindings[scope][string(id)]
		return value, ok
	}
	return application.EffectiveShortcut(a.Layout.Keymap, string(id))
}

func (a *Workspace) shortcutCommandAvailable(id commandID) bool {
	return a.Model.Active().Repository.Root != "" || id != push && id != updateProject && id != newBranch
}

func (a *Workspace) completeShortcutChord(c *ui.Context, scope application.KeymapScope) bool {
	for _, local := range []bool{true, false} {
		for _, binding := range application.WorkspaceShortcuts() {
			id := commandID(binding.Command)
			if !a.shortcutCommandAvailable(id) {
				continue
			}
			value, known := a.workspaceShortcutValue(id, scope, local)
			if !known || value == "" {
				continue
			}
			sequence, err := application.ParseKeySequence(value)
			if err != nil || !sequence.Chord() || sequence.First != a.chord.first {
				continue
			}
			mods, key := nativeShortcut(sequence.Second)
			if key != ui.KeyUnknown && c.Shortcut(mods, key) {
				a.chord = shortcutChordState{}
				a.runWorkspaceShortcut(id)
				return true
			}
		}
	}
	return false
}

func (a *Workspace) dispatchWorkspaceShortcuts(c *ui.Context) {
	scope := a.viewShortcutScope
	a.shortcutScope = scope
	if a.chord.first.Key != "" {
		if a.chord.scope != scope || !c.Now().Before(a.chord.deadline) {
			a.chord = shortcutChordState{}
		} else {
			c.After(a.chord.deadline.Sub(c.Now()))
			if c.Shortcut(0, ui.KeyEscape) {
				a.chord = shortcutChordState{}
				c.Invalidate()
				return
			}
			if a.completeShortcutChord(c, scope) {
				return
			}
		}
	}
	// Text fields keep single-stroke shortcuts (as before chords existed) but
	// never start a chord, so the following keystroke reaches the text.
	textInput := c.FocusTakesText() && a.focusedEditor == nil && scope != application.KeymapEditor
	if textInput {
		a.chord = shortcutChordState{}
	}
	if a.chord.first.Key == "" {
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyR) {
			a.run(replaceInProject, commandTarget{})
			return
		}
		if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
			a.run(findInProject, commandTarget{})
			return
		}
	}
	for _, local := range []bool{true, false} {
		for _, chord := range []bool{true, false} {
			for _, binding := range application.WorkspaceShortcuts() {
				id := commandID(binding.Command)
				if !a.shortcutCommandAvailable(id) {
					continue
				}
				value, known := a.workspaceShortcutValue(id, scope, local)
				if !known || value == "" {
					continue
				}
				sequence, err := application.ParseKeySequence(value)
				if err != nil || sequence.Chord() != chord || chord && textInput {
					continue
				}
				mods, key := nativeShortcut(sequence.First)
				if key == ui.KeyUnknown || !c.Shortcut(mods, key) {
					continue
				}
				a.chord = shortcutChordState{}
				if chord {
					a.chord = shortcutChordState{first: sequence.First, scope: scope, deadline: c.Now().Add(shortcutChordTimeout)}
					c.After(shortcutChordTimeout)
					c.Invalidate()
					return
				}
				a.runWorkspaceShortcut(id)
				return
			}
		}
	}
	_, globalCloseOverride := layoutKeymap(a.Layout)[string(closeTab)]
	_, localCloseOverride := layoutLocalKeymap(a.Layout)[scope][string(closeTab)]
	if !globalCloseOverride && !localCloseOverride && c.Shortcut(ui.Ctrl, ui.KeyF4) {
		a.runWorkspaceShortcut(closeTab)
	}
}

func (a *Workspace) normalizedShortcutDraft(globals map[string]string, locals map[application.KeymapScope]map[string]string) bool {
	normalized, err := application.NormalizeKeymapSettings(&application.KeymapSettings{Bindings: globals, LocalBindings: locals})
	if err != nil {
		a.settings.keymapMessage = err.Error()
		return false
	}
	a.settings.keymap = nil
	a.settings.localKeymap = nil
	if normalized != nil {
		a.settings.keymap = normalized.Bindings
		a.settings.localKeymap = normalized.LocalBindings
	}
	a.settings.keymapMessage = ""
	a.settings.keymapCommand = ""
	return true
}

func (a *Workspace) assignShortcut(value string) {
	s := &a.settings
	bindings := application.WorkspaceShortcuts()
	if s.keymapRow < 0 || s.keymapRow >= len(bindings) {
		return
	}
	id := bindings[s.keymapRow].Command
	globals := maps.Clone(s.keymap)
	locals := cloneLocalKeymap(s.localKeymap)
	if s.keymapScope == application.KeymapGlobal {
		if globals == nil {
			globals = map[string]string{}
		}
		globals[id] = value
	} else {
		if locals == nil {
			locals = map[application.KeymapScope]map[string]string{}
		}
		if locals[s.keymapScope] == nil {
			locals[s.keymapScope] = map[string]string{}
		}
		locals[s.keymapScope][id] = value
	}
	a.normalizedShortcutDraft(globals, locals)
}

func (a *Workspace) resetSelectedShortcut() {
	s := &a.settings
	bindings := application.WorkspaceShortcuts()
	if s.keymapRow < 0 || s.keymapRow >= len(bindings) {
		return
	}
	if s.keymapScope == application.KeymapGlobal {
		a.assignShortcut(bindings[s.keymapRow].Shortcut)
		return
	}
	locals := cloneLocalKeymap(s.localKeymap)
	delete(locals[s.keymapScope], bindings[s.keymapRow].Command)
	if len(locals[s.keymapScope]) == 0 {
		delete(locals, s.keymapScope)
	}
	a.normalizedShortcutDraft(maps.Clone(s.keymap), locals)
}

func keymapScopeLabel(scope application.KeymapScope) string {
	switch scope {
	case application.KeymapEditor:
		return "Editor"
	case application.KeymapList:
		return "List"
	case application.KeymapToolWindow:
		return "Tool Window"
	default:
		return "Global"
	}
}

func keymapScopeFromLabel(label string) application.KeymapScope {
	switch label {
	case "Editor":
		return application.KeymapEditor
	case "List":
		return application.KeymapList
	case "Tool Window":
		return application.KeymapToolWindow
	default:
		return application.KeymapGlobal
	}
}

func (a *Workspace) keymapSettings(c *ui.Context, p tokens) {
	s := &a.settings
	bindings := application.WorkspaceShortcuts()
	s.keymapRow = max(0, min(s.keymapRow, len(bindings)-1))
	ui.Text(c, "Customize workspace shortcuts").Bold()
	ui.Text(c, "Use one or two strokes, such as Ctrl+Alt+J Ctrl+Alt+C. Local bindings override globals in their focus scope; Esc cancels a pending chord.").FillWidth().TextColor(p.theme.TextMuted)
	scopeLabel := keymapScopeLabel(s.keymapScope)
	ui.Select(c, &scopeLabel, []string{"Global", "Editor", "List", "Tool Window"}).Label("Shortcut scope")
	s.keymapScope = keymapScopeFromLabel(scopeLabel)
	s.keymapList.Selected = &s.keymapRow
	s.keymapList.Key = func(i int) any { return bindings[i].Command }
	s.keymapList.Label = func(i int) string { return commandLabel(commandID(bindings[i].Command)) }
	list := ui.List(c, &s.keymapList, len(bindings), func(i int) {
		binding := bindings[i]
		label := commandLabel(commandID(binding.Command))
		settings := &application.KeymapSettings{Bindings: s.keymap, LocalBindings: s.localKeymap}
		value, _ := application.EffectiveShortcutForScope(settings, s.keymapScope, binding.Command)
		if value == "" {
			value = "Disabled"
		}
		row := ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Label("Shortcut action " + label)
		if i%2 == 1 && i != s.keymapRow {
			row.Background(p.stripe)
		}
		row.Children(func() {
			ui.Text(c, label).Grow(1).SingleLine().Ellipsis("…")
			ui.Text(c, value).Width(230).SingleLine()
		})
	}).Height(float32(len(bindings)) * rowHeight).FillWidth().Label("Workspace shortcuts")
	if list.Changed() {
		s.keymapCommand = ""
		s.keymapMessage = ""
	}
	selected := bindings[s.keymapRow]
	selectionKey := string(s.keymapScope) + ":" + selected.Command
	if s.keymapCommand != selectionKey {
		s.keymapCommand = selectionKey
		s.keymapBinding, _ = application.EffectiveShortcutForScope(&application.KeymapSettings{Bindings: s.keymap, LocalBindings: s.localKeymap}, s.keymapScope, selected.Command)
	}
	ui.Text(c, "Binding for "+commandLabel(commandID(selected.Command))+" · "+keymapScopeLabel(s.keymapScope))
	ui.TextInput(c, &s.keymapBinding).Label("Shortcut binding").Placeholder("Ctrl+Alt+J Ctrl+Alt+C").FillWidth()
	ui.Row(c).FillWidth().Gap(gap).Children(func() {
		ui.Button(c, "Assign").Label("Assign Shortcut").OnClick(func() { a.assignShortcut(s.keymapBinding) })
		ui.Button(c, "Disable").Label("Disable Shortcut").OnClick(func() { a.assignShortcut("") })
		ui.Button(c, "Reset Selected").Label("Reset Selected Shortcut").OnClick(a.resetSelectedShortcut)
		ui.Button(c, "Restore Defaults").Label("Restore Default Shortcuts").OnClick(func() {
			s.keymap = nil
			s.localKeymap = nil
			s.keymapMessage = ""
			s.keymapCommand = ""
		})
	})
	if s.keymapMessage != "" {
		ui.Text(c, s.keymapMessage).FillWidth().TextColor(p.theme.Danger)
	}
	ui.Checkbox(c, &s.shortcutReference, "Show default shortcut reference")
	if s.shortcutReference {
		shortcutHelp(c, p)
	}
}
