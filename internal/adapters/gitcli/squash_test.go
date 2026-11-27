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

func squashFixture(t *testing.T) (*workflowFixture, []string) {
	t.Helper()
	f := newWorkflow(t)
	f.git("checkout", "-b", "feature")
	ids := []string{f.git("rev-parse", "HEAD")}
	for _, text := range []string{"first", "second", "third", "fourth"} {
		f.write("文件 name.txt", text+"\n")
		if text == "first" {
			f.git("-c", "user.name=Oldest Author", "commit", "--date=2001-01-01T12:00:00+0000", "-am", text)
		} else {
			f.git("commit", "-am", text)
		}
		ids = append(ids, f.git("rev-parse", "HEAD"))
	}
	return f, ids
}

func TestSquashNativeConflictContinueAndAbort(t *testing.T) {
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
			plan, err := f.g.PreviewSquash(ctx, repo, selected)
			if err != nil {
				t.Fatal(err)
			}
			err = f.g.ExecuteRepositoryOperation(ctx, repo, domain.RepositoryOperation{Kind: domain.SquashCommits, SquashSelection: selected, SquashVersion: plan.Version, ConfirmSquash: true, Message: "combined"})
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

func TestSquashContinuousRangePreservesFinalTree(t *testing.T) {
	for _, kind := range []string{"suffix", "middle", "root"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f, ids := squashFixture(t)
			ctx := context.Background()
			repo := f.read()
			tree := f.git("rev-parse", "HEAD^{tree}")
			index := f.git("write-tree")
			selected := []string{ids[3], ids[4]}
			parent := ids[2]
			wantCount := "4"
			if kind == "middle" {
				selected = []string{ids[1], ids[2]}
				parent = ids[0]
			}
			if kind == "root" {
				selected = []string{ids[0], ids[1]}
				parent = ""
			}
			plan, err := f.g.PreviewSquash(ctx, repo, selected)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Version == "" || len(plan.Selected) != 2 || !strings.Contains(plan.Message, "first") && kind != "suffix" {
				t.Fatalf("invalid preview: %+v", plan)
			}
			op := domain.RepositoryOperation{Kind: domain.SquashCommits, SquashSelection: selected, SquashVersion: plan.Version, ConfirmSquash: true, Message: "combined 中文 message\n\nDetails"}
			if err := f.g.ExecuteRepositoryOperation(ctx, repo, op); err != nil {
				t.Fatal(err)
			}
			if f.git("rev-parse", "HEAD^{tree}") != tree || f.git("write-tree") != index || f.git("status", "--porcelain=v1") != "" || f.git("rev-list", "--count", "HEAD") != wantCount {
				t.Fatal("squash changed final tree/index/worktree or count")
			}
			newID := f.git("log", "--format=%H", "--grep=^combined", "HEAD")
			if newID == "" || f.git("show", "-s", "--format=%B", newID) != "combined 中文 message\n\nDetails" {
				t.Fatal("squash message was not kept")
			}
			parents := strings.Fields(f.git("show", "-s", "--format=%P", newID))
			if parent == "" && len(parents) != 0 || parent != "" && (len(parents) != 1 || parents[0] != parent) {
				t.Fatal("wrong squashed parent")
			}
			expectedAuthor := "Vela Test"
			if kind == "middle" {
				expectedAuthor = "Oldest Author"
			}
			if f.git("show", "-s", "--format=%an", newID) != expectedAuthor || f.git("show", "-s", "--format=%aI", newID) != f.git("show", "-s", "--format=%aI", plan.Selected[len(plan.Selected)-1]) {
				t.Fatal("oldest author lost")
			}
		})
	}
}

func TestSquashRejectsMergeDescendant(t *testing.T) {
	t.Parallel()
	f, ids := squashFixture(t)
	f.git("checkout", "-b", "other", ids[1])
	f.write("other.txt", "separate\n")
	f.git("add", "--", "other.txt")
	f.git("commit", "-m", "other")
	f.git("checkout", "feature")
	f.git("merge", "--no-ff", "other", "-m", "merge descendant")
	repo := f.read()
	if _, err := f.g.PreviewSquash(context.Background(), repo, []string{ids[2], ids[3]}); err == nil {
		t.Fatal("Squash accepted a rewritten merge descendant")
	}
	if f.git("rev-parse", "HEAD") != repo.Head {
		t.Fatal("preview moved HEAD")
	}
}

