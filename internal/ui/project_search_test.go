package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestProjectSearchNativeFindAndOpen(t *testing.T) {
	for _, dark := range []bool{true, false} {
		t.Run(map[bool]string{true: "dark", false: "light"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			a.openCode("project", projectFiles(a.Model.Active().Repository)[0])
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Source editor"); err != nil {
				t.Fatal(err)
			}
			tt.Key(native.Ctrl|native.Shift, native.KeyF)
			if a.projectSearch == nil {
				t.Fatal("Find in Files shortcut did not open dialog")
			}
			if err := tt.Click("Find in Files query"); err != nil {
				t.Fatal(err)
			}
			tt.Type("bootstrap")
			if err := tt.Click("Find Project Text"); err != nil {
				t.Fatal(err)
			}
			s := a.projectSearch
			if s.loading || len(s.result.Matches) < 2 {
				t.Fatalf("mock project matches missing: %+v", s)
			}
			expected := s.result.Matches[0]
			if err := tt.Click("Match case"); err != nil {
				t.Fatal(err)
			}
			if len(s.result.Matches) != 0 || !s.matchCase {
				t.Fatal("case change did not invalidate results")
			}
			if err := tt.Click("Find Project Text"); err != nil {
				t.Fatal(err)
			}
			expected = s.result.Matches[1]
			if err := tt.Click("Find in Files results"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, native.KeyDown)
			tt.Key(0, native.KeyEnter)
			entry := a.activeDocument()
			if a.projectSearch != nil || entry == nil || entry.path != expected.Path || entry.code.Cursor() != expected.End || entry.code.Buffer.Modified() {
				t.Fatal("result did not select unchanged source")
			}
		})
	}
}

func TestProjectSearchAsyncCancellationAndIsolation(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	root := t.TempDir()
	a.Model.Active().Repository.Root = root
	a.Model.Active().Repository.Project = []domain.File{{Path: "unopened.go"}, {Path: "dirty.go"}}
	a.openCode("project", domain.File{Path: "dirty.go", New: "disk"})
	a.activeDocument().code.Buffer.Set("unsaved needle")
	type call struct {
		ctx   context.Context
		input application.ProjectSearchInput
		done  func(application.ProjectSearchResult, error)
	}
	var calls []call
	a.SearchProjectFiles = func(ctx context.Context, input application.ProjectSearchInput, done func(application.ProjectSearchResult, error)) {
		calls = append(calls, call{ctx, input, done})
	}
	a.run(findInProject, commandTarget{})
	s := a.projectSearch
	s.query = "needle"
	a.run(startProjectSearch, commandTarget{})
	if len(calls) != 1 || calls[0].input.Overlays["dirty.go"] != "unsaved needle" || len(calls[0].input.Paths) != 2 {
		t.Fatal("search scope not captured")
	}
	s.query = "other"
	a.invalidateProjectSearch()
	if calls[0].ctx.Err() != context.Canceled {
		t.Fatal("editing did not cancel")
	}
	a.run(startProjectSearch, commandTarget{})
	calls[0].done(application.ProjectSearchResult{Matches: []application.ProjectSearchMatch{{Path: "old"}}}, nil)
	if len(s.result.Matches) != 0 || !s.loading {
		t.Fatal("old query overwrote new search")
	}
	a.run(cancelProjectSearch, commandTarget{})
	if calls[1].ctx.Err() != context.Canceled || s.loading {
		t.Fatal("cancel failed")
	}
	a.run(startProjectSearch, commandTarget{})
	a.run(selectRepo, commandTarget{index: 1})
	calls[2].done(application.ProjectSearchResult{Matches: []application.ProjectSearchMatch{{Path: "old"}}}, nil)
	if a.projectSearch != nil || a.Model.Index != 1 || calls[2].ctx.Err() != context.Canceled {
		t.Fatal("repository switch retained search")
	}
}

func TestProjectSearchDiskNavigationAndStaleResults(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Active().Repository.Root = t.TempDir()
	a.Model.Active().Repository.Project = []domain.File{{Path: "unopened.go"}}
	a.SearchProjectFiles = func(ctx context.Context, input application.ProjectSearchInput, done func(application.ProjectSearchResult, error)) {
		result, err := application.SearchProject(ctx, input, func(context.Context, string) (string, error) { return "first\r\n中文 needle\r\n", nil })
		done(result, err)
	}
	var callbacks []func(domain.File, error)
	a.ReadRepositoryFile = func(_ context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		if source != "project" || f.Path != "unopened.go" {
			t.Fatal("wrong file read")
		}
		callbacks = append(callbacks, done)
	}
	a.run(findInProject, commandTarget{})
	s := a.projectSearch
	s.query = "needle"
	a.run(startProjectSearch, commandTarget{})
	a.run(openProjectSearchResult, commandTarget{})
	callbacks[0](domain.File{Path: "unopened.go", New: "external edit needle"}, nil)
	if a.projectSearch != s || s.loading || !strings.Contains(s.message, "changed") {
		t.Fatal("stale result navigated")
	}
	a.run(openProjectSearchResult, commandTarget{})
	callbacks[1](domain.File{Path: "unopened.go", New: "first\r\n中文 needle\r\n", DiskVersion: "current"}, nil)
	entry := a.activeDocument()
	if a.projectSearch != nil || entry.path != "unopened.go" || entry.file.DiskVersion != "current" || entry.code.Cursor() != s.result.Matches[0].End || entry.code.CursorPos().Line != 1 {
		t.Fatal("disk match did not navigate with current file identity")
	}
}

