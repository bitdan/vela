package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/application"
	"vela/internal/domain"
)

func hunkShelfFixture(t *testing.T) (*workflowFixture, domain.Shelf, string) {
	t.Helper()
	f := newWorkflow(t)
	var body strings.Builder
	for i := 0; i < 32; i++ {
		fmt.Fprintf(&body, "line %02d\n", i)
	}
	base := strings.Replace(body.String(), "line 01\n", "\n", 1)
	f.write("文件 name.txt", base)
	f.write("other.txt", "old\n")
	f.git("add", ".")
	f.git("commit", "-m", "base for separated hunks")
	staged := strings.Replace(base, "line 23\n", "saved staged second\n", 1)
	f.write("文件 name.txt", staged)
	f.git("add", "--", "文件 name.txt")
	f.write("文件 name.txt", strings.Replace(staged, "line 02\n", "chosen first\n", 1))
	f.write("other.txt", "whole other\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Hunk shelf", Paths: []string{"文件 name.txt", "other.txt"}}); err != nil {
		t.Fatal(err)
	}
	return f, f.read().Shelf[0], base
}

func TestShelfHunksRestoreOnlyChosenContent(t *testing.T) {
	t.Parallel()
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprint(mixed), func(t *testing.T) {
			f, entry, base := hunkShelfFixture(t)
			repo := f.read()
			beforeIndex := f.git("write-tree")
			doc, err := f.g.ReadPatch(context.Background(), repo, "shelf:"+entry.ID, "文件 name.txt")
			if err != nil {
				t.Fatal(err)
			}
			paths := []string{"文件 name.txt"}
			if mixed {
				paths = append(paths, "other.txt")
			}
			op := domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, Paths: paths, SelectShelfFiles: true, ShelfHunks: []domain.ShelfHunkSelection{{Path: doc.Path, Version: doc.Version, Hunks: []int{0}}}}
			if err := f.g.ExecuteRepositoryOperation(context.Background(), repo, op); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(f.root, doc.Path))
			want := strings.Replace(base, "line 02\n", "chosen first\n", 1)
			if string(data) != want || f.git("write-tree") != beforeIndex || f.git("rev-parse", "HEAD") != repo.Head {
				t.Fatal("Hunk restore included unselected staged/worktree content or changed Index/HEAD")
			}
			data, _ = os.ReadFile(filepath.Join(f.root, "other.txt"))
			other := "old\n"
			if mixed {
				other = "whole other\n"
			}
			if string(data) != other {
				t.Fatalf("whole-file selection: %q", data)
			}
			kept := f.read()
			if len(kept.Shelf) != 1 || kept.Shelf[0].OID != entry.OID {
				t.Fatal("Hunk restore modified original Shelf")
			}
		})
	}
}

func TestShelfHunksRejectInvalidAndStaleSelection(t *testing.T) {
	t.Parallel()
	f, entry, _ := hunkShelfFixture(t)
	repo := f.read()
	doc, err := f.g.ReadPatch(context.Background(), repo, "shelf:"+entry.ID, "文件 name.txt")
	if err != nil {
		t.Fatal(err)
	}
	status, index := f.git("status", "--porcelain=v1", "-z"), f.git("write-tree")
	valid := domain.ShelfHunkSelection{Path: doc.Path, Version: doc.Version, Hunks: []int{0}}
	for _, kind := range []string{"empty", "duplicate", "unknown", "version", "index", "remove"} {
		t.Run(kind, func(t *testing.T) {
			op := domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, Paths: []string{doc.Path}, SelectShelfFiles: true, ShelfHunks: []domain.ShelfHunkSelection{valid}}
			switch kind {
			case "empty":
				op.ShelfHunks[0].Hunks = nil
			case "duplicate":
				op.ShelfHunks[0].Hunks = []int{0, 0}
			case "unknown":
				op.ShelfHunks[0].Path = "missing.txt"
			case "version":
				op.ShelfHunks[0].Version = "stale"
			case "index":
				op.RestoreIndex = true
			case "remove":
				op.RemoveApplied = true
			}
			if err := f.g.ExecuteRepositoryOperation(context.Background(), repo, op); err == nil {
				t.Fatal("invalid selection applied")
			}
			if f.git("status", "--porcelain=v1", "-z") != status || f.git("write-tree") != index {
				t.Fatal("rejected selection wrote files")
			}
		})
	}
	f.git("commit", "--allow-empty", "-m", "new HEAD")
	op := domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, Paths: []string{doc.Path}, SelectShelfFiles: true, ShelfHunks: []domain.ShelfHunkSelection{valid}}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), f.read(), op); !errors.Is(err, ErrStalePatch) {
		t.Fatalf("old HEAD preview accepted: %v", err)
	}
}

