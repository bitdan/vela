package gitcli

import (
	"context"
	"errors"
	"testing"

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
