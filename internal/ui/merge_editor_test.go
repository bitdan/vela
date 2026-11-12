package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestMergeEditorAppliesBlocksAndStagesEditedResult(t *testing.T) {
	conflict := domain.MergeConflict{Path: "config.txt", Current: "local", Incoming: "remote"}
	doc := domain.MergeDocument{Conflict: conflict, Head: "head", MergeHeads: []string{"remote"}, WorktreeVersion: "version", MarkerSize: 7,
		Current: "header\nlocal one\nshared\nlocal two\n", Incoming: "header\nremote one\nshared\nremote two\n",
		Result: "header\n<<<<<<< HEAD\nlocal one\n=======\nremote one\n>>>>>>> topic\nshared\n<<<<<<< HEAD\nlocal two\n=======\nremote two\n>>>>>>> topic\n"}
	repo := domain.Repository{Root: t.TempDir(), Name: "sample-repository", CurrentBranch: "main", Head: doc.Head,
		IndexVersion: "index", MergeHeads: doc.MergeHeads, Conflicts: []domain.MergeConflict{conflict},
		Working: []domain.File{{Path: conflict.Path, Status: "U"}}}
	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	a.Model.Switch(a.installRepository(repo))
	a.ReadRepositoryFile = func(context.Context, string, string, domain.File, func(domain.File, error)) {
		t.Fatal("opened the two-pane preview")
	}
	a.ReadMergeDocument = func(_ context.Context, _ domain.Repository, path string, done func(domain.MergeDocument, error)) {
		if path != conflict.Path {
			t.Fatalf("wrong merge file: %q", path)
		}
		done(doc, nil)
	}
	var applied domain.RepositoryOperation
	a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		applied = op
		snapshot.Conflicts, snapshot.Working = nil, nil
		done(snapshot, nil, nil)
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	click(conflict.Path)
	click(conflict.Path)
	draft := a.repo().mergeDrafts[conflict.Path]
	if draft == nil || !draft.current.ReadOnly || !draft.incoming.ReadOnly || draft.result.ReadOnly {
		t.Fatal("merge editor did not open with only the result editable")
	}
	if button, ok := tt.Access("Apply Changes"); !ok || !button.Disabled {
		t.Fatal("apply is available before the conflicts are resolved")
	}
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, "three-way-light.png"))
		a.Layout.Dark = true
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, "three-way-dark.png"))
	}
	tt.Key(0, native.KeyF7)
	if draft.selected != 1 {
		t.Fatal("F7 did not move to the next conflict")
	}
	click("Apply Incoming Block")
	click("Apply Current Block")
	click("Close Merge Editor")
	if a.repo().mergeActive != "" || !draft.result.Buffer.Modified() {
		t.Fatal("closing did not keep the draft")
	}
	if a.RequestClose() {
		t.Fatal("closing the workspace would discard a merge draft")
	}
	tt.Frame()
	if !tt.HasText("Discard unsaved edits?") {
		t.Fatal("workspace close did not warn about unsaved merge edits")
	}
	click("Cancel")
	click("Merge Drafts")
	if err := tt.ChooseMenuItem(conflict.Path); err != nil {
		t.Fatal(err)
	}
	tt.Key(native.Cmd, native.KeyHome)
	tt.Type("edited\n")
	tt.Frame()
	click("Apply Changes")
	if applied.Kind != domain.SaveMergeResult || applied.Result != "edited\nheader\nlocal one\nshared\nremote two\n" ||
		applied.WorktreeVersion != doc.WorktreeVersion || a.repo().mergeActive != "" {
		t.Fatalf("unexpected staged merge result: %+v", applied)
	}
}
