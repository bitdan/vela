package gitcli

import (
	"context"
	"errors"
	"strings"
	"testing"
	"vela/internal/domain"
)

func TestFileHistoryPagesFollowRenameAtFrozenRevision(t *testing.T) {
	f := newWorkflow(t)
	old := "文件 name.txt"
	path := "-renamed [文件].txt"
	f.write(old, "second\n")
	f.git("commit", "-am", "second")
	f.git("mv", "--", old, path)
	f.git("commit", "-m", "rename")
	f.write(path, "latest\n")
	f.git("commit", "-am", "latest")
	revision := f.git("rev-parse", "HEAD")
	head, index := revision, f.git("write-tree")
	page, err := f.g.ReadFileHistory(context.Background(), f.read().Root, revision, path, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Commits) != 2 || !page.HasMore || page.Commits[0].Subject != "latest" || page.Commits[1].Subject != "rename" {
		t.Fatalf("wrong first page %+v", page)
	}
	f.write(path, "later\n")
	f.git("commit", "-am", "later")
	older, err := f.g.ReadFileHistory(context.Background(), f.read().Root, revision, path, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(older.Commits) != 2 || older.HasMore || older.Commits[0].Subject != "second" || older.Commits[1].Subject != "base" {
		t.Fatalf("rename history pagination failed %+v", older)
	}
	for _, commit := range older.Commits {
		if len(commit.Files) != 1 || commit.Files[0].Path != old {
			t.Fatalf("historical path lost %+v", commit)
		}
		file, err := f.g.ReadFile(context.Background(), f.read().Root, commit.Hash, commit.Files[0])
		if err != nil || !strings.Contains(file.New, "\n") {
			t.Fatalf("older version unreadable %+v %v", file, err)
		}
	}
	if head == f.git("rev-parse", "HEAD") || index == f.git("write-tree") {
		t.Fatal("fixture did not advance")
	}
}

func TestFileHistoryRejectsInvalidQueriesAndCancellation(t *testing.T) {
	f := newWorkflow(t)
	repo := f.read()
	before, index := repo.Head, f.git("write-tree")
	for _, path := range []string{"../escape", ".git/config"} {
		if _, err := f.g.ReadFileHistory(context.Background(), repo.Root, repo.Head, path, 0, 100); err == nil {
			t.Fatal("unsafe history path accepted")
		}
	}
	if _, err := f.g.ReadFileHistory(context.Background(), repo.Root, "HEAD;bad", "文件 name.txt", 0, 100); err == nil {
		t.Fatal("non-OID revision accepted")
	}
	if _, err := f.g.ReadFileHistory(context.Background(), repo.Root, repo.Head, "文件 name.txt", -1, 100); err == nil {
		t.Fatal("negative offset accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.g.ReadFileHistory(ctx, repo.Root, repo.Head, "文件 name.txt", 0, 100); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
	page, err := f.g.ReadFileHistory(context.Background(), repo.Root, repo.Head, "not tracked.txt", 0, 100)
	if err != nil || len(page.Commits) != 0 || page.HasMore {
		t.Fatalf("empty history incorrect %+v %v", page, err)
	}
	if f.git("rev-parse", "HEAD") != before || f.git("write-tree") != index {
		t.Fatal("history query mutated repository")
	}
}

func TestReadHistoricalRevisionWithoutParentOrCurrentPath(t *testing.T) {
	f := newWorkflow(t)
	base := f.git("rev-parse", "HEAD")
	f.git("mv", "--", "文件 name.txt", "renamed.txt")
	f.git("commit", "-m", "rename")
	file, err := f.g.ReadFile(context.Background(), f.read().Root, "revision:"+base, domain.File{Path: "文件 name.txt"})
	if err != nil || file.New != "base\n" || file.Old != "" || file.DiskVersion != "" {
		t.Fatalf("root historical version unavailable %+v %v", file, err)
	}
}

func TestFileHistoryObjectOnlyNewlinePath(t *testing.T) {
	f := newWorkflow(t)
	f.git("config", "core.protectNTFS", "false")
	path := "object\npath.txt"
	f.write("history-blob.tmp", "unique newline history\n")
	blob := f.git("hash-object", "-w", "--", "history-blob.tmp")
	f.git("update-index", "--add", "--cacheinfo", "100644,"+blob+","+path)
	f.git("commit", "-m", "newline history")
	revision := f.git("rev-parse", "HEAD")
	page, err := f.g.ReadFileHistory(context.Background(), f.root, revision, path, 0, 100)
	if err != nil || len(page.Commits) != 1 || page.Commits[0].Files[0].Path != path {
		t.Fatalf("newline path protocol failed %+v %v", page, err)
	}
	file, err := f.g.ReadFile(context.Background(), f.root, "revision:"+revision, domain.File{Path: path})
	if err != nil || file.New != "unique newline history\n" {
		t.Fatalf("newline historical blob unavailable %+v %v", file, err)
	}
}
