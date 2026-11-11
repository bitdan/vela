package ui

import (
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestShelfMockActions(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Shelf"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Expand UI baseline changes"); err != nil {
		t.Fatal(err)
	}
	if arrow, ok := tt.Access("Collapse UI baseline changes"); !ok || arrow.Role != native.RoleDisclosure || !arrow.Expanded {
		t.Fatal("shelf arrow did not expose its expanded state")
	}
	if err := tt.Click("Collapse UI baseline changes"); err != nil {
		t.Fatal(err)
	}
	if arrow, ok := tt.Access("Expand UI baseline changes"); !ok || arrow.Expanded || tt.HasText("docs/ui-notes.md") {
		t.Fatal("shelf arrow did not collapse the file list")
	}
	if err := tt.RightClick("UI baseline changes"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Rename…"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Shelf name"); err != nil {
		t.Fatal(err)
	}
	tt.Key(native.Cmd, native.KeyA)
	tt.Type("Layout notes")
	tt.Key(0, native.KeyEnter)
	if err := tt.RightClick("Layout notes"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Delete…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Recently Deleted") {
		t.Fatal("shelf was not soft deleted")
	}
	if err := tt.RightClick("Layout notes"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Restore"); err != nil {
		t.Fatal(err)
	}
	if err := tt.RightClick("Layout notes"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Unshelve…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Unshelve mock changes?") {
		t.Fatal("missing confirmation")
	}
	if err := tt.Click("Unshelve"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Commit"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("docs/ui-notes.md") {
		t.Fatal("mock unshelve did not add the file")
	}
}
