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

func TestRebaseConflictRecovery(t *testing.T) {
	t.Parallel()
	ctx, dir, g := context.Background(), t.TempDir(), New()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	path := "中文 file.txt"
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("config", "user.name", "Vela Test")
	git("config", "user.email", "vela@example.test")
	git("config", "commit.gpgsign", "false")
	write("base")
	git("add", "--", path)
	git("commit", "-m", "base")
	git("checkout", "-b", "topic")
	write("topic")
	git("commit", "-am", "topic")
	original := git("rev-parse", "HEAD")
	git("checkout", "main")
	write("onto")
	git("commit", "-am", "onto")
	onto := git("rev-parse", "HEAD")
	git("checkout", "topic")
	read := func() domain.Repository {
		t.Helper()
		repo, err := g.ReadRepository(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	run := func(kind domain.RepositoryOperationKind, source string) error {
		return g.ExecuteRepositoryOperation(ctx, read(), domain.RepositoryOperation{Kind: kind, Source: source})
	}
	start := func() domain.Repository {
		t.Helper()
		if err := run(domain.StartRebase, "refs/heads/main"); err == nil {
			t.Fatal("expected a conflict")
		}
		repo := read()
		if repo.Rebase.Branch != "topic" || repo.Rebase.Onto != onto || len(repo.Conflicts) != 1 {
			t.Fatalf("wrong rebase state: %+v", repo.Rebase)
		}
		return repo
	}
	start()
	if err := run(domain.ContinueRebase, ""); err == nil {
		t.Fatal("continued with an unresolved file")
	}
	if err := run(domain.AbortRebase, ""); err != nil || git("rev-parse", "HEAD") != original {
		t.Fatalf("abort did not restore the original tip: %v", err)
	}
	repo := start()
	doc, err := g.ReadMergeDocument(ctx, repo, path)
	if err != nil || doc.Current != "onto\n" || doc.Incoming != "topic\n" {
		t.Fatalf("wrong rebase conflict sides: %+v, %v", doc, err)
	}
	if err := run(domain.AcceptIncoming, path); err != nil {
		t.Fatal(err)
	}
	if err := run(domain.ContinueRebase, ""); err != nil || git("rev-parse", "HEAD^") != onto || git("show", "HEAD:"+path) != "topic" {
		t.Fatalf("continue did not replay the resolution: %v", err)
	}
	git("checkout", "-b", "skip-topic", original)
	if err := run(domain.StartRebase, "refs/heads/main"); err == nil {
		t.Fatal("expected a conflict before skip")
	}
	if err := run(domain.SkipRebase, ""); err != nil || git("rev-parse", "HEAD") != onto {
		t.Fatalf("skip did not drop the replayed commit: %v", err)
	}
	git("checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "generated.txt"), []byte("tracked upstream\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", "generated.txt")
	git("commit", "-m", "upstream adds generated file")
	git("checkout", "skip-topic")
	exclude := filepath.Join(dir, ".git", "info")
	if err := os.MkdirAll(exclude, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(exclude, "exclude"), []byte("generated.txt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(dir, "generated.txt")
	if err := os.WriteFile(generated, []byte("local generated file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(domain.StartRebase, "refs/heads/main"); err == nil || !strings.Contains(err.Error(), "ignored") {
		t.Fatalf("ignored overwrite was not blocked: %v", err)
	}
	if data, err := os.ReadFile(generated); err != nil || string(data) != "local generated file\n" {
		t.Fatalf("ignored file changed: %q %v", data, err)
	}
	if err := os.Remove(generated); err != nil {
		t.Fatal(err)
	}
	if err := run(domain.StartRebase, "refs/heads/main"); err != nil || git("rev-parse", "HEAD") != git("rev-parse", "main") {
		t.Fatalf("clean rebase failed: %v", err)
	}
}
