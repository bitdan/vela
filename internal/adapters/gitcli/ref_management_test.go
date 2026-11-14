package gitcli

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"vela/internal/domain"
)

func TestRefManagementRoundtrip(t *testing.T) {
	f := newWorkflow(t)
	f.git("branch", "topic")
	beforeHead, beforeIndex, beforeWork := f.git("rev-parse", "HEAD"), f.git("ls-files", "--stage"), f.git("status", "--porcelain=v2")
	if err := f.run(domain.RepositoryOperation{Kind: domain.RenameBranch, Source: "refs/heads/topic", Branch: "feature/中文"}); err != nil {
		t.Fatal(err)
	}
	if f.git("rev-parse", "refs/heads/feature/中文") != beforeHead {
		t.Fatal("rename changed branch tip")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateTag, Source: "refs/heads/feature/中文", Target: "release/测试"}); err != nil {
		t.Fatal(err)
	}
	if f.git("cat-file", "-t", "refs/tags/release/测试") != "commit" {
		t.Fatal("tag was not lightweight")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.DeleteBranch, Source: "refs/heads/feature/中文"}); err != nil {
		t.Fatal(err)
	}
	if f.git("branch", "--list", "feature/中文") != "" || f.git("rev-parse", "HEAD") != beforeHead || f.git("ls-files", "--stage") != beforeIndex || f.git("status", "--porcelain=v2") != beforeWork {
		t.Fatal("ref operation changed checkout")
	}
	f.write("文件 name.txt", "dirty user edit\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.RenameBranch, Source: "refs/heads/main", Branch: "renamed-main"}); err != nil {
		t.Fatal(err)
	}
	if f.git("symbolic-ref", "--short", "HEAD") != "renamed-main" || f.git("diff", "--numstat") == "" {
		t.Fatal("current rename lost working tree")
	}
}

func TestRefManagementSafety(t *testing.T) {
	f := newWorkflow(t)
	f.git("branch", "topic")
	for _, op := range []domain.RepositoryOperation{
		{Kind: domain.DeleteBranch, Source: "refs/heads/main"},
		{Kind: domain.RenameBranch, Source: "refs/heads/topic", Branch: "main"},
		{Kind: domain.RenameBranch, Source: "refs/heads/topic", Branch: "--force"},
		{Kind: domain.CreateTag, Source: "refs/heads/main", Target: "--evil"},
		{Kind: domain.CreateTag, Source: "refs/heads/main", Target: "bad name"},
	} {
		if err := f.run(op); err == nil {
			t.Fatalf("unsafe ref operation accepted %+v", op)
		}
	}
	f.git("config", "tag.gpgSign", "true")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateTag, Source: "refs/heads/main", Target: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateTag, Source: "refs/heads/main", Target: "v1"}); err == nil {
		t.Fatal("existing tag replaced")
	}
	snapshot := f.read()
	f.git("update-ref", "refs/heads/topic", "HEAD")
	f.git("checkout", "topic")
	f.git("commit", "--allow-empty", "-m", "not merged")
	f.git("checkout", "main")
	if err := f.run(domain.RepositoryOperation{Kind: domain.DeleteBranch, Source: "refs/heads/topic"}); err == nil {
		t.Fatal("unmerged branch deleted")
	}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.RenameBranch, Source: "refs/heads/topic", Branch: "stale"}); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale source accepted: %v", err)
	}
	linked := filepath.Join(t.TempDir(), "linked")
	f.git("worktree", "add", linked, "topic")
	for _, kind := range []domain.RepositoryOperationKind{domain.RenameBranch, domain.DeleteBranch} {
		if err := f.run(domain.RepositoryOperation{Kind: kind, Source: "refs/heads/topic", Branch: "renamed"}); err == nil {
			t.Fatal("branch in another worktree changed")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), domain.RepositoryOperation{Kind: domain.CreateTag, Source: "refs/heads/main", Target: "canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
}
