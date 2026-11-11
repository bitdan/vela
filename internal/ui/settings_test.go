package ui

import (
	"os"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
)

func TestAppearanceAndEditorPreferences(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := tt.Find("More actions"); ok {
		t.Fatal("obsolete toolbar button is still present")
	}
	click("Settings")
	click("Settings Appearance")
	click("Theme Light")
	if selected, ok := tt.Access("Theme Light"); !ok || !selected.Checked {
		t.Fatal("the selected theme segment is not exposed")
	}
	if len(settingsFonts()) != 0 {
		click("Select UI font")
		tt.Key(0, native.KeyDown)
		tt.Key(0, native.KeyEnter)
		if a.settings.appearance.Font == application.SystemFont {
			t.Fatal("keyboard selection did not update the font")
		}
		click("Select UI font")
		tt.Key(0, native.KeyHome)
		tt.Key(0, native.KeyEnter)
		if a.settings.appearance.Font != application.SystemFont {
			t.Fatal("could not restore the default font")
		}
	}
	click("Increase UI font size")
	click("Increase UI scale")
	if !a.Layout.Dark {
		t.Fatal("draft theme applied before confirmation")
	}
	click("Apply")
	if a.Layout.Dark || a.Layout.Appearance.FontSize != 14 || a.Layout.Appearance.Scale != 105 {
		t.Fatal("light theme did not apply")
	}
	click("Theme System")
	click("Apply")
	tt.SetDark(true)
	if !a.darkTheme() {
		t.Fatal("System theme did not follow dark mode")
	}
	tt.SetDark(false)
	if a.darkTheme() {
		t.Fatal("System theme did not follow light mode")
	}
	click("Settings Editor")
	click("Increase Editor font size")
	click("Increase Tab width")
	click("Apply")
	if diff := a.activeDocument().diff; diff.Font.Size != 14 || diff.TabSize != 5 {
		t.Fatalf("editor settings did not reach the diff: font=%v tab width=%d", diff.Font.Size, diff.TabSize)
	}
	click("Settings About")
	if !tt.HasText("Apache-2.0") {
		t.Fatal("About is missing the license")
	}
	click("Settings Editor")
	click("Increase Tab width")
	click("Close settings")
	if a.Layout.Editor.TabSize != 5 {
		t.Fatal("closing settings applied an unconfirmed editor change")
	}
}

