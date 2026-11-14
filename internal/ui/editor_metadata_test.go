package ui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	native "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestEditorBookmarksKeyboardGutterAndNavigation(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			a.openCode("project", domain.File{Path: "bookmarks.go", New: "first\nsecond\nthird\nfourth\n"})
			entry := a.activeDocument()
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Source editor"); err != nil {
				t.Fatal(err)
			}
			entry.code.GoTo(editor.Pos{Line: 1})
			tt.Key(0, native.KeyF11)
			if !reflect.DeepEqual(entry.document.bookmarks, []int{1}) {
				t.Fatal("F11 did not bookmark current line")
			}
			entry.code.OnGutterClick(3)
			tt.Frame()
			if len(entry.code.GutterMarks) != 2 {
				t.Fatal("native gutter marks missing")
			}
			entry.code.GoTo(editor.Pos{Line: 0})
			tt.Key(0, native.KeyF2)
			if entry.code.CursorPos().Line != 1 {
				t.Fatal("next bookmark failed")
			}
			tt.Key(0, native.KeyF2)
			if entry.code.CursorPos().Line != 3 {
				t.Fatal("second bookmark failed")
			}
			tt.Key(0, native.KeyF2)
			if entry.code.CursorPos().Line != 1 {
				t.Fatal("next bookmark did not wrap")
			}
			tt.Key(native.Shift, native.KeyF2)
			if entry.code.CursorPos().Line != 3 {
				t.Fatal("previous bookmark did not wrap")
			}
			if err := tt.RightClick("Source editor"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Bookmarks", "Clear Bookmarks"); err != nil {
				t.Fatal(err)
			}
			if len(entry.document.bookmarks) != 0 || entry.code.Buffer.Modified() {
				t.Fatal("clear changed text or retained bookmarks")
			}
		})
	}
}

func TestEditorMetadataSplitAndTargetIsolation(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.openCode("project", domain.File{Path: "one.go", New: "one\ntwo\nthree\n"})
	first := a.activeDocument()
	id, code := first.id, first.code
	a.splitTab(id, false, false)
	a.openCode("revision:fixed", domain.File{Path: "one.go", New: "old\n"})
	second := a.activeDocument()
	a.run(toggleBookmark, commandTarget{tabID: id, editorState: code, index: 1, source: "gutter"})
	if len(first.document.bookmarks) != 1 || len(second.document.bookmarks) != 0 {
		t.Fatal("bookmark target leaked into active historical tab")
	}
	if a.repo().split.document != first.document {
		t.Fatal("split does not share metadata")
	}
	a.run(fileType, commandTarget{tabID: id, editorState: code})
	a.fileTypeDialog.choice = "Plain Text"
	a.run(applyFileType, commandTarget{})
	if first.code.Buffer.Language() != nil || a.repo().split.code.Buffer.Language() != nil || second.code.Buffer.Language() == nil {
		t.Fatal("file type isolation failed")
	}
	if a.repo().active == a.repo().tabIndex(id) {
		t.Fatal("metadata action stole active tab")
	}
	a.run(selectRepo, commandTarget{index: 1})
	if a.disabledFor(nextBookmark, commandTarget{}) == "" {
		t.Fatal("bookmark navigation leaked into another repository")
	}
}

func TestEditorFileTypeApplyCancelAndStaleTarget(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.openCode("project", domain.File{Path: "example.go", New: "package main\n"})
	entry := a.activeDocument()
	entry.code.Buffer.Set("package edited\n")
	entry.code.Select(4, 4)
	a.run(fileType, commandTarget{})
	a.fileTypeDialog.choice = "JSON"
	a.run(cancelFileType, commandTarget{})
	if entry.code.Buffer.Language().Name != "Go" {
		t.Fatal("cancel applied draft")
	}
	a.run(fileType, commandTarget{})
	a.fileTypeDialog.choice = "JSON"
	a.run(applyFileType, commandTarget{})
	if entry.code.Buffer.Language().Name != "JSON" || entry.code.Cursor() != 4 || !entry.code.Buffer.Modified() || entry.code.Buffer.String() != "package edited\n" {
		t.Fatal("file type apply changed editing state")
	}
	a.run(fileType, commandTarget{})
	a.fileTypeDialog.choice = "Automatic"
	a.run(applyFileType, commandTarget{})
	if entry.code.Buffer.Language().Name != "Go" {
		t.Fatal("automatic file type failed")
	}
	a.run(fileType, commandTarget{})
	a.fileTypeDialog.choice = "JSON"
	a.run(selectRepo, commandTarget{index: 1})
	a.run(applyFileType, commandTarget{})
	if entry.code.Buffer.Language().Name != "Go" {
		t.Fatal("stale dialog changed old repository")
	}
}

