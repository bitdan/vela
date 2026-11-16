package ui

import (
	"maps"
	"strconv"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func layoutKeymap(layout application.Layout) map[string]string {
	if layout.Keymap == nil {
		return nil
	}
	return layout.Keymap.Bindings
}

func (a *Workspace) nativeWorkspaceShortcut(id commandID) (ui.Modifiers, ui.Key, bool) {
	value, known := application.EffectiveShortcut(a.Layout.Keymap, string(id))
	if !known || value == "" {
		return 0, ui.KeyUnknown, known
	}
	shortcut, err := application.ParseShortcut(value)
	if err != nil {
		return 0, ui.KeyUnknown, true
	}
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
	} else if shortcut.Key[0] == 'F' {
		n, _ := strconv.Atoi(shortcut.Key[1:])
		keys := [...]ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12}
		key = keys[n-1]
	}
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

func (a *Workspace) dispatchWorkspaceShortcuts(c *ui.Context) {
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyR) {
		a.run(replaceInProject, commandTarget{})
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
		a.run(findInProject, commandTarget{})
	}
	for _, binding := range application.WorkspaceShortcuts() {
		id := commandID(binding.Command)
		if a.Model.Active().Repository.Root == "" && (id == push || id == updateProject || id == newBranch) {
			continue
		}
		mods, key, _ := a.nativeWorkspaceShortcut(id)
		if key != ui.KeyUnknown && c.Shortcut(mods, key) {
			a.runWorkspaceShortcut(id)
		}
	}
	if _, overridden := layoutKeymap(a.Layout)[string(closeTab)]; !overridden && c.Shortcut(ui.Ctrl, ui.KeyF4) {
		a.runWorkspaceShortcut(closeTab)
	}
}

func (a *Workspace) assignShortcut(value string) {
	s := &a.settings
	bindings := application.WorkspaceShortcuts()
	if s.keymapRow < 0 || s.keymapRow >= len(bindings) {
		return
	}
	candidate := maps.Clone(s.keymap)
	if candidate == nil {
		candidate = map[string]string{}
	}
	candidate[bindings[s.keymapRow].Command] = value
	normalized, err := application.NormalizeKeymap(candidate)
	if err != nil {
		s.keymapMessage = err.Error()
		return
	}
	s.keymap = normalized
	s.keymapMessage = ""
	s.keymapCommand = ""
}

func (a *Workspace) keymapSettings(c *ui.Context, p tokens) {
	s := &a.settings
	bindings := application.WorkspaceShortcuts()
	s.keymapRow = max(0, min(s.keymapRow, len(bindings)-1))
	ui.Text(c, "Customize workspace shortcuts").Bold()
	ui.Text(c, "Ctrl or Alt with optional Shift. Editor and panel shortcuts take precedence; protected and duplicate bindings are rejected.").FillWidth().TextColor(p.theme.TextMuted)
	s.keymapList.Selected = &s.keymapRow
	s.keymapList.Key = func(i int) any { return bindings[i].Command }
	s.keymapList.Label = func(i int) string { return commandLabel(commandID(bindings[i].Command)) }
	list := ui.List(c, &s.keymapList, len(bindings), func(i int) {
		binding := bindings[i]
		label := commandLabel(commandID(binding.Command))
		value, _ := application.EffectiveShortcut(&application.KeymapSettings{Bindings: s.keymap}, binding.Command)
		if value == "" {
			value = "Disabled"
		}
		row := ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).Label("Shortcut action " + label)
		if i%2 == 1 && i != s.keymapRow {
			row.Background(p.stripe)
		}
		row.Children(func() {
			ui.Text(c, label).Grow(1).SingleLine().Ellipsis("…")
			ui.Text(c, value).Width(150).SingleLine()
		})
	}).Height(float32(len(bindings)) * rowHeight).FillWidth().Label("Workspace shortcuts")
	if list.Changed() {
		s.keymapCommand = ""
		s.keymapMessage = ""
	}
	selected := bindings[s.keymapRow]
	if s.keymapCommand != selected.Command {
		s.keymapCommand = selected.Command
		s.keymapBinding, _ = application.EffectiveShortcut(&application.KeymapSettings{Bindings: s.keymap}, selected.Command)
	}
	ui.Text(c, "Binding for "+commandLabel(commandID(selected.Command)))
	ui.TextInput(c, &s.keymapBinding).Label("Shortcut binding").Placeholder("Ctrl+Alt+G").FillWidth()
	ui.Row(c).FillWidth().Gap(gap).Children(func() {
		ui.Button(c, "Assign").Label("Assign Shortcut").OnClick(func() { a.assignShortcut(s.keymapBinding) })
		ui.Button(c, "Disable").Label("Disable Shortcut").OnClick(func() { a.assignShortcut("") })
		ui.Button(c, "Reset Selected").Label("Reset Selected Shortcut").OnClick(func() { a.assignShortcut(selected.Shortcut) })
		ui.Button(c, "Restore Defaults").Label("Restore Default Shortcuts").OnClick(func() { s.keymap = nil; s.keymapMessage = ""; s.keymapCommand = "" })
	})
	if s.keymapMessage != "" {
		ui.Text(c, s.keymapMessage).FillWidth().TextColor(p.theme.Danger)
	}
	ui.Checkbox(c, &s.shortcutReference, "Show default shortcut reference")
	if s.shortcutReference {
		shortcutHelp(c, p)
	}
}