func TestSettingsNavigationAndShortcutHelp(t *testing.T) {
	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 900)
	for _, label := range []string{"Settings", "Settings File Status Colors"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	shortcuts, hasShortcuts := tt.Find("Shortcuts")
	group, hasGroup := tt.Find("Version Control")
	child, hasChild := tt.Find("Changelists")
	if !hasShortcuts || !hasGroup || !hasChild || shortcuts.X != group.X || child.X <= group.X {
		t.Fatalf("expected aligned top-level labels and an indented child, got x=%g, %g, %g", shortcuts.X, group.X, child.X)
	}
	row, ok := tt.Access("Settings File Status Colors")
	if !ok || !row.Selected {
		t.Fatal("settings selection is not exposed to accessibility")
	}
	if row.Bounds.W != settingsSidebar {
		t.Fatalf("selection width = %g, want %d", row.Bounds.W, settingsSidebar)
	}
	if err := tt.Click("Search settings"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Shortcuts")
	if err := tt.Click("Settings Shortcuts"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Windows Shortcuts", "General", "Show All Commands", "Ctrl+Alt+S", "Ctrl+Shift+P", "Ctrl+Alt+Shift+C", "Alt+0"} {
		if !tt.HasText(label) {
			t.Fatalf("shortcut help missing %q", label)
		}
	}
}

func TestSettingsVersionControlDisclosure(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	check := func(expanded bool, child string) {
		t.Helper()
		header, ok := tt.Access("Settings Version Control")
		if !ok || header.Role != native.RoleDisclosure || header.Expanded != expanded {
			t.Fatalf("Version Control disclosure expanded = %v, want %v (present: %v)", header.Expanded, expanded, ok)
		}
		if _, found := tt.Access("Settings " + child); found != expanded {
			t.Fatalf("child %q visible = %v, want %v", child, found, expanded)
		}
	}
	click("Settings")
	click("Settings Log")
	check(true, "Log")
	click("Settings Version Control")
	check(false, "Log")
	if !tt.HasText("Version Control  ›  Log") {
		t.Fatal("collapsing the group changed the active settings page")
	}
	tt.Key(0, native.KeyEnter)
	check(true, "Log")
	click("Settings Version Control")
	click("Search settings")
	tt.Type("Shelf")
	check(true, "Shelf")
	if _, found := tt.Access("Settings Log"); found {
		t.Fatal("search showed an unrelated settings page")
	}
	click("Settings Version Control")
	check(false, "Shelf")
	click("Search settings")
	tt.Key(native.Cmd, native.KeyA)
	tt.Type("Log")
	check(true, "Log")
	tt.Key(native.Cmd, native.KeyA)
	tt.Key(0, native.KeyBackspace)
	check(false, "Log")
}

func TestStatusColorsMatchLightReference(t *testing.T) {
	p := design(false)
	var colors application.FileColors
	for status, want := range map[string]string{"A": "#067D17", "M": "#0033B3", "D": "#6C707E", "I": "#8C4F00", "R": "#0033B3", "U": "#DE1B2E"} {
		if got := fileStatusColor(p, &colors, status); got != native.Hex(want) {
			t.Errorf("status %s: got %v, want %s", status, got, want)
		}
	}
	colors.Modified = "#123456"
	if got := fileStatusColor(p, &colors, "M"); got != native.Hex("#123456") {
		t.Fatal("custom color was replaced by the default palette")
	}

	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	tt := native.NewTester(a.View, 1440, 900)
	for _, label := range []string{"Settings", "Settings File Status Colors", "Color Merged with text and property conflicts"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if !tt.HasText("Merged with text and property conflicts status color") {
		t.Fatal("missing reference status entry")
	}
}

func TestStatusColorSettingsValidateBeforeApply(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	for _, label := range []string{"Settings", "Settings File Status Colors", "Status color hex"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	tt.Type("bad")
	if button, ok := tt.Access("Apply"); !ok || !button.Disabled {
		t.Fatal("invalid color can be applied")
	}
	tt.Key(native.Cmd, native.KeyA)
	tt.Type("#12AB34")
	if err := tt.Click("Apply"); err != nil {
		t.Fatal(err)
	}
	if a.Layout.VersionControl.Colors.Added != "#12AB34" {
		t.Fatal("valid color did not apply")
	}
}

func TestSettingsScreenshots(t *testing.T) {
	dir := os.Getenv("VELA_SCREENSHOTS")
	if dir == "" {
		t.Skip("set VELA_SCREENSHOTS to export PNGs")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		layout := application.DefaultLayout()
		layout.Dark = theme == "dark"
		a := New(application.New(), layout)
		tt := native.NewTester(a.View, 1440, 900)
		if err := tt.Click("Settings"); err != nil {
			t.Fatal(err)
		}
		for _, item := range []struct{ page, slug string }{
			{appearancePage, "appearance"}, {editorPage, "editor"}, {aboutPage, "about"},
			{"Changelists", "changelists"}, {"Commit", "commit"}, {"Confirmation", "confirmation"},
			{"Directory Mappings", "mappings"}, {"File Status Colors", "colors"}, {"Log", "log"},
			{"Shelf", "shelf"}, {"Git", "git"}, {shortcutHelpPage, "shortcuts"},
		} {
			if err := tt.Click("Settings " + item.page); err != nil {
				t.Fatal(err)
			}
			writeScreenshot(t, tt, filepath.Join(dir, theme+"-settings-"+item.slug+".png"))
		}
		if err := tt.Click("Settings Version Control"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, theme+"-settings-collapsed.png"))
	}
}

func TestVersionControlSettingsApplyCancel(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click("Settings")
	if !tt.HasText("Version Control") {
		t.Fatal("missing settings navigation")
	}
	click("Settings Log")
	click("Show Hash column")
	if a.repo().logView.showHash {
		t.Fatal("draft leaked into the log")
	}
	click("Cancel")
	if a.repo().logView.showHash {
		t.Fatal("Cancel applied draft")
	}
	click("Settings")
	click("Settings Log")
	click("Show Hash column")
	click("Apply")
	if !a.repo().logView.showHash || !tt.HasText("Version Control") {
		t.Fatal("Apply did not update the log in place")
	}
	click("OK")
	if tt.HasText("Version Control") {
		t.Fatal("OK did not close settings")
	}
	b := New(application.New(), a.Layout)
	if !b.repo().logView.showHash {
		t.Fatal("saved preference was not used to initialize the workspace")
	}
}