func TestEditorMetadataRealSessionRestore(t *testing.T) {
	root := t.TempDir()
	a := New(application.New(), application.DefaultLayout())
	a.Model.Switch(a.installRepository(sessionRepository(root)))
	a.openCode("project", domain.File{Path: "你好 file.go", New: "one\ntwo\nthree\n", DiskVersion: "disk"})
	entry := a.activeDocument()
	entry.document.bookmarks, entry.document.fileType = []int{1, 2}, "Plain Text"
	snapshot := a.SessionSnapshot()
	for _, r := range snapshot.Repositories {
		if r.Root == root {
			r.Tabs[0].Bookmarks[0] = 0
		}
	}
	if entry.document.bookmarks[0] != 1 {
		t.Fatal("snapshot aliases live metadata")
	}
	b := New(application.New(), application.DefaultLayout())
	b.RestoreSession(snapshot)
	b.ReadRepository = func(_ context.Context, _ string, done func(domain.Repository, error)) {
		done(sessionRepository(root), nil)
	}
	b.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) {
		f.New = "one\ntwo\nthree\n"
		f.DiskVersion = "new"
		done(f, nil)
	}
	b.StartSessionRestore()
	restored := b.activeDocument()
	if restored == nil || !reflect.DeepEqual(restored.document.bookmarks, []int{0, 2}) || restored.code.Buffer.Language() != nil {
		t.Fatal("real session metadata not restored")
	}
}

func TestEditorMetadataPendingSnapshotDoesNotAlias(t *testing.T) {
	root := t.TempDir()
	snapshot := application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{{Root: root, Tabs: []application.TabSnapshot{{Source: "project", Path: "later.go", Code: true, Bookmarks: []int{1}}}}}}
	a := New(application.New(), application.DefaultLayout())
	a.RestoreSession(snapshot)
	snapshot.Repositories[0].Tabs[0].Bookmarks[0] = 8
	mutate := func(s application.WorkspaceSnapshot) {
		for _, r := range s.Repositories {
			if r.Root == root {
				if r.Tabs[0].Bookmarks[0] != 1 {
					t.Fatal("restore retained caller's bookmarks slice")
				}
				r.Tabs[0].Bookmarks[0] = 9
			}
		}
	}
	mutate(a.SessionSnapshot())
	mutate(a.SessionSnapshot())
	// A repository can also expose tabs awaiting an asynchronous read.
	a.savedRepositories = nil
	a.Model.Active().Repository.Root = root
	a.repo().tabs = nil
	a.repo().pendingTabs = []application.TabSnapshot{{Source: "project", Path: "later.go", Code: true, Bookmarks: []int{1}}}
	mutate(a.SessionSnapshot())
	mutate(a.SessionSnapshot())
}

func TestEditorMetadataNormalizesLegacyAndInvalidState(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.openCode("project", domain.File{Path: "a.go", New: "one\ntwo\n"})
	entry := a.activeDocument()
	a.restoreEditorMetadata(entry, application.TabSnapshot{Bookmarks: []int{1, 1, -1, 999}, FileType: "not-a-language"})
	if !reflect.DeepEqual(entry.document.bookmarks, []int{1}) || entry.code.Buffer.Language().Name != "Go" {
		t.Fatal("invalid metadata not normalized")
	}
	entry.code.Buffer.Set("short")
	tt := native.NewTester(a.View, 1440, 900)
	if len(entry.document.bookmarks) != 0 {
		t.Fatal("out-of-range bookmark retained after edit")
	}
	a.openDiff("working", domain.File{Path: "a.go"})
	tt.Frame()
	if a.disabledFor(toggleBookmark, commandTarget{}) == "" || a.disabledFor(fileType, commandTarget{}) == "" {
		t.Fatal("Diff supports source-only metadata")
	}
}

