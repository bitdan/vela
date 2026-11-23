package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestMergeConflictWorkflow(t *testing.T) {
	t.Parallel()
	ctx, dir, g := context.Background(), t.TempDir(), New()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Vela Test", "-c", "user.email=vela@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "中文 file.txt"), []byte(text+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("config", "user.name", "Vela Test")
	git("config", "user.email", "vela@example.test")
	git("config", "commit.gpgsign", "false")
	write("base")
	git("add", "--all")
	git("commit", "-m", "base")
	git("checkout", "-b", "topic")
	write("incoming")
	git("commit", "-am", "topic")
	git("checkout", "main")
	write("current")
	git("commit", "-am", "main")
	head := git("rev-parse", "HEAD")
	read := func() domain.Repository {
		t.Helper()
		repo, err := g.ReadRepository(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	run := func(kind domain.RepositoryOperationKind, value string) error {
		return g.ExecuteRepositoryOperation(ctx, read(), domain.RepositoryOperation{Kind: kind, Source: value})
	}
	start := func() {
		t.Helper()
		if err := run(domain.StartMerge, "refs/heads/topic"); err == nil {
			t.Fatal("expected merge to pause on conflict")
		}
		repo := read()
		if len(repo.MergeHeads) != 1 || len(repo.Conflicts) != 1 || repo.Head != head {
			t.Fatalf("unexpected merge state: %v %v %s", repo.MergeHeads, repo.Conflicts, repo.Head)
		}
	}
	start()
	if err := run(domain.ContinueMerge, ""); err == nil {
		t.Fatal("continued with unresolved files")
	}
	if err := run(domain.AbortMerge, ""); err != nil || len(read().MergeHeads) != 0 || git("rev-parse", "HEAD") != head {
		t.Fatalf("abort failed: %v", err)
	}
	start()
	write("manual resolution")
	if err := run(domain.MarkResolved, "中文 file.txt"); err != nil || git("show", ":中文 file.txt") != "manual resolution" {
		t.Fatalf("mark resolved did not stage the manually edited file: %v", err)
	}
	if err := run(domain.AbortMerge, ""); err != nil {
		t.Fatal(err)
	}
	start()
	file, err := g.ReadFile(ctx, dir, "merge-conflict", domain.File{Path: "中文 file.txt", Status: "U"})
	if err != nil || file.Old != "current\n" || file.New != "incoming\n" {
		t.Fatalf("wrong conflict sides: %+v, %v", file, err)
	}
	snapshot := read()
	doc, err := g.ReadMergeDocument(ctx, snapshot, file.Path)
	if err != nil || doc.Current != file.Old || doc.Incoming != file.New {
		t.Fatalf("wrong editable merge document: %+v, %v", doc, err)
	}
	blocks, incomplete := domain.ConflictBlocks(doc.Result, doc.MarkerSize)
	if incomplete || len(blocks) != 1 {
		t.Fatalf("could not locate the Git conflict block: %v, %t", blocks, incomplete)
	}
	edit := domain.RepositoryOperation{Kind: domain.SaveMergeResult, Source: file.Path, Result: doc.Result,
		WorktreeVersion: doc.WorktreeVersion, Conflict: doc.Conflict}
	if err := g.ExecuteRepositoryOperation(ctx, snapshot, edit); err == nil {
		t.Fatal("saved unresolved markers")
	}
	edit.Result = "edited result\n"
	write("external change")
	if err := g.ExecuteRepositoryOperation(ctx, snapshot, edit); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("external change was not protected: %v", err)
	}
	worktree := filepath.Join(dir, file.Path)
	if err := os.WriteFile(worktree, []byte(doc.Result), 0600); err != nil {
		t.Fatal(err)
	}
	if err := g.ExecuteRepositoryOperation(ctx, snapshot, edit); err != nil || git("show", ":中文 file.txt") != "edited result" {
		t.Fatalf("editor result was not staged: %v", err)
	}
	if err := run(domain.AbortMerge, ""); err != nil {
		t.Fatal(err)
	}
	start()
	if err := run(domain.AcceptIncoming, file.Path); err != nil || len(read().Conflicts) != 0 {
		t.Fatalf("accept incoming failed: %v", err)
	}
	if err := run(domain.ContinueMerge, ""); err != nil {
		t.Fatal(err)
	}
	if len(strings.Fields(git("show", "-s", "--format=%P"))) != 2 || git("show", "HEAD:中文 file.txt") != "incoming" {
		t.Fatal("continued merge did not record both parents and the resolution")
	}
	git("checkout", "-b", "next")
	write("fast forward")
	git("commit", "-am", "next")
	next := git("rev-parse", "HEAD")
	git("checkout", "main")
	write("uncommitted")
	if err := run(domain.StartMerge, "refs/heads/next"); err == nil || git("rev-parse", "HEAD") == next {
		t.Fatalf("dirty merge was accepted: %v", err)
	}
	git("restore", "--worktree", "--", "中文 file.txt")
	if status := git("status", "--porcelain=v1"); status != "" {
		t.Fatalf("fixture still has changes after restore: %s", status)
	}
	if err := run(domain.StartMerge, "refs/heads/next"); err != nil || git("rev-parse", "HEAD") != next {
		t.Fatalf("fast-forward merge failed: %v", err)
	}
	git("checkout", "-b", "drop")
	git("rm", "--", "中文 file.txt")
	git("commit", "-m", "delete file")
	git("checkout", "main")
	write("modify file")
	git("commit", "-am", "modify file")
	if err := run(domain.StartMerge, "refs/heads/drop"); err == nil {
		t.Fatal("expected a modify/delete conflict")
	}
	if err := run(domain.AcceptIncoming, "中文 file.txt"); err != nil {
		t.Fatalf("accepting the deleted side failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "中文 file.txt")); !os.IsNotExist(err) || len(read().Conflicts) != 0 {
		t.Fatalf("incoming deletion did not resolve the file: %v", err)
	}
	if err := run(domain.ContinueMerge, ""); err != nil {
		t.Fatal(err)
	}
}
