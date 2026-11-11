package ui

import (
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestCommitPreviewTabContextMenu(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick("Commit: theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Close"); err != nil {
		t.Fatal(err)
	}
	r := a.repo()
	if r.preview != nil || len(r.tabs) != 1 || r.tabs[0].title != "main.go" {
		t.Fatal("right-click close affected the pinned tab")
	}
	if err := tt.Click("go.mod"); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick("Commit: go.mod"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Close Other Tabs"); err != nil {
		t.Fatal(err)
	}
	if r.preview == nil || len(r.tabs) != 0 {
		t.Fatal("close others did not retain the preview")
	}
	if err := tt.RightClick("Commit: go.mod"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Pin Tab"); err != nil {
		t.Fatal(err)
	}
	if r.preview != nil || len(r.tabs) != 1 || !r.tabs[0].pinned {
		t.Fatal("pin did not promote preview")
	}
}

func TestChangeClickPreviewsAndDoubleClickOpensSource(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	const file = "internal/ui/theme.go"
	if err := tt.Click(file); err != nil {
		t.Fatal(err)
	}
	r := a.repo()
	if r.preview == nil || r.preview.path != file || len(r.tabs) != 1 || !tt.HasText("Commit: theme.go") {
		t.Fatalf("single click did not show a temporary Commit tab: preview=%v tabs=%d texts=%v", r.preview, len(r.tabs), tt.Texts())
	}
	if err := tt.Click("Next changed file"); err != nil {
		t.Fatal(err)
	}
	if r.preview.path != "go.mod" || a.Model.Active().Working != "go.mod" {
		t.Fatal("Diff navigation did not follow the Working list")
	}
	if err := tt.Click(file); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(file); err != nil {
		t.Fatal(err)
	}
	if r.preview != nil || len(r.tabs) != 2 || r.tabs[r.active].code == nil || r.tabs[r.active].path != file {
		t.Fatal("double click did not open editable source")
	}
	if !tt.HasText("vela › internal › ui › theme.go") || !tt.HasText("UTF-8") {
		t.Fatal("status bar is missing file context")
	}
}
