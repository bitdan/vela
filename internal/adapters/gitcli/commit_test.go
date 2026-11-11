package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitUsesIndexAndAmendsLocalHead(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	g := New()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
		return string(output)
	}
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("config", "user.name", "Vela Test")
	git("config", "user.email", "vela@example.test")
	git("config", "commit.gpgsign", "false")
	write("file.txt", "index version\n")
	git("add", "--", "file.txt")
	write("file.txt", "working version\n")
	write("other.txt", "untracked\n")
	snapshot, err := g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	git("add", "--", "other.txt")
	if err := g.Commit(ctx, snapshot, "stale", false); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale index was accepted: %v", err)
	}
	git("rm", "--cached", "--", "other.txt")
	if err := g.Commit(ctx, snapshot, "初始提交\n\nBody", false); err != nil {
		t.Fatal(err)
	}
	if git("show", "HEAD:file.txt") != "index version\n" || git("ls-tree", "--name-only", "HEAD") != "file.txt\n" {
		t.Fatal("commit included content outside the index")
	}
	if git("log", "-1", "--format=%B") != "初始提交\n\nBody\n\n" {
		t.Fatal("multiline commit message did not round-trip")
	}
	snapshot, err = g.ReadRepository(ctx, dir)
	if err != nil || len(snapshot.Working) != 2 || len(snapshot.Staged) != 0 {
		t.Fatalf("unexpected post-commit snapshot: %v %v %v", snapshot.Working, snapshot.Staged, err)
	}
	if err := g.Commit(ctx, snapshot, "empty", false); err == nil {
		t.Fatal("created an empty commit")
	}
	if err := g.Commit(ctx, snapshot, "Reword the local commit", true); err != nil {
		t.Fatal(err)
	}
	if git("rev-list", "--count", "HEAD") != "1\n" || git("log", "-1", "--format=%s") != "Reword the local commit\n" ||
		git("show", "HEAD:file.txt") != "index version\n" || strings.TrimSpace(git("rev-parse", "HEAD")) == snapshot.Head {
		t.Fatal("amend failed to replace the last commit without changing its tree")
	}
	git("add", "--", "file.txt")
	write("file.txt", "later working edit\n")
	snapshot, err = g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Commit(ctx, snapshot, "Amend with a staged change", true); err != nil {
		t.Fatal(err)
	}
	if git("rev-list", "--count", "HEAD") != "1\n" || git("show", "HEAD:file.txt") != "working version\n" {
		t.Fatal("amend did not replace HEAD with the staged version")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "file.txt")); err != nil || string(data) != "later working edit\n" {
		t.Fatalf("amend changed the working file: %q, %v", data, err)
	}
	remote := filepath.ToSlash(filepath.Join(t.TempDir(), "remote.git"))
	git("init", "--bare", remote)
	git("remote", "add", "upstream", remote)
	git("push", "--set-upstream", "upstream", "main")
	snapshot, err = g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.Commit(ctx, snapshot, "Rewrite published", true); !errors.Is(err, ErrPublishedAmend) {
		t.Fatalf("published commit amend was accepted: %v", err)
	}
}

func TestCommitFailureKeepsHeadAndIndex(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	g := New()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	git("config", "user.name", "Vela Test")
	git("config", "user.email", "vela@example.test")
	git("config", "commit.gpgsign", "false")
	git("config", "core.hooksPath", ".git/hooks")
	hooks := filepath.Join(dir, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\necho 'test hook rejected' >&2\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", "file.txt")
	snapshot, err := g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := g.Commit(canceled, snapshot, "Canceled", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled commit, got %v", err)
	}
	if err := g.Commit(ctx, snapshot, "Rejected by hook", false); err == nil || !strings.Contains(err.Error(), "test hook rejected") {
		t.Fatalf("expected a hook failure, got %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git", "rebase-merge"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := g.Commit(ctx, snapshot, "While rebasing", false); err == nil || !strings.Contains(err.Error(), "in-progress Git operation") {
		t.Fatalf("expected in-progress operation rejection, got %v", err)
	}
	if err := os.Remove(filepath.Join(dir, ".git", "rebase-merge")); err != nil {
		t.Fatal(err)
	}
	after, err := g.ReadRepository(ctx, dir)
	if err != nil || after.Head != snapshot.Head || after.IndexVersion != snapshot.IndexVersion {
		t.Fatalf("failed commits modified the checkout: head=%q index=%q error=%v", after.Head, after.IndexVersion, err)
	}
}
