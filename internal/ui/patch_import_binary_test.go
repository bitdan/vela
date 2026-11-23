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

const nativeBinaryPatch = "diff --git a/blob.bin b/blob.bin\nindex 5a4a005f23aa03302b132af8fada87e516e53ed6..1583869155b31c687aaaec3a6351d1fdc756d79c 100644\nGIT binary patch\ndelta 17\nYcmX>gcR+4J3S-1X!(_(Djfv%a06Po@AOHXW\n\ndelta 14\nVcmX>gcR+4J$|gn!zR3lQa{wzD1yTS2\n\n"

func TestImportedBinaryAndModeNativeSelection(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		a, r := historicalWorkspace(t)
		r.WorktreeVersion = "disk"
		a.Model.Active().Repository = r
		a.Layout.Dark = theme == "dark"
		a.Layout.Appearance.FollowSystem = false
		text := nativeBinaryPatch + "diff --git a/script.sh b/script.sh\nold mode 100644\nnew mode 100755\n"
		doc, err := application.ParseImportedPatch(text)
		if err != nil {
			t.Fatal(err)
		}
		a.PickPatchSource = func(done func(string, error)) { done(filepath.Join(t.TempDir(), "binary-mode.patch"), nil) }
		a.PreviewImportedPatch = func(_ context.Context, _ domain.Repository, _ string, target string, done func(domain.ImportedPatch, error)) {
			doc.Target = target
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
		if len(a.patchImport.rows) != 2 || len(a.patchImport.chosen["blob.bin"]) != 1 {
			t.Fatal("binary patch was exposed as text hunks")
		}
		if _, ok := tt.Find("blob.bin → blob.bin · Binary blob.bin (3904 → 3904 bytes; whole file)"); !ok {
			t.Fatal("binary operation or size summary missing")
		}
		if _, ok := tt.Find("script.sh → script.sh · Mode script.sh 100644 → 100755 (whole file)"); !ok {
			t.Fatal("mode operation summary missing")
		}
		if err := tt.Click("Patch application target"); err != nil {
			t.Fatal(err)
		}
		tt.Key(0, native.KeyDown)
		tt.Key(0, native.KeyEnter)
		if err := tt.Click("Preview Target"); err != nil {
			t.Fatal(err)
		}
		if a.patchImport.doc.Target != "Index only" || a.patchImport.doc.Text != text {
			t.Fatal("target change lost binary payload or permission metadata")
		}
		if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			writeScreenshot(t, tt, filepath.Join(dir, theme+"-binary-mode-patch.png"))
		}
		if err := tt.Click(commandLabel(toggleImportedReverse)); err != nil {
			t.Fatal(err)
		}
		if _, ok := tt.Find("script.sh → script.sh · Mode script.sh 100755 → 100644 (whole file)"); !ok {
			t.Fatal("reverse mode label not updated")
		}
		if a.patchImport.ack || a.patchImport.doc.Version != "" {
			t.Fatal("reverse retained stale permission to apply")
		}
	}
}