func TestProjectSearchDirtySplitAndErrors(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Active().Repository.Root = t.TempDir()
	a.openCode("project", domain.File{Path: "dirty.go", New: "disk"})
	a.activeDocument().code.Buffer.Set("needle draft")
	a.splitTab(a.activeDocument().id, false, true)
	split := a.repo().split
	a.SearchProjectFiles = func(ctx context.Context, input application.ProjectSearchInput, done func(application.ProjectSearchResult, error)) {
		result, err := application.SearchProject(ctx, input, func(context.Context, string) (string, error) { return "", errors.New("missing") })
		done(result, err)
	}
	a.run(findInProject, commandTarget{})
	s := a.projectSearch
	s.query = "needle"
	a.run(startProjectSearch, commandTarget{})
	if len(s.result.Matches) != 1 || !s.result.Matches[0].InMemory {
		t.Fatal("moved split draft not searched")
	}
	a.run(openProjectSearchResult, commandTarget{})
	if a.projectSearch != nil || a.searchFocus != split.code || !split.code.Buffer.Modified() || split.code.Buffer.String() != "needle draft" {
		t.Fatal("dirty split was replaced or lost")
	}
	a.run(findInProject, commandTarget{})
	s = a.projectSearch
	s.query = "absent"
	a.run(startProjectSearch, commandTarget{})
	if a.disabledFor(openProjectSearchResult, commandTarget{}) == "" {
		t.Fatal("empty search allowed open")
	}
	a.RequestClose()
	if a.projectSearch != nil {
		t.Fatal("exit retained search")
	}
}

func TestProjectSearchScreenshots(t *testing.T) {
	dir := os.Getenv("VELA_SCREENSHOTS")
	if dir == "" {
		t.Skip("set VELA_SCREENSHOTS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dark", "light"} {
		layout := application.DefaultLayout()
		layout.Dark = name == "dark"
		a := New(application.New(), layout)
		a.run(findInProject, commandTarget{})
		a.projectSearch.query = "bootstrap"
		a.run(startProjectSearch, commandTarget{})
		tt := native.NewTester(a.View, 1440, 900)
		writeScreenshot(t, tt, filepath.Join(dir, name+"-find-in-files.png"))
	}
}

func TestProjectSearchNativeDiskScopeAndEscape(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"unopened.go", "not-in-project.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("中文 needle\r\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := New(application.New(), application.DefaultLayout())
	a.Model.Switch(a.installRepository(sessionRepository(root)))
	a.Model.Active().Repository.Project = []domain.File{{Path: "unopened.go"}}
	a.SearchProjectFiles = func(ctx context.Context, input application.ProjectSearchInput, done func(application.ProjectSearchResult, error)) {
		result, err := (adapters.ProjectFiles{}).SearchProject(ctx, input)
		done(result, err)
	}
	a.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) {
		text, err := os.ReadFile(filepath.Join(root, f.Path))
		f.New = string(text)
		f.DiskVersion = "read-from-disk"
		done(f, err)
	}
	a.run(findInProject, commandTarget{})
	a.openSearch(searchAll)
	if a.search.open {
		t.Fatal("quick search opened over Find in Files")
	}
	tt := native.NewTester(a.View, 1440, 900)
	tt.Type("needle")
	if err := tt.Click("Find Project Text"); err != nil {
		t.Fatal(err)
	}
	if len(a.projectSearch.result.Matches) != 1 || a.projectSearch.result.Scanned != 1 {
		t.Fatal("search did not respect full project snapshot")
	}
	if err := tt.Click("Open Search Result"); err != nil {
		t.Fatal(err)
	}
	if a.activeDocument().code.SelectedText() != "needle" || a.activeDocument().file.DiskVersion != "read-from-disk" {
		t.Fatal("unopened disk result not selected")
	}
	var requestContext context.Context
	a.SearchProjectFiles = func(ctx context.Context, _ application.ProjectSearchInput, _ func(application.ProjectSearchResult, error)) {
		requestContext = ctx
	}
	a.run(findInProject, commandTarget{})
	tt.Frame()
	tt.Type("needle")
	if err := tt.Click("Find Project Text"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyEscape)
	if a.projectSearch != nil || requestContext.Err() != context.Canceled {
		t.Fatal("Escape did not cancel and close search")
	}
}
