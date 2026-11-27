package ui

import (
	"context"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestHunkIndividualLineSelection(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.IndexVersion = "index"
			r.Working = []domain.File{{Path: "file.txt", Status: "M"}}
			a.Model.Switch(a.installRepository(r))
			a.ReadRepositoryPatch = func(_ context.Context, _ domain.Repository, source, path string, done func(domain.PatchDocument, error)) {
				done(domain.PatchDocument{Path: path, Source: source, Version: "v1", Header: "diff --git a/file.txt b/file.txt\nindex aa..bb 100644\n--- a/file.txt\n+++ b/file.txt\n", Hunks: []domain.PatchHunk{{Text: "@@ -1,2 +1,2 @@\n-old A\n-old B\n+new A\n+new B\n", OldStart: 1, OldCount: 2, NewStart: 1, NewCount: 2}, {Text: "@@ -20 +20 @@\n-old\n+new\n", OldStart: 20, OldCount: 1, NewStart: 20, NewCount: 1}}}, nil)
			}
			var submitted domain.RepositoryOperation
			a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, _ func(domain.Repository, error, error)) {
				submitted = op
			}
			a.run(stageHunk, commandTarget{source: "working", file: r.Working[0]})
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Choose individual changed lines"); err != nil {
				t.Fatal(err)
			}
			for _, label := range []string{"Changed line 2: -old B", "Changed line 4: +new B"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			for _, label := range []string{"Git hunk", "2 · -20,1 +20,1", "Git hunk", "1 · -1,2 +1,2"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			if err := tt.Click("Clear Changed Lines"); err != nil {
				t.Fatal(err)
			}
			_ = tt.Click("Run Stage Hunk")
			if submitted.SelectPatchLines {
				t.Fatal("empty line selection widened to full hunk")
			}
			for _, label := range []string{"Changed line 1: -old A", "Changed line 3: +new A"} {
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			exportSessionScreenshot(t, tt, "line-stage-"+theme)
			if err := tt.Click("Run Stage Hunk"); err != nil {
				t.Fatal(err)
			}
			if !submitted.SelectPatchLines || len(submitted.PatchLines) != 2 || submitted.PatchLines[0] != 0 || submitted.PatchLines[1] != 2 {
				t.Fatalf("selection widened: %+v", submitted)
			}
		})
	}
}

func TestHunkLineReadCanceledOnRepositoryNavigation(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	r := sessionRepository(t.TempDir())
	r.IndexVersion = "v1"
	r.Working = []domain.File{{Path: "file.txt", Status: "M"}}
	a.Model.Switch(a.installRepository(r))
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	var request context.Context
	var finish func(domain.PatchDocument, error)
	a.ReadRepositoryPatch = func(ctx context.Context, _ domain.Repository, _, _ string, done func(domain.PatchDocument, error)) {
		request, finish = ctx, done
	}
	a.run(stageHunk, commandTarget{source: "working", file: r.Working[0]})
	a.run(selectRepo, commandTarget{index: 1})
	finish(domain.PatchDocument{Path: "file.txt", Source: "working", Version: "late", Hunks: []domain.PatchHunk{{Text: "@@ -1 +1 @@\n-a\n+b\n"}}}, nil)
	if request.Err() != context.Canceled || a.patchDialog != nil {
		t.Fatal("late hunk preview survived repository navigation")
	}
}
