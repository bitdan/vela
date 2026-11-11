package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestWorkspaceControls(t *testing.T) {
	app := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(app.View, 1440, 900)
	button, ok := tt.Find("Toggle Git")
	if !ok || button.X >= activityWidth || button.Y+button.H < 860 {
		t.Fatalf("Git button is not at the bottom of the activity bar: %+v", button)
	}
	for _, label := range []string{"Working Changes", "Staged Changes", "Branches", "Changed Files", "Commit Details"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %s", label)
		}
	}
	if err := tt.Click("Toggle Git"); err != nil {
		t.Fatal(err)
	}
	if !app.Layout.HideGit || tt.HasText("Branches") {
		t.Fatal("Git stayed visible")
	}
	if err := tt.Click("Toggle Git"); err != nil {
		t.Fatal(err)
	}
	if app.Layout.HideGit || !tt.HasText("Branches") {
		t.Fatal("Git failed to restore")
	}
	chooseRepository(t, tt, "sandbox")
	if app.Model.Active().Repository.Name != "sandbox" {
		t.Fatal("repository did not switch")
	}
	if !tt.HasText("No working changes") {
		t.Fatal("missing empty state")
	}
}

func chooseRepository(t *testing.T, tt *native.Tester, name string) {
	t.Helper()
	if err := tt.Click("Switch repository"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(name); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceScreenshots(t *testing.T) {
	dir := os.Getenv("VELA_SCREENSHOTS")
	if dir == "" {
		t.Skip("set VELA_SCREENSHOTS to export PNGs")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dark", "light"} {
		layout := application.DefaultLayout()
		layout.Dark = name == "dark"
		app := New(application.New(), layout)
		tt := native.NewTester(app.View, 1440, 900)
		writeScreenshot(t, tt, filepath.Join(dir, name+".png"))
		if err := tt.Click("Main menu"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("View menu"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-main-menu.png"))
		if err := tt.Click("Git menu"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-git-menu.png"))
		if err := tt.ChooseMenuItem("Manage Remotes…"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-git-remotes.png"))
		if err := tt.Click("Cancel remotes"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Main menu"); err != nil {
			t.Fatal(err)
		}
		empty := native.NewTester(New(application.New(), layout).View, 1440, 900)
		for _, label := range []string{"Project", "Close main.go"} {
			if err := empty.Click(label); err != nil {
				t.Fatal(err)
			}
		}
		writeScreenshot(t, empty, filepath.Join(dir, name+"-empty-editor.png"))
		if err := tt.Click("Date"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Last 7 days"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-log-date.png"))
		if err := tt.Click("Cancel date filter"); err != nil {
			t.Fatal(err)
		}
		detailsLayout := layout
		detailsLayout.Top, detailsLayout.Files = 45, 40
		details := native.NewTester(New(application.New(), detailsLayout).View, 1440, 900)
		if err := details.Click("Show containing branches"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, details, filepath.Join(dir, name+"-commit-details.png"))
		if err := tt.Click("Search Everywhere"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-search-everywhere.png"))
		tt.Key(0, native.KeyEscape)
		if app.search.open {
			t.Fatal("Escape did not dismiss Search Everywhere")
		}
		if err := tt.Click("Current branch"); err != nil {
			t.Fatal(err)
		}
		if len(tt.Menu()) == 0 {
			t.Fatal("missing branch dropdown")
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-current-branch-open.png"))
		tt.CloseMenu()
		if err := tt.Click("feature/native-ui"); err != nil {
			t.Fatal(err)
		}
		if app.Model.Active().Branch != "feature/native-ui" {
			t.Fatal("branch row did not select")
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-branch-selection.png"))
		if err := tt.Click("internal/ui/theme.go"); err != nil {
			t.Fatal(err)
		}
		if !tt.HasText("Commit: theme.go") {
			t.Fatal("missing temporary diff tab")
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-changes-preview.png"))
		if err := tt.RightClick("Commit: theme.go"); err != nil {
			t.Fatal(err)
		}
		if len(tt.Menu()) == 0 {
			t.Fatal("missing temporary tab menu")
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-changes-preview-menu.png"))
		tt.CloseMenu()
		tt.Key(0, native.KeyEscape)
		if err := tt.RightClick("feature/native-ui"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-branch-menu.png"))
		tt.CloseMenu()
		if err := tt.Click("Historical go.mod"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Historical View Options"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-historical-toolbar.png"))
		tt.CloseMenu()
		for _, item := range []struct{ button, suffix string }{
			{"Log Sort", "-log-sort.png"}, {"Log View Options", "-log-view.png"},
		} {
			if err := tt.Click(item.button); err != nil {
				t.Fatal(err)
			}
			writeScreenshot(t, tt, filepath.Join(dir, name+item.suffix))
			tt.CloseMenu()
		}
		if err := tt.RightClick("Historical cmd/vela/main.go"); err != nil {
			t.Fatal(err)
		}
		if len(tt.Menu()) == 0 {
			t.Fatal("missing historical file menu")
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-historical-file-menu.png"))
		tt.CloseMenu()
		if err := tt.RightClick(app.Model.Active().Repository.History[0].Subject); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-commit-menu.png"))
		tt.CloseMenu()
		if err := tt.RightClick("main.go"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-tab-menu.png"))
		tt.CloseMenu()
		if err := tt.Click("View Options"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-commit-options.png"))
		tt.CloseMenu()
		if err := tt.RightClick("internal/ui/theme.go"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-changes-file-menu.png"))
		tt.CloseMenu()
		if err := tt.Click("View Options"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("Directory"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-change-folders.png"))
		if err := tt.Click("Project"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-project-folders.png"))
		if err := tt.RightClick("internal/ui/theme.go"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-project-file-menu.png"))
		tt.CloseMenu()
		if err := tt.Click("New Project Item"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-project-toolbar.png"))
		tt.CloseMenu()
		if err := tt.Click("Project"); err != nil {
			t.Fatal(err)
		}
		if !app.Layout.HideCommit {
			t.Fatal("Project did not collapse")
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-activity-collapsed.png"))
		if err := tt.Click("Commit tool window"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Shelf"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Expand UI baseline changes"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-shelf-tree.png"))
		if err := tt.RightClick("UI baseline changes"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-shelf-menu.png"))
	}
}

func writeScreenshot(t *testing.T, tt *native.Tester, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	err = png.Encode(f, tt.Image())
	closeErr := f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
}
