package ui

import (
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestCommitToolbarTargetsActiveChangeList(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("README.md"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Show Diff"); err != nil {
		t.Fatal(err)
	}
	if got := a.repo().preview; got == nil || got.path != "README.md" || got.source != "staged" {
		t.Fatalf("wrong preview: %v", got)
	}
	if err := tt.Click("View Options"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Directory", "Repository", "Module", "Ignored Files"} {
		if !slices.Contains(tt.Menu(), label) {
			t.Fatalf("missing %s: %v", label, tt.Menu())
		}
	}
	if err := tt.ChooseMenuItem("Directory"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("cmd/vela") {
		t.Fatal("directory grouping missing")
	}
	clickFolderArrow(t, tt, "Working Changes folder cmd/vela")
	if !outlineContains(&a.repo().working, "internal/ui/theme.go") || outlineContains(&a.repo().working, "cmd/vela/main.go") {
		t.Fatal("collapsing one directory should only hide its own files")
	}
	if err := tt.Click("Collapse All"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("internal/ui/theme.go") {
		t.Fatal("files are still expanded")
	}
	if err := tt.Click("Expand All"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("internal/ui/theme.go") {
		t.Fatal("files did not expand")
	}
	if err := tt.Click("View Options"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Repository"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("View Options"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Ignored Files"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(".idea/workspace.xml") {
		t.Fatal("ignored fixture is not visible")
	}
	if err := tt.RightClick(".idea/workspace.xml"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stage Changes"); err == nil {
		t.Fatal("ignored file can be staged")
	}
}

func clickFolderArrow(t *testing.T, tt *native.Tester, label string) {
	t.Helper()
	bounds, ok := tt.Find(label)
	if !ok {
		t.Fatalf("missing folder %q", label)
	}
	tt.ClickAt(bounds.X-(15+gap+folderIndent/2), bounds.Y+bounds.H/2)
	tt.Frame()
}

func outlineContains(outline *folderOutline, path string) bool {
	for i := range outline.Rows() {
		if outline.Item(i) == path {
			return true
		}
	}
	return false
}
