package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"vela/internal/domain"
)

func TestWorktreeCreateNewBranch(t *testing.T) {
	f := newWorkflow(t)
	base := f.git("rev-parse", "HEAD")
	f.git("commit", "--allow-empty", "-m", "advance")
	before := f.read()
	path := filepath.Join(t.TempDir(), "review 空格")
	// New branch creation must not accidentally inherit tracking defaults.
	f.git("config", "branch.autoSetupMerge", "always")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "New Branch", Branch: "review/测试", Source: base, Path: path}); err != nil {
		t.Fatal(err)
	}
	linked, err := f.g.ReadRepository(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if linked.Head != base || linked.CurrentBranch != "review/测试" || linked.Upstreams["review/测试"].Remote != "" {
		t.Fatalf("wrong new checkout: %s %s %+v", linked.Head, linked.CurrentBranch, linked.Upstreams)
	}
	after := f.read()
	if after.Head != before.Head || after.CurrentBranch != before.CurrentBranch || after.IndexVersion != before.IndexVersion || after.WorktreeVersion != before.WorktreeVersion {
		t.Fatal("creation changed source checkout")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); err != nil {
		t.Fatal(err)
	}
	if f.git("rev-parse", "refs/heads/review/测试") != base {
		t.Fatal("removing checkout lost the new branch")
	}
}

func TestWorktreeNewBranchRejectsUnsafeRequests(t *testing.T) {
	f := newWorkflow(t)
	snapshot := f.read()
	parent := t.TempDir()
	for i, name := range []string{"main", "bad..branch", "-option", ""} {
		path := filepath.Join(parent, "rejected", string(rune('a'+i)))
		err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "New Branch", Branch: name, Source: snapshot.Head, Path: path})
		if err == nil {
			t.Fatalf("unsafe branch %q accepted", name)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("rejected creation left directory: %v", err)
		}
	}
	if f.git("rev-parse", "main") != snapshot.Head {
		t.Fatal("existing branch was overwritten")
	}
	existing := filepath.Join(parent, "existing")
	if err := os.Mkdir(existing, 0700); err != nil {
		t.Fatal(err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "New Branch", Branch: "must-not-exist", Source: snapshot.Head, Path: existing}); err == nil {
		t.Fatal("existing directory accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ExecuteRepositoryOperation(ctx, snapshot, domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "New Branch", Branch: "canceled", Source: snapshot.Head, Path: filepath.Join(parent, "canceled")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled = %v", err)
	}
	after := f.read()
	if after.Refs["refs/heads/must-not-exist"] != "" || after.Refs["refs/heads/canceled"] != "" || len(after.Worktrees) != 1 {
		t.Fatal("rejected operation created refs or worktrees")
	}
}

func TestWorktreeNewBranchRejectsMovedSource(t *testing.T) {
	f := newWorkflow(t)
	f.git("branch", "source")
	snapshot := f.read()
	f.git("commit", "--allow-empty", "-m", "advance")
	f.git("branch", "-f", "source", "HEAD")
	// Preserve the source-checkout identity while changing only the selected ref.
	fresh := f.read()
	snapshot.Head, snapshot.IndexVersion = fresh.Head, fresh.IndexVersion
	path := filepath.Join(t.TempDir(), "stale")
	err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "New Branch", Branch: "review", Source: "refs/heads/source", Path: path})
	if !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale source = %v", err)
	}
	if f.read().Refs["refs/heads/review"] != "" {
		t.Fatal("stale source created branch")
	}
}
