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

func TestFixupKeepsOldestMessageAndTree(t *testing.T) {
	for _, start := range []int{0, 1, 3} {
		t.Run([]string{"root", "middle", "", "HEAD"}[start], func(t *testing.T) {
			t.Parallel()
			f, ids := squashFixture(t)
			ctx := context.Background()
			r := f.read()
			tree := f.git("write-tree")
			raw, _ := f.g.read(ctx, f.root, "cat-file", "commit", ids[start])
			_, message, _ := strings.Cut(raw, "\n\n")
			selected := ids[start : start+2]
			p, err := f.g.PreviewCommitRewrite(ctx, r, domain.FixupCommits, selected)
			if err != nil {
				t.Fatal(err)
			}
			if p.Message != message {
				t.Fatal("preview modified original message bytes")
			}
			op := domain.RepositoryOperation{Kind: domain.FixupCommits, RewriteSelection: selected, RewriteVersion: p.Version, ConfirmRewrite: true, Message: "must ignore edited message"}
			if err = f.g.ExecuteRepositoryOperation(ctx, r, op); err != nil {
				t.Fatal(err)
			}
			fixed := f.git("rev-parse", "HEAD~"+[]string{"3", "2", "1", "0"}[start])
			raw, _ = f.g.read(ctx, f.root, "cat-file", "commit", fixed)
			_, actual, _ := strings.Cut(raw, "\n\n")
			if actual != message || f.git("write-tree") != tree || f.git("rev-parse", "HEAD^{tree}") != tree || f.git("rev-list", "--count", "HEAD") != "4" {
				t.Fatal("Fixup lost message/tree/count")
			}
			if f.git("show", "-s", "--format=%an <%ae> %aI", fixed) != f.git("show", "-s", "--format=%an <%ae> %aI", ids[start]) {
				t.Fatal("original author lost")
			}
		})
	}
}
func TestFixupExactWhitespaceAndGuards(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	ctx := context.Background()
	f.git("checkout", "-b", "feature")
	f.git("commit", "--allow-empty", "--cleanup=verbatim", "-m", " \nOriginal 中文\n\nbody\n\n")
	first := f.git("rev-parse", "HEAD")
	f.git("commit", "--allow-empty", "-m", "fixup content")
	r := f.read()
	p, err := f.g.PreviewCommitRewrite(ctx, r, domain.FixupCommits, []string{first, r.Head})
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.FixupCommits, RewriteSelection: []string{first, r.Head}, RewriteVersion: p.Version, ConfirmRewrite: true}
	if _, err = f.g.PreviewCommitRewrite(ctx, r, domain.FixupCommits, []string{first}); err == nil {
		t.Fatal("single Fixup accepted")
	}
	bad := op
	bad.RewriteVersion = "reword:" + p.Version
	if f.g.ExecuteRepositoryOperation(ctx, r, bad) == nil {
		t.Fatal("wrong operation identity accepted")
	}
	if err = f.g.ExecuteRepositoryOperation(ctx, r, op); err != nil {
		t.Fatal(err)
	}
	raw, _ := f.g.read(ctx, f.root, "cat-file", "commit", "HEAD")
	_, message, _ := strings.Cut(raw, "\n\n")
	if message != p.Message || !strings.HasPrefix(message, " \n") || !strings.HasSuffix(message, "\n\n") {
		t.Fatalf("verbatim message lost: %q", message)
	}
}

func TestFixupNativeConflictContinueAndAbort(t *testing.T) {
	for _, abort := range []bool{false, true} {
		t.Run(map[bool]string{false: "continue", true: "abort"}[abort], func(t *testing.T) {
			t.Parallel()
			f, ids := squashFixture(t)
			ctx := context.Background()
			repo := f.read()
			tree := f.git("write-tree")
			// A real post-checkout hook changes the newly checked-out replay base. This
			// induces a native Git conflict rather than fabricating rebase metadata.
			marker := filepath.ToSlash(filepath.Join(t.TempDir(), "hook-used"))
			quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
			script := "#!/bin/sh\nif test ! -e " + quote(marker) + "; then\n : > " + quote(marker) + "\n printf 'external\\n' > '文件 name.txt'\n git add -- '文件 name.txt'\n git -c core.hooksPath=/dev/null commit --amend --no-edit --no-verify\nfi\n"
			if err := os.WriteFile(filepath.Join(f.root, ".git", "hooks", "post-checkout"), []byte(script), 0755); err != nil {
				t.Fatal(err)
			}
			selected := []string{ids[1], ids[2]}
			plan, err := f.g.PreviewCommitRewrite(ctx, repo, domain.FixupCommits, selected)
			if err != nil {
				t.Fatal(err)
			}
			err = f.g.ExecuteRepositoryOperation(ctx, repo, domain.RepositoryOperation{Kind: domain.FixupCommits, RewriteSelection: selected, RewriteVersion: plan.Version, ConfirmRewrite: true})
			paused := f.read()
			if err != nil && paused.Rebase.Active() && len(paused.Conflicts) == 0 {
				// Git detects the hook's index/HEAD change before replaying. Align
				// that real checkout, then let Continue hit the conflicting patch.
				f.git("reset", "--mixed", "HEAD")
				err = f.g.ExecuteRepositoryOperation(ctx, f.read(), domain.RepositoryOperation{Kind: domain.ContinueRebase})
				paused = f.read()
			}
			if err == nil || !paused.Rebase.Active() || len(paused.Conflicts) == 0 {
				t.Fatalf("expected native paused rebase conflict: %v %+v", err, paused.Rebase)
			}
			if paused.Rebase.Original != repo.Head {
				t.Fatal("recovery lost original HEAD")
			}
			if abort {
				if err := f.g.ExecuteRepositoryOperation(ctx, paused, domain.RepositoryOperation{Kind: domain.AbortRebase}); err != nil {
					t.Fatal(err)
				}
				if f.git("rev-parse", "HEAD") != repo.Head || f.git("write-tree") != tree {
					t.Fatal("abort failed to restore original checkout")
				}
			} else {
				f.write("文件 name.txt", "third\n")
				f.git("add", "--", "文件 name.txt")
				if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), domain.RepositoryOperation{Kind: domain.ContinueRebase}); err != nil {
					t.Fatal(err)
				}
				if f.git("rev-parse", "HEAD^{tree}") != tree || f.read().Rebase.Active() {
					t.Fatal("continue failed to replay remaining commits")
				}
			}
		})
	}
}

func TestFixupPrepublishCancelAndProtection(t *testing.T) {
	t.Parallel()
	f, ids := squashFixture(t)
	r := f.read()
	ctx := context.Background()
	tree := f.git("write-tree")
	p, err := f.g.PreviewCommitRewrite(ctx, r, domain.FixupCommits, ids[3:])
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.FixupCommits, RewriteSelection: ids[3:], RewriteVersion: p.Version, ConfirmRewrite: true, Safety: domain.RepositorySafetyPolicy{ProtectedBranches: "feature"}}
	if f.g.ExecuteRepositoryOperation(ctx, r, op) == nil {
		t.Fatal("protected Fixup executed")
	}
	op.Safety.ProtectedBranches = ""
	parent, cancel := context.WithCancel(ctx)
	active := application.WithTaskProgress(parent, func(phase string) {
		if phase == "Publishing rewritten HEAD" {
			cancel()
		}
	})
	err = f.g.ExecuteRepositoryOperation(active, r, op)
	cancel()
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancel lost", err)
	}
	if f.git("rev-parse", "HEAD") != r.Head || f.git("write-tree") != tree {
		t.Fatal("canceled Fixup changed checkout")
	}
}
