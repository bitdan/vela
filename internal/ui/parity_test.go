package ui

import (
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestIDEAToolbars(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Current branch"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Show Diff with Working Tree") || !slices.Contains(tt.Menu(), "Update") {
		t.Fatalf("branch menu: %v", tt.Menu())
	}
	tt.CloseMenu()
	if err := tt.Click("Console"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Mock task output") || tt.HasText("Commit Log / Graph") {
		t.Fatal("Console did not replace Log")
	}
	if err := tt.Click("Log"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Commit Log / Graph") || !tt.HasText("Local") || !tt.HasText("Remote") {
		t.Fatal("Log or branch groups missing")
	}
	if err := tt.RightClick("origin/main"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tt.Menu(), "Rename…") {
		t.Fatal("remote branch menu offered local rename")
	}
	if err := tt.ChooseMenuItem("Update"); err == nil {
		t.Fatal("remote branch update was enabled")
	}
	tt.CloseMenu()
	if err := tt.RightClick("feature/native-ui"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Rename…") {
		t.Fatal("local branch menu missing rename")
	}
	if err := tt.ChooseMenuItem("Show Diff with Working Tree"); err != nil {
		t.Fatal(err)
	}
	if a.repo().preview == nil || a.repo().preview.source != "local vs feature/native-ui" {
		t.Fatal("branch compare used wrong target")
	}
	if a.Model.Active().Repository.CurrentBranch != "main" {
		t.Fatal("branch comparison changed HEAD")
	}
}

func TestCommitAndHistoricalFileMenus(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	history := a.Model.Active().Repository.History
	merge := history[2]
	if err := tt.Click(merge.Subject); err != nil {
		t.Fatal(err)
	}
	tt.Key(native.Shift, native.KeyF10)
	if err := tt.ChooseMenuItem("Go to Parent Commit", history[3].Hash+" "+history[3].Subject); err != nil {
		t.Fatal(err)
	}
	if a.Model.Active().Commit != 3 {
		t.Fatal("wrong merge parent")
	}
	tt.Key(native.Shift, native.KeyF10)
	if err := tt.ChooseMenuItem("Copy Revision Number"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != history[3].Hash {
		t.Fatal("copied the wrong revision")
	}
	tt.Key(native.Shift, native.KeyF10)
	if err := tt.ChooseMenuItem("Reset Current Branch to Here…"); err == nil {
		t.Fatal("real Git command was enabled")
	}
	tt.Key(0, native.KeyEscape)
	tt.Key(0, native.KeyLeft)
	if a.Model.Active().Commit != 2 {
		t.Fatal("menu close lost keyboard focus or went to the wrong child")
	}
	if a.Model.Active().Repository.CurrentBranch != "main" {
		t.Fatal("log navigation changed HEAD")
	}

	if err := tt.RightClick("Historical cmd/vela/main.go"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Compare Before with Local", "Open Repository Version", "History Up to Here"} {
		if !slices.Contains(tt.Menu(), label) {
			t.Fatalf("historical file menu missing %s: %v", label, tt.Menu())
		}
	}
	if err := tt.ChooseMenuItem("Open Repository Version"); err != nil {
		t.Fatal(err)
	}
	entry := a.repo().tabs[a.repo().active]
	if entry.code == nil || !entry.code.ReadOnly || entry.source != "revision:"+merge.Hash {
		t.Fatal("opened the wrong historical version")
	}
}

func TestObjectMenusAndDiffPreview(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.RightClick("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	items := tt.Menu()
	if !slices.Contains(items, "Show Diff") || !slices.Contains(items, "Show Diff in a New Tab") {
		t.Fatalf("working file menu: %v", items)
	}
	if err := tt.ChooseMenuItem("Show Diff"); err != nil {
		t.Fatal(err)
	}
	if len(a.repo().tabs) != 1 || !tt.HasText("Diff Preview") {
		t.Fatal("preview modified pinned tabs")
	}
	tt.Key(0, native.KeyEscape)
	if tt.HasText("Diff Preview") {
		t.Fatal("Escape did not close preview")
	}
	if err := tt.RightClick("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Show Diff in a New Tab"); err != nil {
		t.Fatal(err)
	}
	if len(a.repo().tabs) != 2 {
		t.Fatal("new-tab action did not pin a diff")
	}
	if err := tt.Click("Shelf"); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick("UI baseline changes"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Unshelve…") {
		t.Fatalf("Shelf entry menu: %v", tt.Menu())
	}
	tt.CloseMenu()
	if err := tt.Click("Expand UI baseline changes"); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick("docs/ui-notes.md"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tt.Menu(), "Import Patches…") || !slices.Contains(tt.Menu(), "Compare with Local") {
		t.Fatalf("Shelf file received the entry menu: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Show Diff"); err != nil {
		t.Fatal(err)
	}
	if preview := a.repo().preview; preview == nil || preview.source != "shelf" {
		t.Fatal("wrong shelf diff source")
	}
	if err := tt.Click("Project"); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Copy Path/Reference…") || !slices.Contains(tt.Menu(), "Git") {
		t.Fatalf("project menu: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Git", "Show Diff"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Diff Preview") {
		t.Fatal("project Git submenu did not open preview")
	}
	tt.Key(0, native.KeyEscape)
	if err := tt.RightClick("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open in Editor"); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick("Source editor"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "Git") {
		t.Fatalf("editor Git submenu missing: %v", tt.Menu())
	}
}
