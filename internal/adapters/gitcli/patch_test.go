package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestPatchStageUnstageDiscardDistinctHunks(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	p := "文件 name.txt"
	var lines []string
	for n := 1; n <= 30; n++ {
		lines = append(lines, fmt.Sprintf("line %02d", n))
	}
	base := strings.Join(lines, "\n") + "\n"
	f.write(p, base)
	f.git("add", "--", p)
	f.git("commit", "-m", "many lines")
	head := f.git("rev-parse", "HEAD")
	first := strings.Replace(base, "line 02", "first changed", 1)
	both := strings.Replace(first, "line 27", "second changed", 1)
	f.write(p, both)
	repo := f.read()
	doc, err := f.g.ReadPatch(context.Background(), repo, "working", p)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Hunks) != 2 {
		t.Fatalf("expected separate Git hunks: %+v", doc)
	}
	if err = f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.StageHunk, Path: p, PatchVersion: doc.Version, HunkIndex: 0}); err != nil {
		t.Fatal(err)
	}
	if f.git("show", ":"+p) != strings.TrimSuffix(first, "\n") {
		t.Fatal("staging first hunk changed other index lines")
	}
	if text, _ := os.ReadFile(filepath.Join(repo.Root, p)); string(text) != both {
		t.Fatal("stage changed worktree")
	}
	repo = f.read()
	staged, err := f.g.ReadPatch(context.Background(), repo, "staged", p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.UnstageHunk, Path: p, PatchVersion: staged.Version, HunkIndex: 0}); err != nil {
		t.Fatal(err)
	}
	if f.git("show", ":"+p) != strings.TrimSuffix(base, "\n") {
		t.Fatal("unstage did not restore selected index hunk")
	}
	repo = f.read()
	doc, err = f.g.ReadPatch(context.Background(), repo, "working", p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.DiscardHunk, Path: p, PatchVersion: doc.Version, HunkIndex: 1}); err != nil {
		t.Fatal(err)
	}
	if text, _ := os.ReadFile(filepath.Join(repo.Root, p)); string(text) != first {
		t.Fatalf("discard lost unrelated edit: %q", text)
	}
	if f.git("rev-parse", "HEAD") != head || f.git("show", ":"+p) != strings.TrimSuffix(base, "\n") {
		t.Fatal("discard changed HEAD/index")
	}
}

func TestPatchRejectsStaleDiskAndCancellation(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	p := "文件 name.txt"
	f.write(p, "changed\n")
	repo := f.read()
	doc, err := f.g.ReadPatch(context.Background(), repo, "working", p)
	if err != nil {
		t.Fatal(err)
	}
	index, head := f.git("write-tree"), f.git("rev-parse", "HEAD")
	f.write(p, "changed again\n")
	op := domain.RepositoryOperation{Kind: domain.DiscardHunk, Path: p, PatchVersion: doc.Version, HunkIndex: 0}
	if err = f.g.ExecuteRepositoryOperation(context.Background(), repo, op); !errors.Is(err, ErrStalePatch) {
		t.Fatalf("stale disk patch accepted %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = f.g.ExecuteRepositoryOperation(ctx, repo, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel not respected %v", err)
	}
	if text, _ := os.ReadFile(filepath.Join(repo.Root, p)); string(text) != "changed again\n" || f.git("write-tree") != index || f.git("rev-parse", "HEAD") != head {
		t.Fatal("failed/canceled patch mutated repository")
	}
}

func TestPatchSpecialPathAndNoFinalNewline(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	p := "-文件 [name].txt"
	f.write(p, "before")
	f.git("add", "--", p)
	f.git("commit", "-m", "special path")
	f.write(p, "after")
	repo := f.read()
	doc, err := f.g.ReadPatch(context.Background(), repo, "working", p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.StageHunk, Path: p, PatchVersion: doc.Version}); err != nil {
		t.Fatal(err)
	}
	if f.git("show", ":"+p) != "after" {
		t.Fatal("quoted/no-final-newline patch did not stage exact content")
	}
}

func TestPatchRejectsUnsupportedChangesAndStaleIndex(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	p := "文件 name.txt"
	ctx := context.Background()
	f.write("untracked.txt", "new\n")
	if _, err := f.g.ReadPatch(ctx, f.read(), "working", "untracked.txt"); err == nil {
		t.Fatal("untracked patch accepted")
	}
	f.git("add", "--", "untracked.txt")
	if _, err := f.g.ReadPatch(ctx, f.read(), "staged", "untracked.txt"); err == nil {
		t.Fatal("new-file patch accepted")
	}
	f.write(p, "binary\x00\n")
	if _, err := f.g.ReadPatch(ctx, f.read(), "working", p); err == nil {
		t.Fatal("binary patch accepted")
	}
	f.write(p, "changed\n")
	repo := f.read()
	doc, err := f.g.ReadPatch(ctx, repo, "working", p)
	if err != nil {
		t.Fatal(err)
	}
	f.git("add", "--", p)
	index := f.git("write-tree")
	err = f.g.ExecuteRepositoryOperation(ctx, repo, domain.RepositoryOperation{Kind: domain.StageHunk, Path: p, PatchVersion: doc.Version})
	if !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale index accepted %v", err)
	}
	if f.git("write-tree") != index {
		t.Fatal("stale operation changed index")
	}
}

func TestPatchDiscardRejectsCRLFAndTransformAttributes(t *testing.T) {
	t.Parallel()
	for _, attributes := range []bool{false, true} {
		t.Run(fmt.Sprint(attributes), func(t *testing.T) {
			f := newWorkflow(t)
			p := "文件 name.txt"
			text := "changed\r\n"
			if attributes {
				text = "changed\n"
				f.write(".gitattributes", "* text\n")
			}
			f.write(p, text)
			repo := f.read()
			doc, err := f.g.ReadPatch(context.Background(), repo, "working", p)
			if err != nil {
				t.Fatal(err)
			}
			index := f.git("write-tree")
			if err = f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.DiscardHunk, Path: p, PatchVersion: doc.Version}); err == nil {
				t.Fatal("transformed working file discard accepted")
			}
			if actual, _ := os.ReadFile(filepath.Join(repo.Root, p)); string(actual) != text || f.git("write-tree") != index {
				t.Fatal("rejected discard mutated files")
			}
		})
	}
}

func TestPatchUnstagePreservesUnrelatedBinaryWorkingFile(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	p := "文件 name.txt"
	f.write(p, "staged text\n")
	f.git("add", "--", p)
	f.write(p, "binary\x00working\n")
	repo := f.read()
	doc, err := f.g.ReadPatch(context.Background(), repo, "staged", p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.UnstageHunk, Path: p, PatchVersion: doc.Version}); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(repo.Root, p)); string(data) != "binary\x00working\n" || f.git("show", ":"+p) != "base" {
		t.Fatal("unstage changed working binary or failed to restore index")
	}
}
