package ui

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	native "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
)

func TestWorkspaceShortcutSettingsApplyCancelReset(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			open := func() { a.run(showSettings, commandTarget{}); tt.Frame(); click("Settings Shortcuts") }
			assign := func(value string) {
				click("Shortcut binding")
				tt.Key(native.Ctrl, native.KeyA)
				tt.Type(value)
				click("Assign Shortcut")
			}
			open()
			assign("Ctrl+Alt+G")
			exportSessionScreenshot(t, tt, "keymap-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Apply")
			click("Cancel")
			if a.Layout.Keymap == nil || a.Layout.Keymap.Bindings[string(toggleGit)] != "Ctrl+Alt+G" {
				t.Fatal("shortcut setting did not publish")
			}
			tt.Key(native.Alt, native.Key0)
			if a.Layout.HideGit {
				t.Fatal("old shortcut stayed active")
			}
			tt.Key(native.Ctrl|native.Alt, native.KeyG)
			if !a.Layout.HideGit {
				t.Fatal("new shortcut not routed to command")
			}
			open()
			assign("Ctrl+Alt+H")
			click("Cancel")
			tt.Key(native.Ctrl|native.Alt, native.KeyH)
			if !a.Layout.HideGit {
				t.Fatal("canceled keymap draft changed live bindings")
			}
			tt.Key(native.Ctrl|native.Alt, native.KeyG)
			if a.Layout.HideGit {
				t.Fatal("published keymap was mutated by later draft")
			}
			open()
			click("Reset Selected Shortcut")
			click("Apply")
			click("Cancel")
			tt.Key(native.Alt, native.Key0)
			if !a.Layout.HideGit {
				t.Fatal("default shortcut not restored")
			}
		})
	}
}

func TestShortcutConflictAndDisable(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	a.run(showSettings, commandTarget{})
	tt.Frame()
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Settings Shortcuts")
	click("Shortcut binding")
	tt.Key(native.Ctrl, native.KeyA)
	tt.Type("Ctrl+Shift+P")
	click("Assign Shortcut")
	if !tt.HasText("Ctrl+Shift+P is already assigned; change that action first") {
		t.Fatal("shortcut collision not reported")
	}
	click("Disable Shortcut")
	click("OK")
	tt.Key(native.Alt, native.Key0)
	if a.Layout.HideGit {
		t.Fatal("disabled shortcut still active")
	}
	a.run(toggleGit, commandTarget{})
	if !a.Layout.HideGit {
		t.Fatal("disabling shortcut disabled its command")
	}
}

func TestResetShortcutCannotIntroduceDefaultCollision(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Keymap = &application.KeymapSettings{Bindings: map[string]string{string(toggleGit): "Alt+1", string(toggleCommit): ""}}
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 900)
	a.run(showSettings, commandTarget{})
	tt.Frame()
	for _, label := range []string{"Settings Shortcuts", "Shortcut action Toggle Commit", "Reset Selected Shortcut", "OK"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if value, exists := a.Layout.Keymap.Bindings[string(toggleCommit)]; !exists || value != "" {
		t.Fatal("reset silently introduced an effective shortcut collision")
	}
	if _, err := application.NormalizeKeymap(a.Layout.Keymap.Bindings); err != nil {
		t.Fatal("published keymap can no longer be persisted", err)
	}
}

