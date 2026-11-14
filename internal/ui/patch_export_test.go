package ui

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func exportTestDocument() domain.PatchExport {
	return domain.PatchExport{Source: "working", Revision: strings.Repeat("a", 40), Paths: []string{"file.txt"}, Text: "diff --git a/file.txt b/file.txt\nindex 123..456 100644\n--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-before\n+SECRET-CONTENT\n"}
}

func TestPatchCopyUsesSelectionAndDoesNotLogBody(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Working = []domain.File{{Path: "file.txt", Status: "M"}, {Path: "second.txt", Status: "M"}}
	a.Model.Switch(a.installRepository(repo))
	a.repo().workingChosen.Add("file.txt")
	a.repo().workingChosen.Add("second.txt")
	var selected int
	a.ReadRepositoryChangesPatch = func(ctx context.Context, r domain.Repository, source string, files []domain.File, done func(domain.PatchExport, error)) {
		selected = len(files)
		done(exportTestDocument(), nil)
	}
	a.run(copyPatch, commandTarget{source: "working", file: repo.Working[0]})
	if selected != 2 || a.clipboard == nil || *a.clipboard != exportTestDocument().Text {
		t.Fatal("patch copy did not use selected real changes")
	}
	tt := native.NewTester(a.View, 1440, 900)
	tt.Frame()
	if strings.Contains(a.status, "SECRET-CONTENT") {
		t.Fatal("patch body logged to status")
	}
}

func TestPatchExportPreviewSaveAndCancel(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			repo := sessionRepository(t.TempDir())
			repo.IndexVersion = "index"
			repo.Working = []domain.File{{Path: "file.txt", Status: "M"}}
			a.Model.Switch(a.installRepository(repo))
			a.ReadRepositoryChangesPatch = func(ctx context.Context, r domain.Repository, source string, files []domain.File, done func(domain.PatchExport, error)) {
				done(exportTestDocument(), nil)
			}
			calls := 0
			target := filepath.Join(t.TempDir(), "changes.patch")
			a.PickPatchDestination = func(done func(string, error)) { done(target, nil) }
			a.WritePatchFile = func(ctx context.Context, r domain.Repository, path, text string, done func(domain.Repository, error, error)) {
				calls++
				if path != target || text != exportTestDocument().Text {
					t.Fatal("export changed chosen artifact")
				}
				done(repo, nil, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			if err := tt.RightClick("file.txt"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Create Patch from Local Changes…"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.patchExport == nil || a.patchExport.preview == nil || !a.patchExport.preview.ReadOnly {
				t.Fatal("patch preview missing or editable")
			}
			exportSessionScreenshot(t, tt, "patch-export-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Save Patch File")
			tt.Frame()
			if calls != 1 || a.repositoryWrite != "" {
				t.Fatal("patch was not saved/refreshed")
			}
			a.run(createPatch, commandTarget{source: "working", file: repo.Working[0]})
			tt.Frame()
			click("Cancel")
			tt.Frame()
			if calls != 1 || a.patchExport != nil {
				t.Fatal("cancel exported patch")
			}
		})
	}
}

func TestPatchCopyLateResultAndExportFailure(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Working = []domain.File{{Path: "file.txt", Status: "M"}}
	a.Model.Switch(a.installRepository(repo))
	var finish func(domain.PatchExport, error)
	var request context.Context
	a.ReadRepositoryChangesPatch = func(ctx context.Context, r domain.Repository, source string, files []domain.File, done func(domain.PatchExport, error)) {
		request = ctx
		finish = done
	}
	a.run(copyPatch, commandTarget{source: "working", file: repo.Working[0]})
	a.run(selectRepo, commandTarget{index: 1})
	finish(exportTestDocument(), nil)
	if request.Err() == nil || a.clipboard != nil {
		t.Fatal("late copy crossed repository navigation")
	}
	a.Model.Switch(a.installRepository(repo))
	a.ReadRepositoryChangesPatch = func(ctx context.Context, r domain.Repository, source string, files []domain.File, done func(domain.PatchExport, error)) {
		done(exportTestDocument(), nil)
	}
	a.PickPatchDestination = func(done func(string, error)) { done(filepath.Join(t.TempDir(), "exists.patch"), nil) }
	a.WritePatchFile = func(ctx context.Context, r domain.Repository, path, text string, done func(domain.Repository, error, error)) {
		done(repo, errors.New("destination already exists"), nil)
	}
	a.run(createPatch, commandTarget{source: "working", file: repo.Working[0]})
	tt := native.NewTester(a.View, 1440, 900)
	tt.Frame()
	if err := tt.Click("Save Patch File"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.patchExport == nil || a.patchExport.message == "" || a.repositoryWrite != "" {
		t.Fatal("failed export lost retryable preview or kept task busy")
	}
}

func TestNewClipboardIntentCancelsPendingPatchCopy(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Working = []domain.File{{Path: "file.txt", Status: "M"}}
	a.Model.Switch(a.installRepository(repo))
	var finish func(domain.PatchExport, error)
	var request context.Context
	a.ReadRepositoryChangesPatch = func(ctx context.Context, r domain.Repository, source string, files []domain.File, done func(domain.PatchExport, error)) {
		request = ctx
		finish = done
	}
	a.run(copyPatch, commandTarget{source: "working", file: repo.Working[0]})
	a.run(copyHash, commandTarget{source: "new-copy"})
	finish(exportTestDocument(), nil)
	if request.Err() == nil || a.clipboard == nil || *a.clipboard != "new-copy" {
		t.Fatal("older patch copy replaced a newer clipboard intent")
	}
}