func TestShelfHunkConflictRetainsOnlySelectedSavedContent(t *testing.T) {
	t.Parallel()
	f, entry, base := hunkShelfFixture(t)
	f.write("文件 name.txt", strings.Replace(base, "line 02\n", "target branch change\n", 1))
	f.git("add", ".")
	f.git("commit", "-m", "conflicting target")
	repo := f.read()
	doc, err := f.g.ReadPatch(context.Background(), repo, "shelf:"+entry.ID, "文件 name.txt")
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, Paths: []string{doc.Path}, SelectShelfFiles: true, ShelfHunks: []domain.ShelfHunkSelection{{Path: doc.Path, Version: doc.Version, Hunks: []int{0}}}}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), repo, op); err == nil {
		t.Fatal("conflicting Hunk restore falsely reported success")
	}
	if text := f.git("show", ":3:"+doc.Path); !strings.Contains(text, "chosen first") || strings.Contains(text, "saved staged second") {
		t.Fatal("conflict imported unchecked saved content")
	}
	paused := f.read()
	if len(paused.Conflicts) == 0 || len(paused.Shelf) != 1 || paused.Shelf[0].OID != entry.OID || paused.Head != repo.Head {
		t.Fatal("conflict lost original Shelf/HEAD")
	}
}

func TestReadShelfHunkPatch(t *testing.T) {
	t.Parallel()
	f, entry, _ := hunkShelfFixture(t)
	f.git("config", "diff.suppressBlankEmpty", "true")
	doc, err := f.g.ReadPatch(context.Background(), f.read(), "shelf:"+entry.ID, "文件 name.txt")
	if err != nil || len(doc.Hunks) != 2 || doc.Version == "" || doc.Source != "shelf:"+entry.ID {
		t.Fatalf("Shelf Git hunk preview: %+v %v", doc, err)
	}
	if !strings.Contains(doc.Hunks[0].Text, "+chosen first") || !strings.Contains(doc.Hunks[1].Text, "+saved staged second") {
		t.Fatal("preview does not contain real independent Git hunks")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if strings.Contains(string(data), "chosen first") {
		t.Fatal("preview wrote the worktree")
	}
}

func TestShelfHunkProjectionCancellationPreservesOriginals(t *testing.T) {
	t.Parallel()
	f, entry, _ := hunkShelfFixture(t)
	repo := f.read()
	doc, err := f.g.ReadPatch(context.Background(), repo, "shelf:"+entry.ID, "文件 name.txt")
	if err != nil {
		t.Fatal(err)
	}
	index, status := f.git("write-tree"), f.git("status", "--porcelain=v1", "-z")
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := application.WithTaskProgress(parent, func(phase string) {
		if phase == "Preparing selected shelf index and working versions" {
			cancel()
		}
	})
	op := domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, Paths: []string{doc.Path, "other.txt"}, SelectShelfFiles: true, ShelfHunks: []domain.ShelfHunkSelection{{Path: doc.Path, Version: doc.Version, Hunks: []int{0}}}}
	if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("projection cancel: %v", err)
	}
	if f.git("write-tree") != index || f.git("status", "--porcelain=v1", "-z") != status || f.git("rev-parse", "HEAD") != repo.Head || f.read().Shelf[0].OID != entry.OID {
		t.Fatal("canceled projection changed original state")
	}
}
