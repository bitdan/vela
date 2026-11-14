package ui

import (
	"fmt"
	"runtime"
	"testing"

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
