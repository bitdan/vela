package ui

import (
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestLogFilterPopupsSwitchWithoutOverlap(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("User")
	if !slices.Contains(tt.Menu(), "Recent") {
		t.Fatal("user menu did not open")
	}
	click("Date")
	if !tt.HasText("Date range filter") || len(tt.Menu()) != 0 {
		t.Fatalf("Date and User did not switch cleanly: menu=%v", tt.Menu())
	}
	click("Last 7 days")
	click("Paths")
	if tt.HasText("Date range filter") || !slices.Contains(tt.Menu(), "Select in Tree…") {
		t.Fatalf("Paths and Date did not switch cleanly: menu=%v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("Select…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Filter by path") || len(tt.Menu()) != 0 {
		t.Fatal("path editor overlapped its menu")
	}
	click("Date")
	if tt.HasText("Filter by path") || !tt.HasText("Date range filter") {
		t.Fatal("Date overlapped the path editor")
	}
	if a.repo().datePicker.draft.preset != logDateAll || a.repo().logDate.preset != "" {
		t.Fatal("switching filters did not discard the unapplied date range")
	}
	tt.Key(0, native.KeyEscape)
	if tt.HasText("Date range filter") {
		t.Fatal("Escape left the date panel open")
	}
	click("Date")
	click("User")
	if tt.HasText("Date range filter") || !slices.Contains(tt.Menu(), "Recent") {
		t.Fatalf("User and Date did not switch cleanly: menu=%v", tt.Menu())
	}
	if date, ok := tt.Access("Date"); !ok || date.Expanded {
		t.Fatal("Date button remained expanded after switching to User")
	}
	if err := tt.ChooseMenuItem("Lin"); err != nil {
		t.Fatal(err)
	}
	click("Date")
	click("Last 7 days")
	click("Apply date filter")
	click("Paths")
	if err := tt.ChooseMenuItem("Select…"); err != nil {
		t.Fatal(err)
	}
	click("Log Sort")
	if tt.HasText("Filter by path") || !slices.Contains(tt.Menu(), "By Commit Date") {
		t.Fatal("sorting menu overlapped the path editor")
	}
	if a.repo().logAuthor != "Lin" || a.repo().logDate.preset != logDateWeek {
		t.Fatal("switching popups cleared the applied filters")
	}
}

func TestLogToolbarOptions(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Log Sort"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("No Merges"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText(a.Model.Active().Repository.History[2].Subject) {
		t.Fatal("merge remained visible")
	}
	if err := tt.Click("Log View Options"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Columns", "Author"); err != nil {
		t.Fatal(err)
	}
	if !a.repo().logView.hideAuthor {
		t.Fatal("author column remained visible")
	}
	if button, ok := tt.Access("Cherry-Pick"); !ok || !button.Disabled {
		t.Fatal("real Cherry-Pick was enabled")
	}
	if err := tt.Click("Branch"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "HEAD") || !slices.Contains(tt.Menu(), "Local") {
		t.Fatalf("branch menu: %v", tt.Menu())
	}
	tt.CloseMenu()
	if err := tt.Click("Date"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Last 24 hours"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Apply date filter"); err != nil {
		t.Fatal(err)
	}
	if a.repo().logDate.preset != "Last 24 hours" {
		t.Fatal("date preset not selected")
	}
	if err := tt.Click("Paths"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Select in Tree…", "go.mod"); err != nil {
		t.Fatal(err)
	}
	if a.repo().logPath != "go.mod" || len(a.repo().filteredLog) == 0 || len(a.repo().filteredLog) >= 24 {
		t.Fatal("path and date filters did not combine")
	}
}

func TestLogDateRangeAndColumns(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	branches, okBranches := tt.Find("Filter branches")
	log, okLog := tt.Find("Filter commits")
	if !okBranches || !okLog || branches.Y != log.Y {
		t.Fatalf("branch and log filters are not aligned: %v, %v", branches, log)
	}
	first := a.Model.Active().Repository.History[0]
	author, okAuthor := tt.Find("Commit author " + first.Hash)
	graph, okGraph := tt.Find("Commit graph " + first.Hash)
	date, okDate := tt.Find("Commit date " + first.Hash)
	if !okAuthor || !okGraph || !okDate || !(author.X < graph.X && graph.X < date.X) {
		t.Fatal("expected author on the left, followed by the graph, with date on the right")
	}
	click("Date")
	click("Last 7 days")
	click("Cancel date filter")
	if a.repo().logDate.preset != "" {
		t.Fatal("cancel applied the draft")
	}
	click("Date")
	click("Last 7 days")
	click("End date")
	click("November 3, 2026")
	if apply, ok := tt.Access("Apply date filter"); !ok || !apply.Disabled {
		t.Fatal("an inverted date range was accepted")
	}
	click("November 9, 2026")
	click("Apply date filter")
	if r := a.repo(); r.logDate.preset != logDateCustom || len(r.filteredLog) == 0 || r.filteredLog[0] < 1 {
		t.Fatal("custom range did not exclude the latest commit")
	}
	click("Date")
	click("End follows latest commit")
	click("Apply date filter")
	if r := a.repo(); !r.logDate.followLatest || len(r.filteredLog) == 0 || r.filteredLog[0] != 0 {
		t.Fatal("following the latest commit did not extend the end")
	}
	click("Date")
	click("All")
	click("Apply date filter")
	if a.repo().logDate.preset != "" {
		t.Fatal("All did not clear the date filter")
	}
}
