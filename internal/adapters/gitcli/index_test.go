package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"vela/internal/domain"
)

func TestUpdateIndexStagesAndUnstagesSelectedFiles(t *testing.T) {
	dir := t.TempDir()
	g := New()
	ctx := context.Background()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"中文 [one].txt", "unrelated.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	selected := []domain.File{{Path: "中文 [one].txt", Status: "?"}}
	if err := g.UpdateIndex(ctx, repo, true, selected); err != nil {
		t.Fatal(err)
	}
	staged, err := g.ReadRepository(ctx, dir)
	if err != nil || len(staged.Staged) != 1 || staged.Staged[0].Path != selected[0].Path ||
		len(staged.Working) != 1 || staged.Working[0].Path != "unrelated.txt" {
		t.Fatalf("unexpected changes: staged %v working %v, %v", staged.Staged, staged.Working, err)
	}
	if err := g.UpdateIndex(ctx, repo, true, repo.Working); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale snapshot was accepted: %v", err)
	}
	if err := g.UpdateIndex(ctx, staged, false, staged.Staged); err != nil {
		t.Fatal(err)
	}
	result, err := g.ReadRepository(ctx, dir)
	if err != nil || len(result.Staged) != 0 || len(result.Working) != 2 {
		t.Fatalf("unstage in unborn branch: %v %v, %v", result.Staged, result.Working, err)
	}
	if content, err := os.ReadFile(filepath.Join(dir, selected[0].Path)); err != nil || string(content) != selected[0].Path {
		t.Fatalf("unstage modified the worktree: %q, %v", content, err)
	}
	cmd := exec.Command("git", "symbolic-ref", "HEAD")
	cmd.Dir = dir
	if head, err := cmd.Output(); err != nil || string(head) != "refs/heads/main\n" {
		t.Fatalf("HEAD changed: %q, %v", head, err)
	}
}

func TestUpdateIndexUnstagesRenameAndKeepsOtherChanges(t *testing.T) {
	dir := t.TempDir()
	g := New()
	ctx := context.Background()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return string(out)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("old.txt", "initial\n")
	write("other.txt", "before\n")
	git("add", "--all")
	git("commit", "-m", "Initial")
	head := git("rev-parse", "HEAD")
	git("mv", "--", "old.txt", "new.txt")
	write("new.txt", "unstaged contents\n")
	write("other.txt", "staged separately\n")
	git("add", "--", "other.txt")
	snapshot, err := g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	var rename domain.File
	for _, file := range snapshot.Staged {
		if file.Path == "new.txt" {
			rename = file
		}
	}
	if rename.Status != "R" || rename.OldPath != "old.txt" {
		t.Fatalf("expected staged rename, got %+v", rename)
	}
	if err := g.UpdateIndex(ctx, snapshot, false, []domain.File{rename}); err != nil {
		t.Fatal(err)
	}
	result, err := g.ReadRepository(ctx, dir)
	if err != nil || len(result.Staged) != 1 || result.Staged[0].Path != "other.txt" {
		t.Fatalf("unrelated index entries changed: %+v, %v", result.Staged, err)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "new.txt")); err != nil || string(content) != "unstaged contents\n" {
		t.Fatalf("worktree content changed: %q, %v", content, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old.txt")); !errors.Is(err, os.ErrNotExist) || git("rev-parse", "HEAD") != head {
		t.Fatal("unstage restored a working path or changed HEAD")
	}
}
