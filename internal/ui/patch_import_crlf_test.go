package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestImportedCRLFNativeSelectionPreservesBytes(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a, r := historicalWorkspace(t)
			r.WorktreeVersion = "disk"
			a.Model.Active().Repository = r
			a.Layout.Dark = theme == "dark"
			header := "diff --git a/file.txt b/file.txt\nindex a..b 100644\n--- a/file.txt\n+++ b/file.txt\n"
			first := "@@ -1 +1 @@\n-old\r\n+selected CRLF\r\n"
			second := "@@ -20 +20 @@\n-old\r\n+unchecked CRLF\r\n"
			doc, err := application.ParseImportedPatch(header + first + second)
			if err != nil {
				t.Fatal(err)
			}
			a.PickPatchSource = func(done func(string, error)) { done(filepath.Join(t.TempDir(), "CRLF.patch"), nil) }
			a.PreviewImportedPatch = func(_ context.Context, _ domain.Repository, _ string, target string, done func(domain.ImportedPatch, error)) {
				doc.Target, doc.Version = target, "v1"
				done(doc, nil)
			}
			a.ReviewImportedPatch = func(_ context.Context, _ domain.Repository, input domain.ImportedPatch, done func(domain.ImportedPatch, error)) {
				input.Version = "v2"
				done(input, nil)
			}
			var applied string
			a.ApplyImportedPatch = func(_ context.Context, _ domain.Repository, input domain.ImportedPatch, _ func(domain.Repository, error, error)) {
				applied = input.Text
			}
			a.run(importPatch, commandTarget{})
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("@@ -20 +20 @@"); err != nil {
				t.Fatal(err)
			}
			if a.patchImport.doc.Text != header+first || a.patchImport.doc.Version != "" {
				t.Fatal("selection normalized raw CRLF or retained old preview")
			}
			if err := tt.Click("Preview Target"); err != nil {
				t.Fatal(err)
			}
			if err := tt.Click("Apply the checked selection to the mapped paths, direction and target"); err != nil {
				t.Fatal(err)
			}
			exportSessionScreenshot(t, tt, "crlf-import-"+theme)
			if err := tt.Click("Apply Patch"); err != nil {
				t.Fatal(err)
			}
			if applied != header+first || strings.Contains(applied, "unchecked") {
				t.Fatal("UI applied unselected or normalized bytes")
			}
		})
	}
}
