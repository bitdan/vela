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

func TestImportedRenameWholeFileSelectionAndDirtySource(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.WorktreeVersion = "disk"
	a.Model.Active().Repository = r
	text := "diff --git a/old.txt b/new.txt\nsimilarity index 100%\nrename from old.txt\nrename to new.txt\n"
	doc, err := application.ParseImportedPatch(text)
	if err != nil {
		t.Fatal(err)
	}
	a.PickPatchSource = func(done func(string, error)) { done(filepath.Join(t.TempDir(), "rename.patch"), nil) }
	a.PreviewImportedPatch = func(_ context.Context, _ domain.Repository, _ string, target string, done func(domain.ImportedPatch, error)) {
		doc.Target, doc.Version = target, "preview"
		done(doc, nil)
	}
	a.ReviewImportedPatch = func(_ context.Context, _ domain.Repository, input domain.ImportedPatch, done func(domain.ImportedPatch, error)) {
		input.Version = "checked"
		done(input, nil)
	}
	a.ApplyImportedPatch = func(context.Context, domain.Repository, domain.ImportedPatch, func(domain.Repository, error, error)) {
		t.Fatal("dirty source was overwritten")
	}
	a.run(importPatch, commandTarget{})
	if a.patchImport == nil || len(a.patchImport.chosen["new.txt"]) != 1 || len(a.patchImport.rows) != 1 {
		t.Fatal("pure rename needs one whole-file choice")
	}
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click(commandLabel(clearImportedSelection)); err != nil {
		t.Fatal(err)
	}
	if a.patchImport.doc.Text != "" {
		t.Fatal("empty selection retained rename")
	}
	if err := tt.Click(commandLabel(selectImportedAll)); err != nil {
		t.Fatal(err)
	}
	if a.patchImport.doc.Text != text {
		t.Fatal("reselect lost pure rename")
	}
	if err := tt.Click("Preview Target"); err != nil {
		t.Fatal(err)
	}
	a.patchImport.ack = true
	a.openCode("project", domain.File{Path: "old.txt", New: "before\n"})
	a.repo().tabs[a.repo().active].code.Buffer.Set("unsaved\n")
	if a.patchImportDisabled(applyImportedPatch) == "" {
		t.Fatal("rename source missing from unsaved-document protection")
	}
}

func TestImportedStructuralScreenshots(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		a, r := historicalWorkspace(t)
		r.WorktreeVersion = "disk"
		a.Model.Active().Repository = r
		a.Layout.Dark = theme == "dark"
		a.Layout.Appearance.FollowSystem = false
		text := "diff --git a/new.txt b/new.txt\nnew file mode 100644\nindex 0000000..1234567\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1 @@\n+new\n" +
			"diff --git a/deleted.txt b/deleted.txt\ndeleted file mode 100644\nindex 1234567..0000000\n--- a/deleted.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-old\n" +
			"diff --git a/original.txt b/renamed.txt\nsimilarity index 100%\nrename from original.txt\nrename to renamed.txt\n"
		doc, err := application.ParseImportedPatch(text)
		if err != nil {
			t.Fatal(err)
		}
		a.PickPatchSource = func(done func(string, error)) { done(filepath.Join(t.TempDir(), "structural.patch"), nil) }
		a.PreviewImportedPatch = func(_ context.Context, _ domain.Repository, _ string, target string, done func(domain.ImportedPatch, error)) {
			doc.Target, doc.Version = target, "preview"
			done(doc, nil)
		}
		a.ReviewImportedPatch = func(_ context.Context, _ domain.Repository, input domain.ImportedPatch, done func(domain.ImportedPatch, error)) {
			input.Version = "checked"
			done(input, nil)
		}
		a.ApplyImportedPatch = func(context.Context, domain.Repository, domain.ImportedPatch, func(domain.Repository, error, error)) {
		}
		a.run(importPatch, commandTarget{})
		tt := native.NewTester(a.View, 1440, 900)
		if len(a.patchImport.rows) != 3 || len(a.patchImport.doc.Files) != 3 || len(a.patchImport.doc.Paths) != 4 {
			t.Fatal("structural file count or rename endpoints lost")
		}
		if box, ok := tt.Find("Imported patch file and hunk selection"); !ok || box.H < 60 {
			t.Fatal("structural choices are not visible")
		}
		if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			writeScreenshot(t, tt, filepath.Join(dir, theme+"-structural-patch.png"))
		}
		if err := tt.Click(commandLabel(toggleImportedReverse)); err != nil {
			t.Fatal(err)
		}
		if !a.patchImport.reverse || a.patchImport.doc.Version != "" || a.patchImport.ack {
			t.Fatal("reverse reused stale confirmation")
		}
		if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
			writeScreenshot(t, tt, filepath.Join(dir, theme+"-structural-patch-reverse.png"))
		}
	}
}
