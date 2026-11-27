package ui

import (
	"context"
	"slices"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestShelveSilentlyNativeDispatch(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			repo := sessionRepository(t.TempDir())
			repo.IndexVersion = "index"
			selected := domain.File{Path: "selected.txt", Status: "M"}
			repo.Working = []domain.File{selected, {Path: "other.txt", Status: "M"}}
			a.Model.Switch(a.installRepository(repo))
			id, err := a.Model.Active().Changelists.Create("Ready for review")
			if err != nil {
				t.Fatal(err)
			}
			if err := a.Model.Active().Changelists.Move([]string{selected.Path}, id); err != nil {
				t.Fatal(err)
			}
			a.Model.Active().Working = selected.Path
			var request context.Context
			var complete func(domain.Repository, error, error)
			calls := 0
			a.OperateRepository = func(ctx context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				calls++
				if op.Kind != domain.SaveShelf || op.Message != "Ready for review" || !slices.Equal(op.Paths, []string{selected.Path}) {
					t.Fatalf("unexpected silent operation: %+v", op)
				}
				request, complete = ctx, done
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click(selected.Path); err != nil {
				t.Fatal(err)
			}
			if theme == "dark" {
				if err := tt.Click("Shelve Silently"); err != nil {
					t.Fatal(err)
				}
			} else {
				tt.Key(native.Ctrl|native.Shift, native.KeyH)
			}
			if calls != 1 || complete == nil || a.repositoryOperation != nil || !strings.Contains(a.status, "Shelve Silently") {
				t.Fatal("did not immediately start a silent shelf task")
			}
			exportSessionScreenshot(t, tt, "shelve-silently-"+theme)
			a.repositoryWriteCancel()
			if request.Err() != context.Canceled {
				t.Fatal("silent shelf task could not be canceled")
			}
			complete(repo, context.Canceled, nil)
			if calls != 1 || a.repositoryWrite != "" || !strings.Contains(a.status, "Git did not confirm") {
				t.Fatal("silent shelf cancellation was not reported")
			}
			a.run(shelve, commandTarget{file: selected, source: "working"})
			if a.repositoryOperation == nil || calls != 1 {
				t.Fatal("ordinary Shelve lost its naming dialog")
			}
		})
	}
}

func TestShelveSilentlyProtectsRenameSourceBuffer(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	f := domain.File{Path: "new name.txt", Status: "M"}
	repo.Working = []domain.File{f}
	repo.Staged = []domain.File{{Path: f.Path, OldPath: "old name.txt", Status: "R"}}
	a.Model.Switch(a.installRepository(repo))
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		t.Fatal("shelf began with the rename source editor dirty")
	}
	a.openCode("project", domain.File{Path: "old name.txt", New: "base", DiskVersion: "disk"})
	a.activeDocument().code.Buffer.Set("unsaved")
	target := commandTarget{source: "working", file: f}
	for _, id := range []commandID{shelveSilently, shelve} {
		if reason := a.disabledFor(id, target); !strings.Contains(reason, "unsaved") {
			t.Fatalf("dirty rename source did not disable %s: %q", id, reason)
		}
	}
	if got := a.shelfSelection(target); !slices.Equal(got, []string{"new name.txt", "old name.txt"}) {
		t.Fatalf("shelf and editor refresh would miss a rename endpoint: %v", got)
	}
}
