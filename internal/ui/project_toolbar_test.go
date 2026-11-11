package ui

import (
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestActivityButtonTogglesCurrentPanel(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Commit = 30
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Commit tool window")
	if !a.Layout.HideCommit || tt.HasText("Working Changes") {
		t.Fatal("Commit did not collapse")
	}
	click("Commit tool window")
	if a.Layout.HideCommit || !tt.HasText("Working Changes") {
		t.Fatal("Commit did not restore")
	}
	click("Shelf")
	click("Commit tool window")
	if !a.Layout.HideCommit {
		t.Fatal("Shelf did not collapse with Commit")
	}
	click("Commit tool window")
	if a.Layout.HideCommit || !tt.HasText("Shelved Changes") {
		t.Fatal("Shelf state was lost")
	}
	click("Project")
	click("Project")
	if !a.Layout.HideCommit || tt.HasText("Project tree") {
		t.Fatal("Project did not collapse")
	}
	click("Project")
	if a.Layout.HideCommit || !tt.HasText("Project tree") || a.Layout.Commit != 30 {
		t.Fatal("Project did not restore its panel and width")
	}
}

func TestProjectToolbarActions(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Project"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Collapse Project Tree"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("cmd/vela/main.go") && a.repo().project.Rows() > 4 {
		t.Fatal("tree did not collapse")
	}
	if err := tt.Click("Select Opened File"); err != nil {
		t.Fatal(err)
	}
	r := a.repo()
	if r.projectIndex < 0 || r.project.Item(r.projectIndex) != "cmd/vela/main.go" {
		t.Fatal("open file was not located")
	}
	if err := tt.Click("Expand Project Tree"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("internal/ui/theme.go") {
		t.Fatal("tree did not expand")
	}
	if err := tt.Click("New Project Item"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("File…"); err == nil {
		t.Fatal("disk file creation enabled in mock mode")
	}
	tt.CloseMenu()
	if err := tt.Click("New Project Item"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Scratch File"); err != nil {
		t.Fatal(err)
	}
	entry := a.activeDocument()
	if entry == nil || entry.source != "scratch" || entry.code == nil {
		t.Fatal("scratch editor did not open")
	}
}
