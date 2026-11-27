package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func shelfHunkWorkspace(t *testing.T) *Workspace {
	t.Helper()
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.WorktreeVersion = "work"
	repo.Working = nil
	repo.Staged = nil
	repo.Shelf = []domain.Shelf{{ID: strings.Repeat("a", 32), OID: strings.Repeat("b", 40), Title: "Saved hunks", Files: []domain.File{{Path: "文件 name.txt", Status: "M"}, {Path: "other.txt", Status: "M"}}}}
	a.Model.Switch(a.installRepository(repo))
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	return a
}

func TestShelfHunksNativeSelection(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := shelfHunkWorkspace(t)
			a.Layout.Dark = theme == "dark"
			a.ReadRepositoryPatch = func(_ context.Context, _ domain.Repository, source, path string, done func(domain.PatchDocument, error)) {
				if source != "shelf:"+strings.Repeat("a", 32) {
					t.Fatal("wrong Shelf source")
				}
				done(domain.PatchDocument{Path: path, Source: source, Version: "patch-v", Hunks: []domain.PatchHunk{{Text: "@@ -1 +1 @@\n-old\n+chosen\n"}, {Text: "@@ -20 +20 @@\n-old\n+unchecked\n"}}}, nil)
			}
			var op domain.RepositoryOperation
			a.OperateRepository = func(_ context.Context, repo domain.Repository, input domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				op = input
				done(repo, nil, nil)
			}
			a.run(unshelve, commandTarget{index: 0})
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Hunks 文件 name.txt"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Unshelve hunk 文件 name.txt #2"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("-old → +chosen") {
				t.Fatal("Hunk preview omits the changed text")
			}
			s := a.repositoryOperation
			if len(s.op.ShelfHunks) != 1 || len(s.op.ShelfHunks[0].Hunks) != 1 || s.op.ShelfHunks[0].Hunks[0] != 0 || s.op.RestoreIndex || s.op.RemoveApplied {
				t.Fatalf("wrong Hunk request: %+v", s.op)
			}
			if state, ok := tt.Access("Restore original staged state"); !ok || !state.Disabled {
				t.Fatal("Hunk restore permits original staged state")
			}
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				writeScreenshot(t, tt, filepath.Join(dir, theme+"-shelf-hunks.png"))
			}
			if err := tt.Click("Run Unshelve"); err != nil {
				t.Fatal(err)
			}
			if len(op.Paths) != 2 || len(op.ShelfHunks) != 1 || op.ShelfHunks[0].Version != "patch-v" {
				t.Fatal("mixed selection did not reach shared operation port")
			}
		})
	}
}

func TestShelfHunkPreviewCancellationAndLateResult(t *testing.T) {
	a := shelfHunkWorkspace(t)
	var ctx context.Context
	var finish func(domain.PatchDocument, error)
	a.ReadRepositoryPatch = func(c context.Context, _ domain.Repository, _, _ string, done func(domain.PatchDocument, error)) {
		ctx = c
		finish = done
	}
	a.run(unshelve, commandTarget{index: 0})
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Hunks 文件 name.txt"); err != nil {
		t.Fatal(err)
	}
	if ctx == nil || finish == nil {
		t.Fatal("no preview request")
	}
	if state, ok := tt.Access("Run Unshelve"); !ok || !state.Disabled {
		t.Fatal("restore enabled during preview")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.Canceled {
		t.Fatal("close did not cancel Hunk preview")
	}
	finish(domain.PatchDocument{Path: "文件 name.txt", Version: "late"}, nil)
	tt.Frame()
	if a.repositoryOperation != nil {
		t.Fatal("late preview reopened closed dialog")
	}
}

func TestShelfHunkChoicesSurviveRevisitingFile(t *testing.T) {
	a := shelfHunkWorkspace(t)
	reads := 0
	a.ReadRepositoryPatch = func(_ context.Context, _ domain.Repository, source, path string, done func(domain.PatchDocument, error)) {
		reads++
		done(domain.PatchDocument{Path: path, Source: source, Version: "v", Hunks: []domain.PatchHunk{{Text: "@@ -1 +1 @@\n-a\n+b\n"}, {Text: "@@ -20 +20 @@\n-c\n+d\n"}}}, nil)
	}
	a.run(unshelve, commandTarget{index: 0})
	tt := native.NewTester(a.View, 1440, 900)
	for _, label := range []string{"Hunks 文件 name.txt", "Unshelve hunk 文件 name.txt #2", "Hunks other.txt", "Hunks 文件 name.txt"} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	s := a.repositoryOperation
	var chosen []int
	for _, selection := range s.op.ShelfHunks {
		if selection.Path == "文件 name.txt" {
			chosen = selection.Hunks
		}
	}
	if reads != 2 || len(chosen) != 1 || chosen[0] != 0 {
		t.Fatalf("revisiting expanded selection: reads=%d chosen=%v", reads, chosen)
	}
}

func TestShelfHunkPreviewCanceledByRepositoryNavigation(t *testing.T) {
	for _, load := range []bool{false, true} {
		t.Run(map[bool]string{false: "select", true: "load"}[load], func(t *testing.T) {
			a := shelfHunkWorkspace(t)
			var ctx context.Context
			var finish func(domain.PatchDocument, error)
			a.ReadRepositoryPatch = func(c context.Context, _ domain.Repository, _, _ string, done func(domain.PatchDocument, error)) {
				ctx = c
				finish = done
			}
			a.run(unshelve, commandTarget{index: 0})
			s := a.repositoryOperation
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Hunks 文件 name.txt"); err != nil {
				t.Fatal(err)
			}
			if load {
				a.ReadRepository = func(context.Context, string, func(domain.Repository, error)) {}
				a.loadRepository(t.TempDir(), false)
				defer stopRead(&a.repositoryRead)
			} else {
				a.selectRepository(0)
			}
			if ctx.Err() != context.Canceled || s.shelfHunkRead != nil {
				t.Fatal("repository navigation retained preview request")
			}
			finish(domain.PatchDocument{Path: "文件 name.txt", Source: "shelf:" + s.op.Target, Version: "late", Hunks: []domain.PatchHunk{{Text: "late"}}}, nil)
			if s.shelfHunkRead != nil || len(s.op.ShelfHunks) != 0 {
				t.Fatal("old callback installed a late selection")
			}
		})
	}
}
