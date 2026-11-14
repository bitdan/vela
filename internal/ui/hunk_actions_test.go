package ui

import (
	"context"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestHunkDialogSelectConfirmAndCancel(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			repo := sessionRepository(t.TempDir())
			repo.IndexVersion = "index"
			repo.Working = []domain.File{{Path: "file.txt", Status: "M"}}
			a.Model.Switch(a.installRepository(repo))
			a.ReadRepositoryPatch = func(ctx context.Context, r domain.Repository, source, path string, done func(domain.PatchDocument, error)) {
				done(domain.PatchDocument{Path: path, Source: source, Version: "patch-version", Header: "diff --git a/file.txt b/file.txt\n", Hunks: []domain.PatchHunk{{Text: "@@ -1 +1 @@\n-before\n+after\n", OldStart: 1, OldCount: 1, NewStart: 1, NewCount: 1}, {Text: "@@ -20 +20 @@\n-old\n+new\n", OldStart: 20, OldCount: 1, NewStart: 20, NewCount: 1}}}, nil)
			}
			var submitted domain.RepositoryOperation
			calls := 0
			a.OperateRepository = func(ctx context.Context, r domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				calls++
				submitted = op
				done(repo, nil, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			a.run(stageHunk, commandTarget{source: "working", file: repo.Working[0]})
			tt.Frame()
			if a.patchDialog == nil {
				t.Fatal("hunk command unavailable")
			}
			click("Git hunk")
			click("2 · -20,1 +20,1")
			tt.Frame()
			if a.patchDialog.preview.Buffer.String() != "diff --git a/file.txt b/file.txt\n@@ -20 +20 @@\n-old\n+new\n" {
				t.Fatal("wrong Git hunk preview")
			}
			exportSessionScreenshot(t, tt, "hunk-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Run Stage Hunk")
			tt.Frame()
			if submitted.Kind != domain.StageHunk || submitted.HunkIndex != 1 || submitted.PatchVersion != "patch-version" || submitted.Path != "file.txt" {
				t.Fatalf("wrong partial stage %+v", submitted)
			}
			a.run(discardHunk, commandTarget{source: "working", file: repo.Working[0]})
			tt.Frame()
			click("Run Discard Hunk")
			if calls != 1 {
				t.Fatal("discard did not require acknowledgement")
			}
			click("Cancel")
			tt.Frame()
			if calls != 1 {
				t.Fatal("cancel wrote patch")
			}
			a.run(discardHunk, commandTarget{source: "working", file: repo.Working[0]})
			tt.Frame()
			click("Discard selected working changes permanently")
			click("Run Discard Hunk")
			tt.Frame()
			if calls != 2 || submitted.Kind != domain.DiscardHunk {
				t.Fatal("acknowledged discard was not dispatched")
			}
		})
	}
}

func TestCanceledPatchReadCannotReopenDialog(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Working = []domain.File{{Path: "file.txt", Status: "M"}}
	a.Model.Switch(a.installRepository(repo))
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	var finish func(domain.PatchDocument, error)
	var request context.Context
	a.ReadRepositoryPatch = func(ctx context.Context, r domain.Repository, source, path string, done func(domain.PatchDocument, error)) {
		request = ctx
		finish = done
	}
	a.run(stageHunk, commandTarget{source: "working", file: repo.Working[0]})
	tt := native.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	finish(domain.PatchDocument{Version: "late"}, nil)
	if request.Err() == nil || a.patchDialog != nil || a.repositoryWrite != "" {
		t.Fatal("late patch result survived cancellation")
	}
}
