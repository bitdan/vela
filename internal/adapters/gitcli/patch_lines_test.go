package gitcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestPartialLineStageUnstagePreservesOtherChanges(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
			t.Parallel()
			f := newWorkflow(t)
			name := "文件 name.txt"
			ctx := context.Background()
			base := "start\nold A\nold B\n" + strings.Repeat("context\n", 24) + "other\n"
			base = strings.ReplaceAll(base, "\n", eol)
			f.write(name, base)
			f.git("commit", "-am", "baseline")
			indexed := strings.Replace(base, "other", "already staged", 1)
			f.write(name, indexed)
			f.git("add", "--", name)
			both := strings.Replace(strings.Replace(indexed, "old A", "new A", 1), "old B", "new B", 1)
			f.write(name, both)
			repo := f.read()
			doc, err := f.g.ReadPatch(ctx, repo, "working", name)
			if err != nil {
				t.Fatal(err)
			}
			op := domain.RepositoryOperation{Kind: domain.StageHunk, Path: name, PatchVersion: doc.Version, HunkIndex: 0, SelectPatchLines: true, PatchLines: []int{0, 2}}
			if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); err != nil {
				t.Fatal(err)
			}
			blob, err := f.g.read(ctx, f.root, "show", ":0:"+name)
			expected := strings.Replace(indexed, "old A", "new A", 1)
			if err != nil || blob != expected {
				t.Fatalf("stage changed unchecked/staged lines: %q %v", blob, err)
			}
			disk, _ := os.ReadFile(filepath.Join(f.root, name))
			if string(disk) != both || f.git("rev-parse", "HEAD") != repo.Head {
				t.Fatal("stage changed disk/HEAD")
			}
			if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), op); err == nil {
				t.Fatal("accepted stale line selection")
			}
			repo = f.read()
			doc, err = f.g.ReadPatch(ctx, repo, "staged", name)
			if err != nil {
				t.Fatal(err)
			}
			op.Kind = domain.UnstageHunk
			op.PatchVersion = doc.Version
			op.PatchLines = []int{0, 1} // The refreshed staged hunk has one removed/added pair.
			if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); err != nil {
				t.Fatal(err)
			}
			blob, err = f.g.read(ctx, f.root, "show", ":0:"+name)
			disk, _ = os.ReadFile(filepath.Join(f.root, name))
			if err != nil || blob != indexed || string(disk) != both || f.git("rev-parse", "HEAD") != repo.Head {
				t.Fatal("unstage touched unchecked lines/disk/HEAD")
			}
		})
	}
}

func TestPartialLineZeroRangesRoundtrip(t *testing.T) {
	for _, deletion := range []bool{false, true} {
		t.Run(map[bool]string{false: "insert", true: "delete"}[deletion], func(t *testing.T) {
			t.Parallel()
			f := newWorkflow(t)
			ctx := context.Background()
			name := "文件 name.txt"
			base, work, want := "", "A\nB\n", "B\n"
			if deletion {
				base, work, want = "A\nB\n", "", "B\n"
			}
			f.write(name, base)
			f.git("commit", "-am", "zero range")
			f.write(name, work)
			repo := f.read()
			doc, err := f.g.ReadPatch(ctx, repo, "working", name)
			if err != nil {
				t.Fatal(err)
			}
			line := 1
			if deletion {
				line = 0
			}
			op := domain.RepositoryOperation{Kind: domain.StageHunk, Path: name, PatchVersion: doc.Version, SelectPatchLines: true, PatchLines: []int{line}}
			if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); err != nil {
				t.Fatal(err)
			}
			blob, err := f.g.read(ctx, f.root, "show", ":0:"+name)
			if err != nil || blob != want {
				t.Fatalf("zero range result: %q %v", blob, err)
			}
			repo = f.read()
			doc, err = f.g.ReadPatch(ctx, repo, "staged", name)
			if err != nil {
				t.Fatal(err)
			}
			op.Kind = domain.UnstageHunk
			op.PatchVersion = doc.Version
			op.PatchLines = []int{0}
			if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); err != nil {
				t.Fatal(err)
			}
			blob, err = f.g.read(ctx, f.root, "show", ":0:"+name)
			disk, _ := os.ReadFile(filepath.Join(f.root, name))
			if err != nil || blob != base || string(disk) != work {
				t.Fatalf("zero reverse: %q %v", blob, err)
			}
		})
	}
}

func TestPartialLineRejectsInvalidAndUnrepresentableSelection(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	ctx := context.Background()
	name := "文件 name.txt"
	f.write(name, "old")
	f.git("commit", "-am", "no newline")
	f.write(name, "new\n")
	repo := f.read()
	doc, err := f.g.ReadPatch(ctx, repo, "working", name)
	if err != nil {
		t.Fatal(err)
	}
	tree := f.git("write-tree")
	for _, indices := range [][]int{nil, {}, {1}, {0, 0}, {99}} {
		op := domain.RepositoryOperation{Kind: domain.StageHunk, Path: name, PatchVersion: doc.Version, HunkIndex: 0, SelectPatchLines: true, PatchLines: indices}
		if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); err == nil {
			t.Fatalf("accepted invalid lines %v", indices)
		}
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := f.g.ExecuteRepositoryOperation(cancelCtx, repo, domain.RepositoryOperation{Kind: domain.StageHunk, Path: name, PatchVersion: doc.Version, SelectPatchLines: true, PatchLines: []int{0, 1}}); err == nil {
		t.Fatal("canceled operation succeeded")
	}
	if f.git("write-tree") != tree {
		t.Fatal("failure changed Index")
	}
	f.write(".gitattributes", "*.txt text\n")
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), domain.RepositoryOperation{Kind: domain.StageHunk, Path: name, PatchVersion: doc.Version, SelectPatchLines: true, PatchLines: []int{0, 1}}); err == nil {
		t.Fatal("accepted conversion attributes")
	}
	disk, _ := os.ReadFile(filepath.Join(f.root, name))
	if string(disk) != "new\n" || f.git("write-tree") != tree {
		t.Fatal("failure touched disk/index")
	}
}
