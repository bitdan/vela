package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRewordRejectsGraftedAncestry(t *testing.T) {
	f, ids := squashFixture(t)
	ctx := context.Background()
	repo := f.read()
	selection := ids[4:]
	preview, err := f.g.PreviewCommitRewrite(ctx, repo, domain.RewordCommit, selection)
	if err != nil {
		t.Fatal(err)
	}
	grafts := filepath.Join(repo.CommonDir, "info", "grafts")
	if err := os.WriteFile(grafts, []byte(repo.Head+" "+ids[0]+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := f.git("write-tree")
	if _, err := f.g.PreviewCommitRewrite(ctx, f.read(), domain.RewordCommit, selection); err == nil {
		t.Error("accepted grafted ancestry during preview")
	}
	op := domain.RepositoryOperation{Kind: domain.RewordCommit, RewriteSelection: selection, RewriteVersion: preview.Version, ConfirmRewrite: true, Message: "changed"}
	if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); err == nil {
		t.Error("accepted grafted ancestry after confirmation")
	}
	if f.git("rev-parse", "HEAD") != repo.Head || f.git("write-tree") != before || len(f.read().Working) != 0 {
		t.Fatal("rejected rewrite changed HEAD, index or working files")
	}
}

func TestRewordRejectsOptionLikeCurrentBranch(t *testing.T) {
	f, ids := squashFixture(t)
	f.git("update-ref", "refs/heads/-i", ids[4])
	f.git("symbolic-ref", "HEAD", "refs/heads/-i")
	_, err := f.g.PreviewCommitRewrite(context.Background(), f.read(), domain.RewordCommit, ids[1:2])
	if err == nil || !strings.Contains(err.Error(), "valid branch name") {
		t.Fatalf("preview did not reject option-like branch: %v", err)
	}
}

func TestRewordPreservesTreeAuthorAndDescendants(t *testing.T) {
	for _, n := range []int{0, 1, 4} {
		t.Run([]string{"root", "middle", "", "", "HEAD"}[n], func(t *testing.T) {
			t.Parallel()
			f, ids := squashFixture(t)
			r := f.read()
			tree := f.git("write-tree")
			author := f.git("show", "-s", "--format=%an <%ae> %aI", ids[n])
			count := f.git("rev-list", "--count", "HEAD")
			p, err := f.g.PreviewCommitRewrite(context.Background(), r, domain.RewordCommit, []string{ids[n]})
			if err != nil {
				t.Fatal(err)
			}
			op := domain.RepositoryOperation{Kind: domain.RewordCommit, RewriteSelection: []string{ids[n]}, RewriteVersion: p.Version, ConfirmRewrite: true, Message: "changed 中文\n\nbody"}
			if err = f.g.ExecuteRepositoryOperation(context.Background(), r, op); err != nil {
				t.Fatal(err)
			}
			newID := f.git("rev-parse", "HEAD~"+[]string{"4", "3", "2", "1", "0"}[n])
			if newID == ids[n] || f.git("show", "-s", "--format=%B", newID) != op.Message || f.git("show", "-s", "--format=%an <%ae> %aI", newID) != author {
				t.Fatal("message or original author lost")
			}
			if f.git("write-tree") != tree || f.git("rev-parse", "HEAD^{tree}") != tree || f.git("rev-list", "--count", "HEAD") != count || len(f.read().Working) != 0 {
				t.Fatal("reword changed tree/index/worktree/count")
			}
		})
	}
}
func TestRewordRejectsUnsafeAndCanceled(t *testing.T) {
	t.Parallel()
	f, ids := squashFixture(t)
	r := f.read()
	ctx := context.Background()
	p, err := f.g.PreviewCommitRewrite(ctx, r, domain.RewordCommit, ids[1:2])
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.RewordCommit, RewriteSelection: ids[1:2], RewriteVersion: p.Version, ConfirmRewrite: true, Message: "changed"}
	head, tree := r.Head, f.git("write-tree")
	bad := op
	bad.ConfirmRewrite = false
	if f.g.ExecuteRepositoryOperation(ctx, r, bad) == nil {
		t.Fatal("unconfirmed rewrite")
	}
	bad = op
	bad.Message = " "
	if f.g.ExecuteRepositoryOperation(ctx, r, bad) == nil {
		t.Fatal("blank message")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if e := f.g.ExecuteRepositoryOperation(canceled, r, op); !errors.Is(e, context.Canceled) {
		t.Fatal("cancel lost", e)
	}
	f.git("update-ref", "refs/remotes/origin/published", ids[4])
	if f.g.ExecuteRepositoryOperation(ctx, r, op) == nil {
		t.Fatal("published descendants rewritten")
	}
	f.git("update-ref", "-d", "refs/remotes/origin/published")
	f.write("文件 name.txt", "dirty\n")
	if _, err = f.g.PreviewCommitRewrite(ctx, f.read(), domain.RewordCommit, ids[1:2]); err == nil {
		t.Fatal("dirty reword preview")
	}
	if f.git("rev-parse", "HEAD") != head || f.git("write-tree") != tree {
		t.Fatal("failed rewrite changed Git state")
	}
	if _, err = f.g.PreviewCommitRewrite(ctx, r, domain.RewordCommit, []string{strings.Repeat("f", 40)}); err == nil {
		t.Fatal("nonmember accepted")
	}
}

func TestRewordPublicationGuards(t *testing.T) {
	t.Parallel()
	f, ids := squashFixture(t)
	r := f.read()
	tree := f.git("write-tree")
	selection := ids[4:]
	p, err := f.g.PreviewCommitRewrite(context.Background(), r, domain.RewordCommit, selection)
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.RewordCommit, RewriteSelection: selection, RewriteVersion: p.Version, ConfirmRewrite: true, Message: "changed"}
	protected := op
	protected.Safety.ProtectedBranches = "feature"
	if f.g.ExecuteRepositoryOperation(context.Background(), r, protected) == nil {
		t.Fatal("protected rewritten")
	}
	f.git("update-ref", "refs/remotes/origin/unrelated", ids[0])
	if f.g.ExecuteRepositoryOperation(context.Background(), f.read(), op) == nil {
		t.Fatal("stale remote refs accepted")
	}
	f.git("update-ref", "-d", "refs/remotes/origin/unrelated")
	for _, phase := range []string{"Creating rewritten commit", "Publishing rewritten HEAD"} {
		parent, cancel := context.WithCancel(context.Background())
		ctx := application.WithTaskProgress(parent, func(step string) {
			if phase == step {
				cancel()
			}
		})
		err := f.g.ExecuteRepositoryOperation(ctx, r, op)
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel checkpoint %s: %v", phase, err)
		}
		if f.git("rev-parse", "HEAD") != r.Head || f.git("write-tree") != tree {
			t.Fatal("prepublish cancellation moved checkout")
		}
	}
}

func TestRewordCanReplaceOriginallyEmptyMessage(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("checkout", "-b", "feature")
	f.git("commit", "--allow-empty", "--allow-empty-message", "-m", "")
	r := f.read()
	if _, err := f.g.PreviewCommitRewrite(context.Background(), r, domain.RewordCommit, []string{r.Head}); err != nil {
		t.Fatal("existing empty message must remain editable", err)
	}
}
