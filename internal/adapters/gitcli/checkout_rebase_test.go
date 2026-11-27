package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"vela/internal/application"
	"vela/internal/domain"
)

func TestCheckoutRebaseDirectionAndRecovery(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name := "success"
		if conflict {
			name = "conflict"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f := newWorkflow(t)
			base := f.git("branch", "--show-current")
			path := "中文 conflict.txt"
			f.write(path, "base\n")
			f.git("add", "--", path)
			f.git("commit", "-m", "base")
			f.git("checkout", "-b", "topic")
			if !conflict {
				path = "topic.txt"
			}
			f.write(path, "topic\n")
			f.git("add", "--", path)
			f.git("commit", "-m", "topic")
			topic := f.git("rev-parse", "HEAD")
			f.git("checkout", base)
			f.write("中文 conflict.txt", "onto\n")
			f.git("commit", "-am", "onto")
			r := f.read()
			op := domain.RepositoryOperation{Kind: domain.CheckoutRebase, Source: "refs/heads/topic", ConfirmRewrite: true}
			run := func() error { return f.g.ExecuteRepositoryOperation(ctx, f.read(), op) }
			if !conflict {
				bad := op
				bad.Safety.ProtectedBranches = "topic"
				if err := f.g.ExecuteRepositoryOperation(ctx, r, bad); err == nil {
					t.Fatal("protected target allowed")
				}
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				if err := f.g.ExecuteRepositoryOperation(canceled, r, op); !errors.Is(err, context.Canceled) {
					t.Fatalf("expected cancellation, got %v", err)
				}
				f.git("update-ref", op.Source, r.Head)
				if err := f.g.ExecuteRepositoryOperation(ctx, r, op); err == nil {
					t.Fatal("accepted changed target")
				}
				f.git("update-ref", op.Source, topic)
				f.git("branch", "unselected", topic)
				f.git("symbolic-ref", op.Source, "refs/heads/unselected")
				if err := f.g.ExecuteRepositoryOperation(ctx, r, op); err == nil || f.git("branch", "--show-current") != base {
					t.Fatal("accepted a symbolic replacement for the selected reference")
				}
				f.git("symbolic-ref", "--delete", op.Source)
				f.git("update-ref", op.Source, topic)
				f.write("中文 conflict.txt", "dirty\n")
				if err := run(); err == nil || f.git("branch", "--show-current") != base {
					t.Fatal("checked out over dirty files")
				}
				f.write("中文 conflict.txt", "onto\n")
				if err := run(); err != nil {
					t.Fatal(err)
				}
			} else {
				for _, recovery := range []domain.RepositoryOperationKind{domain.AbortRebase, domain.ContinueRebase} {
					f.git("checkout", base)
					if err := run(); err == nil {
						t.Fatal("expected a native conflict")
					}
					paused := f.read()
					if paused.Rebase.Original != topic || paused.Rebase.Branch != "topic" || paused.Rebase.Onto != r.Head || len(paused.Conflicts) != 1 {
						t.Fatalf("unexpected conflict: %+v", paused.Rebase)
					}
					if recovery == domain.ContinueRebase {
						f.write(path, "resolved\n")
						f.git("add", "--", path)
						paused = f.read()
					}
					if err := f.g.ExecuteRepositoryOperation(ctx, paused, domain.RepositoryOperation{Kind: recovery}); err != nil {
						t.Fatal(err)
					}
					if recovery == domain.AbortRebase && (f.git("branch", "--show-current") != "topic" || f.git("rev-parse", "HEAD") != topic) {
						t.Fatal("abort did not restore topic")
					}
				}
			}
			if f.git("branch", "--show-current") != "topic" || f.git("rev-parse", "HEAD^") != r.Head || f.git("rev-parse", "refs/heads/"+base) != r.Head || f.git("status", "--porcelain") != "" {
				t.Fatal("wrong rebase direction or dirty index/worktree")
			}
		})
	}
}

