package ui

import (
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestKeyboardMenusAndTabs(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	file := "internal/ui/theme.go"
	if err := tt.Click(file); err != nil {
		t.Fatal(err)
	}
	tt.Key(native.Shift, native.KeyF10)
	if !slices.Contains(tt.Menu(), "Jump to Source") {
		t.Fatalf("keyboard context menu: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Jump to Source"); err != nil {
		t.Fatal(err)
	}
	r := a.repo()
	if len(r.tabs) != 2 || r.tabs[r.active].code == nil {
		t.Fatal("opening a source tab replaced the diff")
	}
	code := r.tabs[r.active].code
	code.Select(0, 0)
	box, ok := tt.Find("Source editor")
	if !ok {
		t.Fatal("source editor not found")
	}
	tt.ClickAt(box.X+100, box.Y+14)
	tt.Type("// test")
	if !code.Buffer.Modified() {
		t.Fatal("source editor did not receive keyboard input")
	}
	cursor := code.Cursor()
	tt.Key(native.Cmd, native.KeyW)
	if len(r.tabs) != 2 {
		t.Fatal("shortcut discarded an unsaved tab")
	}
	if err := tt.Click("main.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("theme.go •"); err != nil {
		t.Fatal(err)
	}
	if r.tabs[r.active].code != code || code.Cursor() != cursor || !code.Buffer.Modified() {
		t.Fatal("tab did not retain its editor state")
	}
	chooseRepository(t, tt, "sandbox")
	if len(a.repo().tabs) != 0 {
		t.Fatal("tabs leaked into sandbox")
	}
	chooseRepository(t, tt, "vela")
	if a.repo().tabs[a.repo().active].code != code {
		t.Fatal("tabs lost on return to repository")
	}
	tt.Key(native.Alt, native.Key0)
	if !a.Layout.HideGit {
		t.Fatal("Alt+0 did not hide Git")
	}
}

func TestStageSelectionAndCommitPreview(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ClickWith(native.Cmd, "go.mod"); err != nil {
		t.Fatal(err)
	}
	if a.repo().workingChosen.Len() != 2 {
		t.Fatal("modifier click did not retain both rows")
	}
	if err := tt.RightClick("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Stage Changes"); err != nil {
		t.Fatal(err)
	}
	s := a.Model.Active()
	if len(s.Repository.Working) != 1 || len(s.Repository.Staged) != 3 {
		t.Fatalf("wrong mock stage result: %d working, %d staged", len(s.Repository.Working), len(s.Repository.Staged))
	}
	if err := tt.Click("Commit message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("A mock commit")
	if err := tt.Click("Preview Commit"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Mock commit preview ready • staged changes preserved") {
		t.Fatal("preview did not report outcome")
	}
}

func TestLogNavigationAndMockStates(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Filter branches"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyTab)
	if !tt.Focused("Branches list") {
		t.Fatal("Tab did not move from branch filter to branch list")
	}
	tt.Key(native.Shift, native.KeyTab)
	if !tt.Focused("Filter branches") {
		t.Fatal("Shift+Tab did not return to the filter")
	}
	if err := tt.Click("test(ui): cover keyboard focus and panel toggles"); err != nil {
		t.Fatal(err)
	}
	if a.Model.Active().Commit != 1 || len(a.repo().tabs) != 1 {
		t.Fatal("commit selection disturbed the editor")
	}
	if err := tt.Click("README.md"); err != nil {
		t.Fatal(err)
	}
	if a.Model.Active().Staged != "README.md" || a.Model.Active().Historical != "" {
		t.Fatal("staged selection leaked into history")
	}
	if err := tt.Click("Filter commits"); err != nil {
		t.Fatal(err)
	}
	tt.Type("no such commit")
	if !tt.HasText("No matching commits") {
		t.Fatal("empty filter state missing")
	}
	runAction := func(label string) {
		t.Helper()
		tt.Key(native.Cmd|native.Shift, native.KeyP)
		tt.Type(label)
		tt.Key(0, native.KeyEnter)
	}
	runAction("Show mock loading")
	if !tt.HasText("Loading mock history…") {
		t.Fatal("loading state missing")
	}
	runAction("Show mock error")
	if !tt.HasText("Mock history could not be loaded") {
		t.Fatal("error state missing")
	}
	runAction("Retry history")
	if !tt.HasText("No matching commits") {
		t.Fatal("retry did not restore filtered results")
	}
}

func TestWindowCloseRequiresConfirmationForUnsavedEdits(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	a.openCode("working", a.Model.Active().Repository.Working[0])
	a.repo().tabs[a.repo().active].code.Buffer.Set("package changed")
	if a.RequestClose() {
		t.Fatal("window accepted close with pending edits")
	}
	tt.Frame()
	if !tt.HasText("Discard mock edits?") {
		t.Fatal("missing close confirmation")
	}
	tt.Key(0, native.KeyEscape)
	if !a.hasUnsavedTabs() {
		t.Fatal("Escape discarded edits")
	}
	a.RequestClose()
	tt.Frame()
	if err := tt.Click("Discard and Close"); err != nil {
		t.Fatal(err)
	}
	if !a.RequestClose() {
		t.Fatal("confirmed close was blocked")
	}
}

func TestWorkspaceRestoresTabsAndActiveRepository(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	f := a.Model.Active().Repository.Staged[0]
	a.openCode("staged", f)
	a.repo().tabs[a.repo().active].code.Select(2, 2)
	a.Model.Switch(1)
	sandbox := a.Model.Active().Repository.History[0]
	a.openDiff(sandbox.Hash, sandbox.Files[0])
	b := New(application.New(), application.DefaultLayout())
	b.RestoreSession(a.SessionSnapshot())
	if b.Model.Active().Repository.Name != "sandbox" || len(b.repo().tabs) != 1 || b.repo().tabs[0].diff == nil {
		t.Fatal("active repository and diff were not restored")
	}
	b.Model.Switch(0)
	r := b.repo()
	if len(r.tabs) != 2 || r.tabs[r.active].code == nil || r.tabs[r.active].code.Cursor() != 2 {
		t.Fatal("inactive repository lost its tabs, active tab or cursor")
	}
}

func TestSplitterDragAndRestore(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	before := a.Layout.Commit
	// The first upper divider is between the Commit tool window and editor.
	x, y := float32(409), float32(200)
	tt.Press(x, y)
	tt.Move(x+70, y)
	tt.Release(x+70, y)
	if a.Layout.Commit <= before {
		t.Fatalf("split not resized: %g", a.Layout.Commit)
	}
	resized := a.Layout.Commit
	tt.Key(native.Alt, native.Key1)
	if !a.Layout.HideCommit {
		t.Fatal("Commit did not hide")
	}
	tt.Key(native.Alt, native.Key1)
	if a.Layout.HideCommit || a.Layout.Commit != resized {
		t.Fatal("Commit size did not survive restore")
	}
}