func TestWorkspaceKeymapProtectsLockedEditorBindings(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows keymap acceptance; other platforms are unverified")
	}
	var keys []string
	for key := 'A'; key <= 'Z'; key++ {
		keys = append(keys, string(key))
	}
	for n := 0; n < 10; n++ {
		keys = append(keys, fmt.Sprint(n))
	}
	for n := 1; n <= 12; n++ {
		keys = append(keys, fmt.Sprint("F", n))
	}
	keys = append(keys, "Home", "End")
	a := New(application.New(), application.DefaultLayout())
	for _, prefix := range []string{"Ctrl+", "Alt+", "Ctrl+Alt+", "Ctrl+Shift+", "Alt+Shift+", "Ctrl+Alt+Shift+"} {
		for _, key := range keys {
			value := prefix + key
			candidate := map[string]string{string(toggleGit): value}
			a.Layout.Keymap = &application.KeymapSettings{Bindings: candidate}
			mods, nativeKey, _ := a.nativeWorkspaceShortcut(toggleGit)
			for _, binding := range editor.DefaultBindings() {
				if binding.Key == nativeKey && binding.Mods == mods && binding.Command != editor.NoCommand {
					if _, err := application.NormalizeKeymap(candidate); err == nil {
						t.Errorf("native editor binding %s (%s) was accepted", value, binding.Command)
					}
					break
				}
			}
		}
	}
}

func TestChordShortcutDispatchTimeoutAndEscape(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Keymap = &application.KeymapSettings{Bindings: map[string]string{
		string(toggleGit): "Ctrl+Alt+J Ctrl+Alt+C",
	}}
	a := New(application.New(), layout)
	a.repo().preview = nil
	tt := native.NewTester(a.View, 1440, 900)
	tt.Key(native.Ctrl|native.Alt, native.KeyJ)
	if a.chord.first.String() != "Ctrl+Alt+J" {
		t.Fatalf("first stroke did not create a chord: %+v; scope=%q; focusedEditor=%p", a.chord, a.shortcutScope, a.focusedEditor)
	}
	tt.Frame()
	if !tt.HasText("Shortcut chord waiting") {
		t.Fatal("first stroke did not publish the chord waiting status")
	}
	tt.Key(native.Ctrl|native.Alt, native.KeyC)
	if !a.Layout.HideGit || a.chord.first.Key != "" {
		t.Fatal("second stroke did not dispatch the registered command and clear the chord")
	}
	a.Layout.HideGit = false
	tt.Frame()
	tt.Key(native.Ctrl|native.Alt, native.KeyJ)
	tt.Advance(shortcutChordTimeout + time.Millisecond)
	if a.chord.first.Key != "" {
		t.Fatal("chord did not expire")
	}
	tt.Key(native.Ctrl|native.Alt, native.KeyC)
	if a.Layout.HideGit {
		t.Fatal("expired chord dispatched its command")
	}
	tt.Key(native.Ctrl|native.Alt, native.KeyJ)
	tt.Key(0, native.KeyEscape)
	if a.chord.first.Key != "" {
		t.Fatal("Escape did not cancel the chord")
	}
	tt.Key(native.Ctrl|native.Alt, native.KeyC)
	if a.Layout.HideGit {
		t.Fatal("canceled chord dispatched its command")
	}
}

func TestScopedShortcutPriorityAndTextInput(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Keymap = &application.KeymapSettings{
		Bindings: map[string]string{string(toggleGit): "Ctrl+Alt+G"},
		LocalBindings: map[application.KeymapScope]map[string]string{
			application.KeymapEditor:     {string(toggleTheme): "Ctrl+Alt+G"},
			application.KeymapList:       {string(toggleCommit): "Ctrl+Alt+G"},
			application.KeymapToolWindow: {string(toggleTheme): "Ctrl+Alt+G"},
		},
	}
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 900)
	tt.Key(native.Ctrl|native.Alt, native.KeyG)
	if !a.Layout.HideGit {
		t.Fatal("global shortcut did not run without a local focus scope")
	}
	a.Layout.HideGit = false
	a.run(openCode, commandTarget{source: "working", file: a.Model.Active().Repository.Working[0]})
	tt.Frame()
	if err := tt.Click("Source editor"); err != nil {
		t.Fatal(err)
	}
	if a.shortcutScope != application.KeymapEditor {
		t.Fatalf("source editor scope = %q", a.shortcutScope)
	}
	dark := a.Layout.Dark
	tt.Key(native.Ctrl|native.Alt, native.KeyG)
	if a.Layout.Dark == dark || a.Layout.HideGit {
		t.Fatal("editor-local shortcut did not override the global command")
	}
	listBounds, ok := tt.Find("Working Changes list")
	if !ok {
		t.Fatal("working changes list not found")
	}
	tt.ClickAt(listBounds.X+listBounds.W/2, listBounds.Y+listBounds.H/2)
	if a.shortcutScope != application.KeymapList {
		t.Fatalf("working list scope = %q", a.shortcutScope)
	}
	tt.Key(native.Ctrl|native.Alt, native.KeyG)
	if !a.Layout.HideCommit || a.Layout.HideGit {
		t.Fatal("list-local shortcut did not override the global command")
	}
	a.Layout.HideCommit = false
	tt.Frame()
	if err := tt.Click("Amend last commit"); err != nil {
		t.Fatal(err)
	}
	if a.shortcutScope != application.KeymapToolWindow {
		t.Fatalf("tool window scope = %q", a.shortcutScope)
	}
	dark = a.Layout.Dark
	tt.Key(native.Ctrl|native.Alt, native.KeyG)
	if a.Layout.Dark == dark || a.Layout.HideGit {
		t.Fatal("tool-window shortcut did not override the global command")
	}
}