func TestRemoteCheckoutRebaseCreatesAndReusesTrackingBranch(t *testing.T) {
	for _, reuse := range []bool{false, true} {
		name := "create"
		if reuse {
			name = "reuse"
		}
		t.Run(name, func(t *testing.T) {
			f := newRemoteCheckoutRebaseFixture(t, false)
			remotePath := f.git("remote", "get-url", "origin")
			other := filepath.Join(filepath.Dir(remotePath), "other")
			backgroundGit(t, other, "commit", "--allow-empty", "-m", "unfetched remote commit")
			backgroundGit(t, other, "push", "origin", "topic")
			publishedTip := backgroundGit(t, remotePath, "rev-parse", "refs/heads/topic")
			r := f.read()
			remoteRef := "refs/remotes/origin/topic"
			remoteTip := r.Refs[remoteRef]
			if remoteTip == publishedTip {
				t.Fatal("fixture did not leave an unfetched remote commit")
			}
			if reuse {
				f.git("branch", "--track", "topic", remoteRef)
				r = f.read()
			}
			op := domain.RepositoryOperation{Kind: domain.CheckoutRebase, Source: remoteRef, Branch: "topic", Remote: "origin", Target: "topic", ConfirmRewrite: true}
			if err := f.g.ExecuteRepositoryOperation(context.Background(), r, op); err != nil {
				t.Fatal(err)
			}
			if f.git("branch", "--show-current") != "topic" || f.git("rev-parse", "HEAD^") != r.Head || f.git("rev-parse", "HEAD") == remoteTip || f.git("rev-parse", "HEAD") == publishedTip {
				t.Fatal("remote commits were not replayed onto the fixed base")
			}
			if f.git("rev-parse", "refs/heads/"+r.CurrentBranch) != r.Head || f.git("rev-parse", remoteRef) != remoteTip || backgroundGit(t, remotePath, "rev-parse", "refs/heads/topic") != publishedTip || f.git("rev-parse", "--abbrev-ref", "@{upstream}") != "origin/topic" {
				t.Fatal("base, tracking ref, published ref or upstream changed unexpectedly")
			}
			if f.git("status", "--porcelain=v1") != "" || f.git("write-tree") != f.git("rev-parse", "HEAD^{tree}") {
				t.Fatal("successful rebase left a dirty index or worktree")
			}
		})
	}
}

func TestRemoteCheckoutRebaseRejectsUnsafeOrStaleRequests(t *testing.T) {
	t.Run("different upstream", func(t *testing.T) {
		f := newRemoteCheckoutRebaseFixture(t, false)
		f.git("branch", "--track", "topic", "refs/remotes/origin/topic")
		f.git("config", "branch.topic.merge", "refs/heads/other")
		r := f.read()
		assertRemoteCheckoutRebaseRejected(t, f, r, domain.RepositoryOperation{Kind: domain.CheckoutRebase, Source: "refs/remotes/origin/topic", Branch: "topic", Remote: "origin", Target: "topic", ConfirmRewrite: true})
	})
	t.Run("dirty worktree", func(t *testing.T) {
		f := newRemoteCheckoutRebaseFixture(t, false)
		f.write("文件 name.txt", "precious dirty data\n")
		assertRemoteCheckoutRebaseRejected(t, f, f.read(), remoteCheckoutRebaseOperation())
		data, err := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
		if err != nil || string(data) != "precious dirty data\n" {
			t.Fatalf("dirty data changed: %q %v", data, err)
		}
	})
	t.Run("protected branch", func(t *testing.T) {
		f := newRemoteCheckoutRebaseFixture(t, false)
		op := remoteCheckoutRebaseOperation()
		op.Safety.ProtectedBranches = "topic"
		assertRemoteCheckoutRebaseRejected(t, f, f.read(), op)
	})
	t.Run("stale remote", func(t *testing.T) {
		f := newRemoteCheckoutRebaseFixture(t, false)
		r := f.read()
		f.git("update-ref", "refs/remotes/origin/topic", r.Head)
		if err := f.g.ExecuteRepositoryOperation(context.Background(), r, remoteCheckoutRebaseOperation()); err == nil {
			t.Fatal("stale remote checkout and rebase accepted")
		}
		assertRemoteCheckoutRebaseRejectedState(t, f, f.read())
	})
	t.Run("canceled", func(t *testing.T) {
		f := newRemoteCheckoutRebaseFixture(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r := f.read()
		if err := f.g.ExecuteRepositoryOperation(ctx, r, remoteCheckoutRebaseOperation()); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation = %v", err)
		}
		assertRemoteCheckoutRebaseRejectedState(t, f, r)
	})
}

func TestRemoteCheckoutRebaseRevalidatesUpstreamBeforeReplay(t *testing.T) {
	f := newRemoteCheckoutRebaseFixture(t, false)
	f.git("branch", "--track", "topic", "refs/remotes/origin/topic")
	r := f.read()
	validated := false
	ctx := application.WithTaskProgress(context.Background(), func(phase string) {
		if phase == "Validating Checkout and Rebase target" {
			validated = true
			f.git("config", "branch.topic.merge", "refs/heads/other")
		}
	})
	err := f.g.ExecuteRepositoryOperation(ctx, r, remoteCheckoutRebaseOperation())
	if !validated {
		t.Fatal("missing final target-validation checkpoint")
	}
	if err == nil {
		t.Fatal("upstream changed during validation was rebased")
	}
	assertRemoteCheckoutRebaseRejectedState(t, f, r)
}

