package ui

import (
	"slices"
	"testing"
	"time"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestEmptyEditorQuickActions(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Close main.go")
	for _, label := range []string{"Empty editor", "Go to File", "Recent Files", "Project Navigation"} {
		if !tt.HasText(label) {
			t.Fatalf("empty editor missing %q", label)
		}
	}
	for range 2 {
		tt.CodeDown(native.Shift, native.CodeShiftLeft)
		tt.CodeUp(0, native.CodeShiftLeft)
	}
	if !tt.Focused("Search Everywhere query") {
		t.Fatal("double Shift did not open search from the empty editor")
	}
	tt.Key(0, native.KeyEscape)
	click("Go to File")
	tt.Type("theme.go")
	tt.Key(0, native.KeyEnter)
	if entry := a.activeDocument(); entry == nil || entry.code == nil || entry.path != "internal/ui/theme.go" || tt.HasText("Empty editor") {
		t.Fatal("Go to File did not replace the empty editor with the file")
	}
	click("Close theme.go")
	tt.Key(native.Ctrl, native.KeyE)
	tt.Key(0, native.KeyEnter)
	if entry := a.activeDocument(); entry == nil || entry.path != "internal/ui/theme.go" {
		t.Fatal("Recent Files did not reopen the source")
	}
	click("Close theme.go")
	tt.Key(native.Alt, native.KeyHome)
	if !tt.HasText("Project tree") || !tt.Focused("Project tree") {
		t.Fatal("Project Navigation did not show and focus the tree")
	}
}

func TestTabMenuUsesClickedTab(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	files := a.Model.Active().Repository.Working
	a.openCode("working", files[1])
	a.openCode("working", files[2])
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.RightClick("theme.go"); err != nil {
		t.Fatal(err)
	}
	want := []string{"Close", "Close Other Tabs", "Close All Tabs", "Close Unmodified Tabs", "Close Tabs to the Left", "Close Tabs to the Right", "Close All Read-Only"}
	if got := tt.Menu(); len(got) < len(want) || !slices.Equal(got[:len(want)], want) {
		t.Fatalf("close menu order: %v", got)
	}
	if err := tt.ChooseMenuItem("Close Tabs to the Left"); err != nil {
		t.Fatal(err)
	}
	r := a.repo()
	if len(r.tabs) != 2 || r.tabs[0].title != "theme.go" || r.tabs[r.active].title != "go.mod" {
		t.Fatal("bulk close used active rather than clicked tab")
	}
	if err := tt.RightClick("theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Pin Tab"); err != nil {
		t.Fatal(err)
	}
	r.tabs[1].code.Buffer.Set("module dirty")
	if err := tt.RightClick("theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Close All Tabs"); err == nil {
		t.Fatal("bulk close should be disabled for pinned and dirty tabs")
	}
	tt.CloseMenu()
	if err := tt.RightClick("theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Reopen Closed Tab"); err != nil {
		t.Fatal(err)
	}
	if len(r.tabs) != 3 || r.tabs[r.active].title != "main.go" {
		t.Fatal("last closed tab did not reopen")
	}
	tt.Key(native.Ctrl, native.KeyF4)
	if len(r.tabs) != 2 {
		t.Fatal("Ctrl+F4 did not close the active tab")
	}
}

func TestMoveTabToSplitPreservesDirtyBuffer(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.openCode("working", a.Model.Active().Repository.Working[1])
	r := a.repo()
	code := r.tabs[r.active].code
	code.Buffer.Set("package edited")
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.RightClick("theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Split and Move Down"); err != nil {
		t.Fatal(err)
	}
	if r.split == nil || r.split.code != code || !r.splitVertical || !r.splitMoved || r.tabs[r.active].title != "main.go" {
		t.Fatal("tab was not moved below the original editor")
	}
	if err := tt.Click("Close Split"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("theme.go"); err != nil {
		t.Fatal(err)
	}
	if r.tabs[r.active].code != code || code.Buffer.String() != "package edited" {
		t.Fatal("closing split lost the moved tab or its edits")
	}
}

func TestTitleBarActions(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	tt.SetTitleBar(native.TitleBar{Height: 40, Right: 138})
	if err := tt.Click("Main menu"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Switch repository"); ok {
		t.Fatal("expanded menu did not replace the compact toolbar")
	}
	if err := tt.Click("View menu"); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); !slices.Contains(menu, "Project") ||
		slices.Contains(menu, "Search Everywhere") || slices.Contains(menu, "Switch theme") {
		t.Fatalf("unexpected View menu: %v", menu)
	}
	if err := tt.ChooseMenuItem("Project"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Project tree") {
		t.Fatal("View menu did not open Project")
	}
	if err := tt.Click("View menu"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Git menu"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Fetch") {
		t.Fatalf("did not switch to the Git menu: %v", tt.Menu())
	}
	for _, label := range []string{"Commit…", "Update Project…", "Worktrees…", "Show Git Log", "Patch", "Uncommitted Changes",
		"Current File", "GitHub", "Gitee", "GitLab", "Manage Remotes…", "Clone…"} {
		if !slices.Contains(tt.Menu(), label) {
			t.Fatalf("Git menu is missing %q", label)
		}
	}
	if err := tt.Click("Main menu"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.Find("Switch repository"); !ok {
		t.Fatal("compact toolbar did not return")
	}
	if err := tt.Click("Search Everywhere"); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Search Everywhere query") {
		t.Fatal("search did not focus its query")
	}
}

func TestSearchEverywhereDoubleShift(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	tap := func() {
		tt.CodeDown(native.Shift, native.CodeShiftLeft)
		tt.CodeUp(0, native.CodeShiftLeft)
	}
	tap()
	if tt.Focused("Search Everywhere query") {
		t.Fatal("one Shift opened search")
	}
	tt.Advance(time.Second)
	tap()
	if tt.Focused("Search Everywhere query") {
		t.Fatal("two taps outside the gesture interval opened search")
	}
	tt.CodeDown(native.Shift, native.CodeShiftLeft)
	tt.Key(native.Shift, native.KeyA)
	tt.CodeUp(0, native.CodeShiftLeft)
	if tt.Focused("Search Everywhere query") {
		t.Fatal("Shift used in a chord opened search")
	}
	tap()
	tap()
	if !tt.Focused("Search Everywhere query") {
		t.Fatal("double Shift did not focus search")
	}
	tt.Type("theme.go")
	tt.Key(0, native.KeyEnter)
	if doc := a.activeDocument(); doc == nil || doc.code == nil || doc.path != "internal/ui/theme.go" {
		t.Fatal("Enter did not open the matched source")
	}
	if !tt.Focused("Source editor") {
		t.Fatal("opening a source result did not focus the editor")
	}
	tap()
	tap()
	if !tt.Focused("Search Everywhere query") {
		t.Fatal("double Shift from the editor did not reopen search")
	}
	tt.Key(0, native.KeyEscape)
	tt.Frame()
	if a.search.open || !tt.Focused("Source editor") {
		t.Fatalf("Escape did not restore editor focus: search open=%v", a.search.open)
	}
}

func TestSearchEverywhereCategories(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Search Everywhere")
	click("Files")
	if !tt.Focused("Search Everywhere query") {
		t.Fatal("switching categories did not return focus to the query")
	}
	tt.Key(0, native.KeyDown)
	if a.search.selected != 1 {
		t.Fatalf("Down selected row %d", a.search.selected)
	}
	tt.Key(native.Shift, native.KeyEnter)
	if split := a.repo().split; split == nil || split.path != "cmd/vela/main.go" {
		t.Fatalf("Down and Shift+Enter did not open the second file in a split: %+v", split)
	}
	click("Search Everywhere")
	click("Symbols")
	tt.Type("main")
	tt.Key(0, native.KeyEnter)
	if doc := a.activeDocument(); doc == nil || doc.code == nil || doc.code.SelectedText() != "main" {
		t.Fatal("symbol search did not navigate to the declaration")
	}
	click("Search Everywhere")
	click("Text")
	tt.Type("bootstrap.Run")
	tt.Key(0, native.KeyEnter)
	if doc := a.activeDocument(); doc.code.SelectedText() != "bootstrap.Run" {
		t.Fatal("text search did not select the matching text")
	}
	tt.Key(native.Cmd|native.Shift, native.KeyP)
	tt.Type("Settings")
	tt.Key(0, native.KeyEnter)
	if !a.settingsOpen || a.search.open {
		t.Fatal("Actions shortcut did not dispatch Settings")
	}
}
