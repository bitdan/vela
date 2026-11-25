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

func TestDropHeadAndMiddleIndependentCommit(t *testing.T) {
	for _, middle := range []bool{false, true} {
		t.Run(map[bool]string{false: "HEAD", true: "middle"}[middle], func(t *testing.T) {
			t.Parallel()
			f := newWorkflow(t)
			ctx := context.Background()
			f.git("checkout", "-b", "feature")
			base := f.git("rev-parse", "HEAD")
			f.write("drop 中文.txt", "remove this\n")
			f.git("add", ".")
			f.git("commit", "-m", "drop me")
			drop := f.git("rev-parse", "HEAD")
			f.git("tag", "keep-original", drop)
			if middle {
				f.write("later.txt", "retain this\n")
				f.git("add", ".")
				f.git("commit", "-m", "later")
			}
			r := f.read()
			p, err := f.g.PreviewCommitRewrite(ctx, r, domain.DropCommit, []string{drop})
			if err != nil {
				t.Fatal(err)
			}
			if len(p.Affected) != 1 || p.Affected[0].Path != "drop 中文.txt" {
				t.Fatal("missing affected paths")
			}
			op := domain.RepositoryOperation{Kind: domain.DropCommit, RewriteSelection: []string{drop}, RewriteVersion: p.Version, ConfirmRewrite: true}
			if err = f.g.ExecuteRepositoryOperation(ctx, r, op); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(filepath.Join(f.root, "drop 中文.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("dropped file retained", err)
			}
			if middle {
				if f.git("show", "HEAD:later.txt") != "retain this" || f.git("rev-parse", "HEAD^1") != base {
					t.Fatal("descendant replay lost")
				}
			} else if f.git("rev-parse", "HEAD") != base {
				t.Fatal("HEAD was not dropped")
			}
			if f.git("rev-parse", "refs/tags/keep-original") != drop || len(f.read().Working) != 0 || len(f.read().Staged) != 0 {
				t.Fatal("other refs or clean checkout changed")
			}
		})
	}
}
func TestDropNativeConflictContinueAbort(t *testing.T) {
	for _, abort := range []bool{false, true} {
		t.Run(map[bool]string{false: "continue", true: "abort"}[abort], func(t *testing.T) {
			t.Parallel()
			f, ids := squashFixture(t)
			ctx := context.Background()
			r := f.read()
			tree := f.git("write-tree")
			p, err := f.g.PreviewCommitRewrite(ctx, r, domain.DropCommit, ids[2:3])
			if err != nil {
				t.Fatal(err)
			}
			op := domain.RepositoryOperation{Kind: domain.DropCommit, RewriteSelection: ids[2:3], RewriteVersion: p.Version, ConfirmRewrite: true}
			err = f.g.ExecuteRepositoryOperation(ctx, r, op)
			paused := f.read()
			if err == nil || !paused.Rebase.Active() || len(paused.Conflicts) == 0 || paused.Rebase.Original != r.Head {
				t.Fatal("missing native recovery", err, paused.Rebase)
			}
			if abort {
				if err = f.g.ExecuteRepositoryOperation(ctx, paused, domain.RepositoryOperation{Kind: domain.AbortRebase}); err != nil {
					t.Fatal(err)
				}
				if f.git("rev-parse", "HEAD") != r.Head || f.git("write-tree") != tree {
					t.Fatal("Abort did not restore original")
				}
			} else {
				f.write("文件 name.txt", "third\n")
				f.git("add", ".")
				if err = f.g.ExecuteRepositoryOperation(ctx, f.read(), domain.RepositoryOperation{Kind: domain.ContinueRebase}); err != nil {
					t.Fatal(err)
				}
				if f.read().Rebase.Active() || f.git("show", "HEAD:文件 name.txt") != "fourth" || f.git("rev-list", "--count", "HEAD") != "4" {
					t.Fatal("Continue lost descendants")
				}
			}
		})
	}
}
func TestDropRejectsRootAndCancel(t *testing.T) {
	t.Parallel()
	f, ids := squashFixture(t)
	r := f.read()
	ctx := context.Background()
	tree := f.git("write-tree")
	if _, err := f.g.PreviewCommitRewrite(ctx, r, domain.DropCommit, ids[:1]); err == nil {
		t.Fatal("root drop accepted")
	}
	p, err := f.g.PreviewCommitRewrite(ctx, r, domain.DropCommit, ids[4:])
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.DropCommit, RewriteSelection: ids[4:], RewriteVersion: p.Version, ConfirmRewrite: true}
	bad := op
	bad.Safety.ProtectedBranches = "feature"
	if f.g.ExecuteRepositoryOperation(ctx, r, bad) == nil {
		t.Fatal("protected drop")
	}
	bad = op
	bad.ConfirmRewrite = false
	if f.g.ExecuteRepositoryOperation(ctx, r, bad) == nil {
		t.Fatal("unconfirmed drop")
	}
	parent, cancel := context.WithCancel(ctx)
	active := application.WithTaskProgress(parent, func(phase string) {
		if strings.Contains(phase, "Dropping selected commit") {
			cancel()
		}
	})
	err = f.g.ExecuteRepositoryOperation(active, r, op)
	cancel()
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if f.git("rev-parse", "HEAD") != r.Head || f.git("write-tree") != tree {
		t.Fatal("cancel changed checkout")
	}
}

func TestDropProtectsUntrackedAndPublished(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	ctx := context.Background()
	f.git("checkout", "-b", "feature")
	base := f.git("rev-parse", "HEAD")
	f.git("rm", "--", "文件 name.txt")
	f.git("commit", "-m", "delete file")
	f.write("文件 name.txt", "untracked user input\n")
	r := f.read()
	p, err := f.g.PreviewCommitRewrite(ctx, r, domain.DropCommit, []string{r.Head})
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.DropCommit, RewriteSelection: []string{r.Head}, RewriteVersion: p.Version, ConfirmRewrite: true}
	if f.g.ExecuteRepositoryOperation(ctx, r, op) == nil {
		t.Fatal("untracked file overwritten")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "untracked user input\n" || f.git("rev-parse", "HEAD") != r.Head {
		t.Fatal("untracked data lost")
	}
	f.git("update-ref", "refs/remotes/origin/published", r.Head)
	if _, err = f.g.PreviewCommitRewrite(ctx, f.read(), domain.DropCommit, []string{r.Head}); err == nil {
		t.Fatal("published Drop allowed")
	}
	f.git("update-ref", "-d", "refs/remotes/origin/published")
	f.git("update-ref", "refs/remotes/origin/unrelated", base)
	if f.g.ExecuteRepositoryOperation(ctx, f.read(), op) == nil {
		t.Fatal("stale preview allowed")
	}
}
