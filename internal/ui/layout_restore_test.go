package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters"
	"vela/internal/application"
)

func TestRestoreDefaultLayout(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			layout.Top, layout.Commit, layout.Branches, layout.Log, layout.Files = 70, 35, 30, 50, 65
			layout.HideCommit, layout.HideGit, layout.HideBranches, layout.HideDetails = true, true, true, true
			layout.Editor.TabSize = 8
			a := New(application.New(), layout)
			a.Model.Switch(1)
			a.run(newScratch, commandTarget{})
			inactive := a.repo().tabs[a.repo().active].code
			inactive.Buffer.Set("inactive repository draft")
			a.Model.Switch(0)
			a.run(newScratch, commandTarget{})
			code := a.repo().tabs[a.repo().active].code
			code.Buffer.Set("unsaved draft")
			code.Select(3, 3)
			a.Model.Active().Message = "commit draft"
			a.Model.Active().Working = "internal/ui/theme.go"
			a.Model.Active().Staged = "go.mod"
			a.Model.Active().Branch = "feature/native-ui"
			a.repo().toolPage, a.repo().consoleOpen = "Project", true
			session := *a.Model.Active()
			tabs := append([]tab(nil), a.repo().tabs...)
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Main menu"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("View menu"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Restore Default Layout"); err != nil {
				t.Fatal(err)
			}
			defaults := application.DefaultLayout()
			expected := layout
			expected.Top, expected.Commit, expected.Branches, expected.Log, expected.Files = defaults.Top, defaults.Commit, defaults.Branches, defaults.Log, defaults.Files
			expected.HideCommit, expected.HideGit, expected.HideBranches, expected.HideDetails = false, false, false, false
			if a.Layout != expected {
				t.Fatalf("layout/preferences: got %+v, want %+v", a.Layout, expected)
			}
			if !reflect.DeepEqual(*a.Model.Active(), session) || !reflect.DeepEqual(a.repo().tabs, tabs) || code.Cursor() != 3 || !code.Buffer.Modified() {
				t.Fatal("restoration changed selection, drafts or editor state")
			}
			if code.Buffer.String() != "unsaved draft" || inactive.Buffer.String() != "inactive repository draft" || !inactive.Buffer.Modified() {
				t.Fatal("restoration lost active or inactive editor content")
			}
			for _, label := range []string{"Working Changes", "Staged Changes", "Branches", "Changed Files", "Commit Details"} {
				if !tt.HasText(label) {
					t.Fatalf("restored panel missing: %s", label)
				}
			}
			path := filepath.Join(t.TempDir(), "layout.json")
			if err := adapters.SaveLayout(path, a.Layout); err != nil {
				t.Fatal(err)
			}
			loaded, err := adapters.LoadLayout(path)
			if err != nil || loaded != expected {
				t.Fatalf("layout reload: %+v, %v", loaded, err)
			}
			a.run(commandID("workspace.restore-default-layout"), commandTarget{})
			if a.Layout != expected || code.Cursor() != 3 {
				t.Fatal("repeated restoration changed layout or editor")
			}
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				writeScreenshot(t, tt, filepath.Join(dir, map[bool]string{false: "light", true: "dark"}[dark]+"-restored.png"))
			}
		})
	}
}

func TestRestoreDefaultLayoutFromKeyboardSearch(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Layout.HideGit = true
	tt := native.NewTester(a.View, 1440, 900)
	tt.Key(native.Ctrl|native.Shift, native.KeyP)
	tt.Type("Restore Default Layout")
	if !tt.HasText("Restore Default Layout") {
		t.Fatal("restore action is missing from search")
	}
	tt.Key(0, native.KeyEnter)
	if a.Layout.HideGit || !tt.HasText("Branches") {
		t.Fatal("keyboard action did not restore Git")
	}
}
