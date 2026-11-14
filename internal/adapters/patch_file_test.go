package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestSaveNewPatchNeverOverwritesAndCleansOwnedTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "changes.patch")
	text := "diff --git a/file b/file\n--- a/file\n+++ b/file\n@@ -1 +1 @@\n-before\n+after\n"
	writer := PatchFiles{}
	if err := writer.SaveNewPatch(context.Background(), domain.Repository{}, path, text); err != nil {
		t.Fatal(err)
	}
	if err := writer.SaveNewPatch(context.Background(), domain.Repository{}, path, text+"extra"); err == nil {
		t.Fatal("existing patch overwritten")
	}
	actual, _ := os.ReadFile(path)
	if string(actual) != text {
		t.Fatal("patch content changed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".vela-patch-") {
			t.Fatal("owned temporary patch leaked")
		}
	}
}

func TestSaveNewPatchCancellationAndMetadataProtection(t *testing.T) {
	dir := t.TempDir()
	writer := PatchFiles{}
	text := "diff --git a/file b/file\n"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	target := filepath.Join(dir, "canceled.patch")
	if err := writer.SaveNewPatch(ctx, domain.Repository{}, target, text); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancellation published patch")
	}
	metadata := filepath.Join(dir, "custom-metadata")
	if err := os.Mkdir(metadata, 0700); err != nil {
		t.Fatal(err)
	}
	repo := domain.Repository{GitDir: metadata, CommonDir: metadata}
	if err := writer.SaveNewPatch(context.Background(), repo, filepath.Join(metadata, "new.patch"), text); err == nil {
		t.Fatal("custom Git metadata modified")
	}
	if err := writer.SaveNewPatch(context.Background(), repo, "relative.patch", text); err == nil {
		t.Fatal("relative destination accepted")
	}
	if err := writer.SaveNewPatch(context.Background(), repo, filepath.Join(dir, "bad.patch"), "not a patch"); err == nil {
		t.Fatal("invalid patch artifact accepted")
	}
}