func TestRemoteCheckoutRebaseConflictContinueAndAbort(t *testing.T) {
	for _, recovery := range []domain.RepositoryOperationKind{domain.ContinueRebase, domain.AbortRebase} {
		name := map[domain.RepositoryOperationKind]string{domain.ContinueRebase: "continue", domain.AbortRebase: "abort"}[recovery]
		t.Run(name, func(t *testing.T) {
			f := newRemoteCheckoutRebaseFixture(t, true)
			r := f.read()
			remoteTip := r.Refs["refs/remotes/origin/topic"]
			if err := f.g.ExecuteRepositoryOperation(context.Background(), r, remoteCheckoutRebaseOperation()); err == nil {
				t.Fatal("expected native conflict")
			}
			paused := f.read()
			if paused.Rebase.Original != remoteTip || paused.Rebase.Branch != "topic" || paused.Rebase.Onto != r.Head || len(paused.Conflicts) != 1 {
				t.Fatalf("unexpected native recovery state: %+v", paused.Rebase)
			}
			if recovery == domain.ContinueRebase {
				f.write("中文 conflict.txt", "resolved\n")
				f.git("add", "--", "中文 conflict.txt")
				paused = f.read()
			}
			if err := f.g.ExecuteRepositoryOperation(context.Background(), paused, domain.RepositoryOperation{Kind: recovery}); err != nil {
				t.Fatal(err)
			}
			if f.git("branch", "--show-current") != "topic" || f.git("rev-parse", "refs/remotes/origin/topic") != remoteTip || f.git("rev-parse", "refs/heads/"+r.CurrentBranch) != r.Head {
				t.Fatal("recovery changed the selected branch, base or tracking ref")
			}
			if recovery == domain.AbortRebase {
				if f.git("rev-parse", "HEAD") != remoteTip {
					t.Fatal("abort did not restore the new tracking branch")
				}
			} else if f.git("rev-parse", "HEAD^") != r.Head || f.git("show", ":中文 conflict.txt") != "resolved" {
				t.Fatal("continue did not finish the fixed-base replay")
			}
			if f.git("status", "--porcelain=v1") != "" || f.git("write-tree") != f.git("rev-parse", "HEAD^{tree}") {
				t.Fatal("recovery left a dirty index or worktree")
			}
		})
	}
}

func remoteCheckoutRebaseOperation() domain.RepositoryOperation {
	return domain.RepositoryOperation{Kind: domain.CheckoutRebase, Source: "refs/remotes/origin/topic", Branch: "topic", Remote: "origin", Target: "topic", ConfirmRewrite: true}
}

func newRemoteCheckoutRebaseFixture(t *testing.T, conflict bool) *workflowFixture {
	t.Helper()
	f := newWorkflow(t)
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	other := filepath.Join(base, "other")
	backgroundGit(t, base, "init", "--bare", "--initial-branch=main", remote)
	f.git("remote", "add", "origin", remote)
	f.git("push", "-u", "origin", "main")
	f.git("checkout", "-b", "topic")
	path := "topic.txt"
	if conflict {
		path = "中文 conflict.txt"
		f.write(path, "topic\n")
	} else {
		f.write(path, "topic\n")
	}
	f.git("add", "--", path)
	f.git("commit", "-m", "topic")
	f.git("push", "-u", "origin", "topic")
	f.git("checkout", "main")
	f.git("branch", "-D", "topic")
	backgroundGit(t, base, "clone", "--branch", "topic", remote, other)
	if conflict {
		f.write("中文 conflict.txt", "base\n")
		f.git("add", "--", "中文 conflict.txt")
	}
	f.git("commit", "--allow-empty", "-m", "onto")
	return f
}

func assertRemoteCheckoutRebaseRejected(t *testing.T, f *workflowFixture, r domain.Repository, op domain.RepositoryOperation) {
	t.Helper()
	if err := f.g.ExecuteRepositoryOperation(context.Background(), r, op); err == nil {
		t.Fatal("unsafe remote checkout and rebase accepted")
	}
	assertRemoteCheckoutRebaseRejectedState(t, f, r)
}

func assertRemoteCheckoutRebaseRejectedState(t *testing.T, f *workflowFixture, r domain.Repository) {
	t.Helper()
	after := f.read()
	if after.CurrentBranch != r.CurrentBranch || after.Head != r.Head || f.git("rev-parse", "refs/heads/"+r.CurrentBranch) != r.Head {
		t.Fatal("rejected operation changed HEAD")
	}
	if after.IndexVersion != r.IndexVersion || after.WorktreeVersion != r.WorktreeVersion || after.Refs["refs/remotes/origin/topic"] != r.Refs["refs/remotes/origin/topic"] {
		t.Fatal("rejected operation changed the index, worktree or tracking ref")
	}
	if _, existed := r.Refs["refs/heads/topic"]; !existed && after.Refs["refs/heads/topic"] != "" {
		t.Fatal("rejected operation created a local branch")
	}
}
