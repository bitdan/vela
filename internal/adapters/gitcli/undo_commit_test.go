package gitcli

import (
	"context"
	"errors"
	"testing"

	"vela/internal/domain"
)

func TestUndoCommitKeepsIndexAndWorktree(t *testing.T) {
	f, ids := squashFixture(t)
	ctx := context.Background()
	r := f.read()
	before := f.git("write-tree")
	f.write("untracked 中文.txt", "untouched\n")
	r = f.read()
	if _, err := f.g.PreviewCommitRewrite(ctx, r, domain.UndoCommit, ids[1:2]); err == nil {
		t.Fatal("accepted history before HEAD")
	}
	plan, err := f.g.PreviewCommitRewrite(ctx, r, domain.UndoCommit, ids[4:])
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.UndoCommit, RewriteSelection: ids[4:], RewriteVersion: plan.Version, ConfirmRewrite: true}
	bad := op
	bad.ConfirmRewrite = false
	if err := f.g.ExecuteRepositoryOperation(ctx, r, bad); err == nil {
		t.Fatal("accepted unconfirmed undo")
	}
	bad = op
	bad.Safety.ProtectedBranches = r.CurrentBranch
	if err := f.g.ExecuteRepositoryOperation(ctx, r, bad); err == nil {
		t.Fatal("accepted protected branch")
	}
	f.git("update-ref", "refs/remotes/origin/published", r.Head)
	if err := f.g.ExecuteRepositoryOperation(ctx, r, op); err == nil {
		t.Fatal("accepted published HEAD")
	}
	f.git("update-ref", "-d", "refs/remotes/origin/published")
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := f.g.ExecuteRepositoryOperation(canceled, r, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if f.git("rev-parse", "HEAD") != r.Head || f.git("write-tree") != before {
		t.Fatal("refusals changed repository")
	}
	if err := f.g.ExecuteRepositoryOperation(ctx, r, op); err != nil {
		t.Fatal(err)
	}
	if f.git("rev-parse", "HEAD") != ids[3] || f.git("write-tree") != before || f.git("diff", "--name-only") != "" || f.git("diff", "--cached", "--name-only") == "" {
		t.Fatal("undo did not keep the committed changes staged")
	}
	if after := f.read(); len(after.Working) != 1 || after.Working[0].Path != "untracked 中文.txt" || after.Working[0].Status != "?" {
		t.Fatalf("unexpected working changes: %+v", after.Working)
	}
}
