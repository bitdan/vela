package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"vela/internal/domain"
)

func TestSaveFilePreservesIndexAndRejectsExternalChanges(t *testing.T) {
	f := newWorkflow(t)
	repo := f.read()
	file, err := f.g.ReadFile(context.Background(), repo.Root, "project", domain.File{Path: "文件 name.txt"})
	if err != nil {
		t.Fatal(err)
	}
	head, index := f.git("rev-parse", "HEAD"), f.git("write-tree")
	saved, err := f.g.SaveFile(context.Background(), repo, file, "edited\r\n")
	if err != nil {
		t.Fatal(err)
	}
	if saved.DiskVersion == "" || saved.DiskVersion == file.DiskVersion {
		t.Fatal("disk identity not updated")
	}
	if content, err := os.ReadFile(filepath.Join(repo.Root, file.Path)); err != nil || string(content) != "edited\r\n" {
		t.Fatalf("wrong saved content %q %v", content, err)
	}
	if f.git("rev-parse", "HEAD") != head || f.git("write-tree") != index {
		t.Fatal("saving implicitly staged or committed")
	}
	f.write(file.Path, "external content\n")
	if _, err := f.g.SaveFile(context.Background(), repo, saved, "would overwrite external\n"); !errors.Is(err, ErrStaleFile) {
		t.Fatalf("external edit overwritten: %v", err)
	}
	if content, _ := os.ReadFile(filepath.Join(repo.Root, file.Path)); string(content) != "external content\n" {
		t.Fatal("external edit lost")
	}
}

func TestSaveFileRejectsUnsafeTargetsAndCancellation(t *testing.T) {
	f := newWorkflow(t)
	repo := f.read()
	file, err := f.g.ReadFile(context.Background(), repo.Root, "project", domain.File{Path: "文件 name.txt"})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"../escape", ".git/config"} {
		bad := file
		bad.Path = path
		if _, err := f.g.SaveFile(context.Background(), repo, bad, "bad"); err == nil {
			t.Fatal("unsafe path accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.g.SaveFile(ctx, repo, file, "canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo.Root, file.Path)); string(data) != "base\n" {
		t.Fatal("cancellation changed original")
	}
	if err := os.Remove(filepath.Join(repo.Root, file.Path)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.g.SaveFile(context.Background(), repo, file, "revive missing"); err == nil {
		t.Fatal("missing target recreated without confirmation")
	}
}

func TestSaveFileRejectsChangedCheckoutAndReadOnlyFile(t *testing.T) {
	f := newWorkflow(t)
	repo := f.read()
	file, err := f.g.ReadFile(context.Background(), repo.Root, "project", domain.File{Path: "文件 name.txt"})
	if err != nil {
		t.Fatal(err)
	}
	f.git("commit", "--allow-empty", "-m", "external checkout advanced")
	if _, err := f.g.SaveFile(context.Background(), repo, file, "stale checkout"); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale checkout accepted: %v", err)
	}
	path := filepath.Join(repo.Root, file.Path)
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0644)
	file, err = f.g.ReadFile(context.Background(), repo.Root, "project", domain.File{Path: file.Path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.g.SaveFile(context.Background(), f.read(), file, "read-only overwrite"); err == nil {
		t.Fatal("read-only target replaced")
	}
}

func TestSaveFileRejectsSymlink(t *testing.T) {
	f := newWorkflow(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("external user data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.root, "link.txt")); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	repo := f.read()
	file, err := f.g.ReadFile(context.Background(), repo.Root, "project", domain.File{Path: "link.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if file.DiskVersion != "" {
		t.Fatal("symlink obtained editable identity")
	}
	if _, err := f.g.SaveFile(context.Background(), repo, file, "overwrite outside"); err == nil {
		t.Fatal("symlink saved")
	}
	if data, _ := os.ReadFile(outside); string(data) != "external user data" {
		t.Fatal("external file changed")
	}
}
