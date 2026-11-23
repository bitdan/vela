package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestPatchExportAppliesSelectedBinaryRenameAndDeletion(t *testing.T) {
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(config, []byte("[core]\n autocrlf = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	f := newWorkflow(t)
	f.write("delete.txt", "delete me\n")
	f.write("binary.dat", "before\x00binary")
	f.git("add", ".")
	f.git("commit", "-m", "patch base")
	head := f.git("rev-parse", "HEAD")
	f.git("mv", "--", "文件 name.txt", "renamed 文件.txt")
	if err := os.Remove(filepath.Join(f.root, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	f.write("binary.dat", "after\x00binary")
	f.write("added.txt", "staged addition\n")
	f.git("add", ".")
	f.write("added.txt", "working after staged\n")
	repo := f.read()
	index := f.git("write-tree")
	doc, err := f.g.ReadChangesPatch(context.Background(), repo, "staged", repo.Staged)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Text, "GIT binary patch") || !strings.Contains(doc.Text, "rename from") || !strings.Contains(doc.Text, "deleted file mode") || strings.Contains(doc.Text, "working after staged") {
		t.Fatal("export omitted binary/rename/deletion or included unstaged data")
	}
	clone, err := f.g.Clone(context.Background(), f.root, filepath.Join(t.TempDir(), "patch-clone"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.g.runInput(context.Background(), clone, doc.Text, "apply", "--check", "--index", "-"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.g.runInput(context.Background(), clone, doc.Text, "apply", "--index", "-"); err != nil {
		t.Fatal(err)
	}
	result, err := f.g.read(context.Background(), clone, "write-tree")
	if err != nil || strings.TrimSpace(result) != index {
		t.Fatal("export did not recreate selected index exactly", err)
	}
	working, err := f.g.ReadChangesPatch(context.Background(), repo, "working", repo.Working)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.g.runInput(context.Background(), clone, working.Text, "apply", "-"); err != nil {
		t.Fatal(err)
	}
	if actual, _ := os.ReadFile(filepath.Join(clone, "added.txt")); string(actual) != "working after staged\n" {
		t.Fatal("working export did not recreate disk content")
	}
	if f.git("rev-parse", "HEAD") != head || f.git("write-tree") != index {
		t.Fatal("export changed source HEAD/index")
	}
}

func TestPatchExportHistoricalRootBoundsAndCancellation(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	repo := f.read()
	file := domain.File{Path: "文件 name.txt", Status: "A"}
	doc, err := f.g.ReadChangesPatch(context.Background(), repo, repo.Head, []domain.File{file})
	if err != nil || !strings.Contains(doc.Text, "new file mode") {
		t.Fatal("root commit patch unavailable", err)
	}
	f.write(file.Path, "changed\n")
	repo = f.read()
	if _, err = f.g.readChangesPatch(context.Background(), repo, "working", repo.Working, 64); !errors.Is(err, errOutputLimit) {
		t.Fatalf("patch output was not bounded %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = f.g.ReadChangesPatch(ctx, repo, "working", repo.Working); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost %v", err)
	}
	f.git("add", ".")
	if _, err = f.g.ReadChangesPatch(context.Background(), repo, "working", repo.Working); !errors.Is(err, ErrStaleIndex) {
		t.Fatal("stale checkout exported", err)
	}
}

func TestPatchExportRejectsRawNULForcedText(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("forced.dat", "before\x00bytes")
	f.write(".gitattributes", "forced.dat diff\n")
	f.git("add", ".")
	f.git("commit", "-m", "forced text")
	f.write("forced.dat", "after\x00bytes")
	repo := f.read()
	if _, err := f.g.ReadChangesPatch(context.Background(), repo, "working", repo.Working); err == nil {
		t.Fatal("raw NUL patch accepted for clipboard/preview")
	}
}

func TestHistoricalPatchExportUsesCanonicalGitlinkDiff(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	first := f.git("rev-parse", "HEAD")
	f.git("update-index", "--add", "--cacheinfo", "160000,"+first+",sub")
	f.git("commit", "-m", "gitlink base")
	second := f.git("rev-parse", "HEAD")
	f.git("update-index", "--cacheinfo", "160000,"+second+",sub")
	f.write("文件 name.txt", "regular update\n")
	f.git("add", "--", "文件 name.txt")
	f.git("commit", "-m", "gitlink update")
	f.git("config", "diff.submodule", "log")
	f.git("config", "diff.ignoreSubmodules", "all")
	repo := f.read()
	doc, err := f.g.ReadChangesPatch(context.Background(), repo, repo.Head, []domain.File{{Path: "sub", Status: "M"}, {Path: "文件 name.txt", Status: "M"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Text, "Subproject commit "+second) || !strings.Contains(doc.Text, "diff --git a/sub b/sub") {
		t.Fatal("user diff config hid or changed gitlink patch semantics")
	}
	if _, err = f.g.runInput(context.Background(), repo.Root, doc.Text, "apply", "--check", "--cached", "--reverse", "-"); err != nil {
		t.Fatal("historical patch could not reverse-check against its commit index", err)
	}
}
