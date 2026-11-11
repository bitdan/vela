package ui

import (
	"slices"
	"testing"
	"time"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestBranchCompareAndHistoryControls(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	clickFolderArrow(t, tt, "Branch folder Local/feature")
	if !outlineContains(&a.repo().branches, "main") || outlineContains(&a.repo().branches, "feature/native-ui") {
		t.Fatal("collapsing a branch prefix should leave its siblings visible")
	}
	if err := tt.Click("Filter branches"); err != nil {
		t.Fatal(err)
	}
	tt.Type("native-ui")
	if !outlineContains(&a.repo().branchResults, "feature/native-ui") {
		t.Fatal("search did not reveal the branch under a closed folder")
	}
	tt.Key(native.Cmd, native.KeyA)
	tt.Key(0, native.KeyBackspace)
	if outlineContains(&a.repo().branches, "feature/native-ui") {
		t.Fatal("clearing search expanded the original branch folder")
	}
	if err := tt.Click("Branch folder Local/feature"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyRight)
	tt.Frame()
	if !outlineContains(&a.repo().branches, "feature/native-ui") {
		t.Fatal("Right did not expand the focused branch folder")
	}
	if err := tt.RightClick("feature/native-ui"); err != nil {
		t.Fatal(err)
	}
	merge := "Merge 'feature/native-ui' into 'main'"
	if !slices.Contains(tt.Menu(), merge) {
		t.Fatalf("branch menu: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem(merge); err == nil {
		t.Fatal("real merge enabled")
	}
	tt.CloseMenu()
	if err := tt.RightClick("feature/native-ui"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Compare with 'main'"); err != nil {
		t.Fatal(err)
	}
	if a.repo().preview == nil || a.repo().preview.source != "feature/native-ui vs main" {
		t.Fatal("wrong branch comparison")
	}
	clickFolderArrow(t, tt, "Changed Files folder cmd/vela")
	if outlineContains(&a.repo().files, "cmd/vela/main.go") || !outlineContains(&a.repo().files, "go.mod") {
		t.Fatal("closing a historical folder hid unrelated files")
	}
	if err := tt.Click("Collapse Historical Files"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Historical go.mod") {
		t.Fatal("historical files did not collapse")
	}
	if err := tt.Click("Expand Historical Files"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Historical go.mod"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Show Historical Diff"); err != nil {
		t.Fatal(err)
	}
	if a.repo().preview.path != "go.mod" {
		t.Fatal("historical toolbar used wrong file")
	}
}

func TestCloseButtonTargetsItsTab(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.openCode("working", a.Model.Active().Repository.Working[1])
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Close main.go"); err != nil {
		t.Fatal(err)
	}
	if len(a.repo().tabs) != 1 || a.repo().tabs[a.repo().active].title != "theme.go" {
		t.Fatal("close targeted active tab instead of its own tab")
	}
}

func TestCommitDetailsMetadataAndBranches(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Top = 37
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 1000)
	for _, label := range []string{"Build the native workspace", "lin@example.test", "2026/11/10 at 16:00", "Commit reference HEAD", "Commit reference main", "Commit reference origin/stable", "In 3 branches"} {
		if !tt.HasText(label) {
			t.Fatalf("commit details missing %q", label)
		}
	}
	if err := tt.Click("Copy commit revision"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != a.Model.Active().Repository.History[0].Hash {
		t.Fatal("copied the wrong revision from commit details")
	}
	if err := tt.Click("Show containing branches"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Containing branch origin/stable") {
		t.Fatal("containing branches did not expand")
	}
	if err := tt.Click("Hide containing branches"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Containing branch origin/stable") {
		t.Fatal("containing branches stayed visible after collapse")
	}
	history := a.Model.Active().Repository.History
	if err := tt.Click(history[1].Subject); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Commit reference HEAD") || !tt.HasText("In 3 branches") {
		t.Fatal("an ancestor must list containing branches without displaying their tip labels")
	}
	a.Layout.Files = 75
	if err := tt.Click(history[0].Subject); err != nil {
		t.Fatal(err)
	}
	details, ok := tt.Find("Commit Details")
	if !ok {
		t.Fatal("missing commit details scroll area")
	}
	tt.Wheel(details.X+details.W/2, details.Y+details.H/2, 0, 420)
	tt.Advance(300 * time.Millisecond)
	if err := tt.Click("Show containing branches"); err != nil {
		t.Fatalf("branch list was not reachable after scrolling a short details panel: %v", err)
	}
}