func TestSquashRejectsPublishedDirtyStaleAndInvalidSelection(t *testing.T) {
	t.Parallel()
	f, ids := squashFixture(t)
	ctx := context.Background()
	repo := f.read()
	selected := []string{ids[1], ids[2]}
	for _, bad := range [][]string{{ids[1]}, {ids[1], ids[3]}, {ids[1], ids[1]}} {
		if _, err := f.g.PreviewSquash(ctx, repo, bad); err == nil {
			t.Fatalf("accepted invalid range %v", bad)
		}
	}
	plan, err := f.g.PreviewSquash(ctx, repo, selected)
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.SquashCommits, SquashSelection: selected, SquashVersion: plan.Version, ConfirmSquash: true, Message: "joined"}
	denied := op
	denied.Safety.ProtectedBranches = "feature"
	if err := f.g.ExecuteRepositoryOperation(ctx, repo, denied); err == nil {
		t.Fatal("protected branch rewritten")
	}
	denied = op
	denied.ConfirmSquash = false
	if err := f.g.ExecuteRepositoryOperation(ctx, repo, denied); err == nil {
		t.Fatal("unconfirmed squash executed")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := f.g.ExecuteRepositoryOperation(canceled, repo, op); err == nil {
		t.Fatal("canceled squash executed")
	}
	f.write("文件 name.txt", "unsaved on disk\n")
	if _, err := f.g.PreviewSquash(ctx, f.read(), selected); err == nil {
		t.Fatal("dirty worktree accepted")
	}
	f.write("文件 name.txt", "fourth\n")
	remote := filepath.Join(t.TempDir(), "remote.git")
	f.git("init", "--bare", remote)
	f.git("remote", "add", "origin", remote)
	f.git("push", "origin", "HEAD:published")
	f.git("fetch", "origin")
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), op); err == nil {
		t.Fatal("published descendant rewritten")
	}
	if f.git("rev-parse", "HEAD") != repo.Head {
		t.Fatal("rejection changed HEAD")
	}
	f.git("push", "origin", ":published")
	f.git("fetch", "--prune", "origin")
	f.git("update-ref", "refs/remotes/origin/unrelated", ids[0])
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), op); err == nil {
		t.Fatal("changed remote-ref preview accepted")
	}
	f.git("update-ref", "-d", "refs/remotes/origin/unrelated")
	f.write("文件 name.txt", "fifth\n")
	f.git("commit", "-am", "new HEAD")
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), op); err == nil {
		t.Fatal("stale HEAD preview accepted")
	}
}

func TestSquashCancelBeforePublishingPreservesCheckout(t *testing.T) {
	t.Parallel()
	f, ids := squashFixture(t)
	repo := f.read()
	selection := []string{ids[3], ids[4]}
	plan, err := f.g.PreviewSquash(context.Background(), repo, selection)
	if err != nil {
		t.Fatal(err)
	}
	tree := f.git("write-tree")
	status := f.git("status", "--porcelain=v1")
	for _, phase := range []string{"Creating squashed commit", "Publishing squashed HEAD"} {
		parent, cancel := context.WithCancel(context.Background())
		ctx := application.WithTaskProgress(parent, func(got string) {
			if got == phase {
				cancel()
			}
		})
		err := f.g.ExecuteRepositoryOperation(ctx, repo, domain.RepositoryOperation{Kind: domain.SquashCommits, SquashSelection: selection, SquashVersion: plan.Version, ConfirmSquash: true, Message: "joined"})
		cancel()
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("%s: %v", phase, err)
		}
		if f.git("rev-parse", "HEAD") != repo.Head || f.git("write-tree") != tree || f.git("status", "--porcelain=v1") != status {
			t.Fatal("pre-publish cancellation changed checkout")
		}
	}
}
