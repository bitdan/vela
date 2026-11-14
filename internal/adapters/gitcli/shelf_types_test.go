package gitcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestShelfBinaryAddDeleteRenameAndConflict(t *testing.T) {
	f := newWorkflow(t)
	f.write("binary.dat", "base\x00bytes")
	f.write("deleted.txt", "delete me\n")
	f.git("add", ".")
	f.git("commit", "-m", "types")
	f.write("binary.dat", "new\x00binary\xff")
	f.git("mv", "文件 name.txt", "renamed.txt")
	if err := os.Remove(filepath.Join(f.root, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(f.root, "added-dir"), 0700); err != nil {
		t.Fatal(err)
	}
	f.write("added-dir/new.txt", "new\r\n")
	f.git("add", "--", "added-dir/new.txt")
	paths := []string{"binary.dat", "deleted.txt", "renamed.txt", "added-dir/new.txt"}
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Types", Paths: paths}); err != nil {
		t.Fatal(err)
	}
	r := f.read()
	if len(r.Shelf) != 1 || len(r.Working) != 0 || len(r.Staged) != 0 {
		t.Fatal("types did not clean")
	}
	entry := r.Shelf[0]
	if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, RestoreIndex: true, RemoveApplied: true}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(f.root, "binary.dat"))
	if string(got) != "new\x00binary\xff" {
		t.Fatal("binary payload changed")
	}
	if _, err := os.Stat(filepath.Join(f.root, "deleted.txt")); !os.IsNotExist(err) {
		t.Fatal("deletion lost")
	}
	if _, err := os.Stat(filepath.Join(f.root, "renamed.txt")); err != nil {
		t.Fatal("rename lost")
	}
	if got, _ := os.ReadFile(filepath.Join(f.root, "added-dir/new.txt")); string(got) != "new\r\n" {
		t.Fatal("addition lost")
	}
	if !f.read().Shelf[0].Deleted {
		t.Fatal("RemoveApplied did not retain recoverable trash")
	}
	// A separate fixture verifies conflicts retain the active shelf.
	conflict := newWorkflow(t)
	conflict.write("文件 name.txt", "shelf version\n")
	if err := conflict.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Conflict", Paths: []string{"文件 name.txt"}}); err != nil {
		t.Fatal(err)
	}
	entry = conflict.read().Shelf[0]
	conflict.write("文件 name.txt", "different commit\n")
	conflict.git("commit", "-am", "different")
	if err := conflict.run(domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, RemoveApplied: true}); err == nil {
		t.Fatal("conflicting apply reported success")
	}
	paused := conflict.read()
	if len(paused.Conflicts) != 1 || len(paused.Shelf) != 1 || paused.Shelf[0].Deleted {
		t.Fatal("conflict did not retain original shelf")
	}
	if _, err := conflict.g.ReadFile(context.Background(), conflict.root, "shelf:"+entry.ID, domain.File{Path: "文件 name.txt"}); err != nil {
		t.Fatal(err)
	}
}

func TestShelfCleanupFailureRetainsDurableCopy(t *testing.T) {
	f := newWorkflow(t)
	f.write("文件 name.txt", "recoverable\n")
	repo := f.read()
	lock := filepath.Join(repo.GitDir, "index.lock")
	if err := os.WriteFile(lock, []byte("test owns this lock"), 0600); err != nil {
		t.Fatal(err)
	}
	err := f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Recovery", Paths: []string{"文件 name.txt"}})
	if err == nil {
		t.Fatal("cleanup unexpectedly succeeded with locked index")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	after := f.read()
	if len(after.Shelf) != 1 || after.Shelf[0].Deleted {
		t.Fatal("cleanup failure lost durable shelf")
	}
	file, err := f.g.ReadFile(context.Background(), f.root, "shelf:"+after.Shelf[0].ID, domain.File{Path: "文件 name.txt"})
	if err != nil || file.New != "recoverable\n" {
		t.Fatalf("saved payload: %+v %v", file, err)
	}
	if f.git("show", ":文件 name.txt") != "base" {
		t.Fatal("cleanup failure changed index")
	}
}

func TestShelfCRLFAutocrlfAndWorktreeScope(t *testing.T) {
	f := newWorkflow(t)
	f.git("config", "core.autocrlf", "true")
	f.write("文件 name.txt", "working\r\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "CRLF", Paths: []string{"文件 name.txt"}}); err != nil {
		t.Fatal(err)
	}
	entry := f.read().Shelf[0]
	linked := filepath.Join(t.TempDir(), "linked")
	f.git("worktree", "add", "-b", "linked", linked, "HEAD")
	other, err := f.g.ReadRepository(context.Background(), linked)
	if err != nil || len(other.Shelf) != 0 {
		t.Fatal("shelf leaked into another worktree", err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "working\r\n" {
		t.Fatalf("autocrlf roundtrip: %q", data)
	}
}

func TestShelfWorktreeSnapshotPreservesStagedMode(t *testing.T) {
	f := newWorkflow(t)
	f.git("config", "core.filemode", "false")
	f.git("update-index", "--chmod=+x", "--", "文件 name.txt")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Mode", Paths: []string{"文件 name.txt"}}); err != nil {
		t.Fatal(err)
	}
	entry := f.read().Shelf[0]
	if !strings.HasPrefix(f.git("ls-tree", entry.OID, "--", "文件 name.txt"), "100755 ") {
		t.Fatal("working snapshot lost staged executable mode")
	}
}

func TestShelfStagedIgnoredAddition(t *testing.T) {
	f := newWorkflow(t)
	f.write(".gitignore", "ignored.txt\n")
	f.git("add", ".gitignore")
	f.git("commit", "-m", "ignore")
	f.write("ignored.txt", "saved ignored addition\n")
	f.git("add", "-f", "ignored.txt")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Ignored addition", Paths: []string{"ignored.txt"}}); err != nil {
		t.Fatal(err)
	}
	entry := f.read().Shelf[0]
	if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, RestoreIndex: true}); err != nil {
		t.Fatal(err)
	}
	if f.git("show", ":ignored.txt") != "saved ignored addition" {
		t.Fatal("staged ignored addition lost")
	}
}
