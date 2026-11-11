package ui

import (
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestActivityBarSwitchesProjectAndCommit(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	project, ok := tt.Find("Project")
	if !ok || project.X >= activityWidth {
		t.Fatal("Project belongs in the activity bar")
	}
	if tt.HasText("Project Files") {
		t.Fatal("obsolete Project Files tab")
	}
	if err := tt.Click("Project"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Project tree") || tt.HasText("Working Changes") {
		t.Fatal("Project did not replace Commit")
	}
	if err := tt.RightClick("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"New", "Copy Path/Reference…", "Local History", "Git", "Compare With…"} {
		if !slices.Contains(tt.Menu(), label) {
			t.Fatalf("Project menu missing %q: %v", label, tt.Menu())
		}
	}
	if err := tt.ChooseMenuItem("Copy Path/Reference…"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != "internal/ui/theme.go" {
		t.Fatal("copied the wrong project path")
	}
	if err := tt.Click("Commit tool window"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Working Changes") || tt.HasText("Project tree") {
		t.Fatal("Commit did not replace Project")
	}
	if err := tt.RightClick("internal/ui/theme.go"); err != nil {
		t.Fatal(err)
	}
	want := []string{"Commit File…", "Rollback…", "Move to Another Changelist…", "Show Diff", "Show Diff in a New Tab", "Jump to Source", "Copy Path/Reference…"}
	if got := tt.Menu(); len(got) < len(want) || !slices.Equal(got[:len(want)], want) {
		t.Fatalf("Changes menu order: %v", got)
	}
}