func TestChordDoesNotInterceptTextInput(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Keymap = &application.KeymapSettings{Bindings: map[string]string{
		string(toggleGit): "Ctrl+Alt+J Ctrl+Alt+C",
	}}
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Commit message"); err != nil {
		t.Fatal(err)
	}
	tt.Key(native.Ctrl|native.Alt, native.KeyJ)
	tt.TypeKey(native.Ctrl, native.KeyC, "c")
	if a.Layout.HideGit || a.chord.first.Key != "" {
		t.Fatal("text input triggered a workspace chord")
	}
	if !strings.Contains(a.Model.Active().Message, "c") {
		t.Fatal("text input was intercepted")
	}
}

func TestEffectiveChordHintsInMenusAndEditor(t *testing.T) {
	layout := application.DefaultLayout()
	pushChord := "Ctrl+Alt+J Ctrl+Alt+C"
	fileChord := "Ctrl+Alt+U Ctrl+Alt+I"
	layout.Keymap = &application.KeymapSettings{Bindings: map[string]string{
		string(push):     pushChord,
		string(goToFile): fileChord,
	}}
	a := New(application.New(), layout)
	a.repo().tabs = nil
	a.repo().preview = nil
	tt := native.NewTester(a.View, 1440, 900)
	tt.Frame()
	if !tt.HasText(fileChord) {
		t.Fatal("editor hint did not use the effective chord")
	}
	if err := tt.Click("Main menu"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Git menu"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(pushChord) {
		t.Fatal("menu hint did not use the effective chord")
	}
	tt.CloseMenu()
}

func TestKeymapChordScopeScreenshots(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			tt := native.NewTester(a.View, 1440, 900)
			for _, label := range []string{"Settings", "Settings Shortcuts", "Shortcut scope"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			tt.Key(0, native.KeyDown)
			tt.Key(0, native.KeyDown)
			tt.Key(0, native.KeyEnter)
			chord := "Ctrl+Alt+J Ctrl+Alt+C"
			if a.settings.keymapScope != application.KeymapList {
				t.Fatalf("keyboard scope selection = %q, want list", a.settings.keymapScope)
			}
			if err := tt.Click("Shortcut binding"); err != nil {
				t.Fatal(err)
			}
			tt.Key(native.Ctrl, native.KeyA)
			tt.Type(chord)
			if err := tt.Click("Assign Shortcut"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText(chord) {
				t.Fatal("assigned chord is not visible in the scoped settings")
			}
			exportSessionScreenshot(t, tt, "keymap-chord-scope-"+map[bool]string{false: "light", true: "dark"}[dark])
			for _, label := range []string{"Apply", "Cancel"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			if got := a.Layout.Keymap.LocalBindings[application.KeymapList][string(toggleGit)]; got != chord {
				t.Fatalf("scoped chord setting = %q", got)
			}
		})
	}
}
