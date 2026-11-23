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

func TestShelfSelectedRoundtripAndIndependentStash(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("other.txt", "base other\n")
	f.git("add", ".")
	f.git("commit", "-m", "other")
	f.write("文件 name.txt", "index\n")
	f.git("add", "--", "文件 name.txt")
	f.write("文件 name.txt", "working\r\n")
	f.write("other.txt", "unrelated\n")
	before := f.read()
	head, stash := before.Head, f.git("stash", "list")
	if err := f.g.ExecuteRepositoryOperation(context.Background(), before, domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "My shelf 中文", Paths: []string{"文件 name.txt"}}); err != nil {
		t.Fatal(err)
	}
	saved := f.read()
	if len(saved.Shelf) != 1 || saved.Head != head || f.git("stash", "list") != stash || f.git("show", ":文件 name.txt") != "base" {
		t.Fatalf("shelf changed unrelated state: %+v", saved.Shelf)
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "other.txt"))
	if string(data) != "unrelated\n" {
		t.Fatal("unselected content changed")
	}
	// Clean the unrelated file explicitly in this temporary repository.
	f.git("restore", "--", "other.txt")
	saved = f.read()
	entry := saved.Shelf[0]
	if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: entry.OID, Target: entry.ID, RestoreIndex: true}); err != nil {
		t.Fatal(err)
	}
	if f.git("show", ":文件 name.txt") != "index" {
		t.Fatal("index distinction was lost")
	}
	data, _ = os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "working\r\n" {
		t.Fatalf("raw worktree content: %q", data)
	}
	if f.git("stash", "list") != stash || f.git("rev-parse", "HEAD") != head || len(f.read().Shelf) != 1 {
		t.Fatal("apply modified stash/head or removed retained entry")
	}
}

func TestShelfMetadataStaleAndCancel(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "shelved\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Original", Paths: []string{"文件 name.txt"}}); err != nil {
		t.Fatal(err)
	}
	repo := f.read()
	entry := repo.Shelf[0]
	if err := f.run(domain.RepositoryOperation{Kind: domain.RenameShelf, Source: entry.OID, Target: entry.ID, Message: "Renamed"}); err != nil {
		t.Fatal(err)
	}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.DeleteShelf, Source: entry.OID, Target: entry.ID}); err == nil {
		t.Fatal("stale shelf identity accepted")
	}
	entry = f.read().Shelf[0]
	for _, kind := range []domain.RepositoryOperationKind{domain.DeleteShelf, domain.RestoreShelf, domain.DeleteShelf, domain.PurgeShelf} {
		if err := f.run(domain.RepositoryOperation{Kind: kind, Source: entry.OID, Target: entry.ID}); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.read().Shelf) != 0 {
		t.Fatal("permanent deletion retained ref")
	}
	f.write("文件 name.txt", "later\n")
	repo = f.read()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ExecuteRepositoryOperation(ctx, repo, domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Canceled", Paths: []string{"文件 name.txt"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled save: %v", err)
	}
	if len(f.read().Shelf) != 0 {
		t.Fatal("canceled save published shelf")
	}
	stale := f.read()
	f.write("文件 name.txt", "external\n")
	if err := f.g.ExecuteRepositoryOperation(context.Background(), stale, domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Stale", Paths: []string{"文件 name.txt"}}); err == nil {
		t.Fatal("stale worktree accepted")
	}
	for _, p := range []string{"../escape", ".git/config", "missing"} {
		if err := f.run(domain.RepositoryOperation{Kind: domain.SaveShelf, Message: "Bad", Paths: []string{p}}); err == nil {
			t.Fatalf("bad shelf path accepted: %q", p)
		}
	}
	if strings.TrimSpace(f.git("stash", "list")) != "" {
		t.Fatal("shelf used refs/stash")
	}
}