func TestEditorMetadataActionSearchTargetsSplit(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.openCode("project", domain.File{Path: "first.go", New: "one\ntwo\n"})
	id := a.activeDocument().id
	a.splitTab(id, false, false)
	a.openCode("project", domain.File{Path: "second.go", New: "other\n"})
	a.focusedEditor = a.repo().split.code
	a.repo().split.code.GoTo(editor.Pos{Line: 1})
	a.openSearch(searchActions)
	a.focusedEditor = nil
	a.chooseSearchResult(searchResult{kind: searchActions, action: toggleBookmark}, false)
	if !reflect.DeepEqual(a.repo().split.document.bookmarks, []int{1}) || len(a.activeDocument().document.bookmarks) != 0 {
		t.Fatal("action search lost split editor target")
	}
	a.repo().preview = new(tab)
	a.repo().preview.diff = &editor.DiffState{Old: editor.NewBuffer(""), New: editor.NewBuffer("")}
	if a.disabledFor(toggleBookmark, commandTarget{}) == "" {
		t.Fatal("command targeted hidden source behind Diff preview")
	}
}

func TestEditorMetadataActionsAreSearchableFromSplit(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.openCode("project", domain.File{Path: "split.go", New: "one\ntwo\n"})
	a.activeDocument().document.bookmarks = []int{1}
	a.splitTab(a.activeDocument().id, false, false)
	a.openDiff("working", domain.File{Path: "main.diff"})
	a.focusedEditor = a.repo().split.code
	a.openSearch(searchActions)
	a.focusedEditor = nil
	results := a.searchResults(searchActions, "Next Bookmark")
	if len(results) != 1 || results[0].action != nextBookmark {
		t.Fatal("metadata action not discoverable for focused split")
	}
	fileTypes := a.searchResults(searchActions, "Override File Type")
	if len(fileTypes) != 1 || fileTypes[0].action != fileType {
		t.Fatal("file type action not discoverable")
	}
	a.chooseSearchResult(results[0], false)
	if a.repo().split.code.CursorPos().Line != 1 || a.searchFocus != a.repo().split.code {
		t.Fatal("search action did not navigate focused split")
	}
}

func TestEditorMetadataRetainsOverrideThroughReload(t *testing.T) {
	a, _, file := sourceLifecycleFixture(t, true)
	entry := a.activeDocument()
	a.run(fileType, commandTarget{})
	a.fileTypeDialog.choice = "JSON"
	a.run(applyFileType, commandTarget{})
	a.run(toggleBookmark, commandTarget{})
	a.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) {
		f.New = "fresh\n"
		f.DiskVersion = "fresh"
		done(f, nil)
	}
	a.run(reloadProjectFile, commandTarget{tabID: entry.id})
	if entry.code.Buffer.Language() == nil || entry.code.Buffer.Language().Name != "JSON" || len(entry.document.bookmarks) != 1 {
		t.Fatal("reload lost document metadata")
	}
	file.New, file.DiskVersion = "updated\n", "updated"
	a.openCode("project", file)
	if entry.code.Buffer.Language().Name != "JSON" {
		t.Fatal("reopening clean file lost override")
	}
}

func TestEditorFileTypeDialogRejectsReopenedTarget(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	f := domain.File{Path: "same.go", New: "package main\n"}
	a.openCode("project", f)
	id := a.activeDocument().id
	a.run(fileType, commandTarget{})
	a.fileTypeDialog.choice = "JSON"
	a.closeTabs(closeTab, commandTarget{tabID: id})
	a.openCode("project", f)
	a.run(applyFileType, commandTarget{})
	if a.activeDocument().code.Buffer.Language().Name != "Go" || a.disabledFor(applyFileType, commandTarget{}) == "" {
		t.Fatal("stale dialog applied to a reopened tab")
	}
}

func TestEditorMetadataScreenshots(t *testing.T) {
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
		a := New(application.New(), layout)
		a.openCode("project", domain.File{Path: "document.go", New: "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"})
		a.activeDocument().document.bookmarks = []int{0, 3}
		tt := native.NewTester(a.View, 1440, 900)
		writeScreenshot(t, tt, filepath.Join(dir, name+"-bookmarks.png"))
		if err := tt.RightClick("Source editor"); err != nil {
			t.Fatal(err)
		}
		if err := tt.ChooseMenuItem("Override File Type"); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, name+"-file-type.png"))
		if err := tt.Click("Document file type"); err != nil {
			t.Fatal(err)
		}
		tt.Key(0, native.KeyHome)
		tt.Key(0, native.KeyDown)
		tt.Key(0, native.KeyEnter)
		if err := tt.Click("Apply File Type"); err != nil {
			t.Fatal(err)
		}
		if a.activeDocument().code.Buffer.Language() != nil {
			t.Fatal("native selector did not apply")
		}
	}
}
