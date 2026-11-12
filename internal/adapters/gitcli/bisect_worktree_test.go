package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"vela/internal/domain"
)

func candidateWorkflow(t *testing.T) (*workflowFixture, domain.Repository) {
	t.Helper()
	f := newWorkflow(t)
	good := f.git("rev-parse", "HEAD")
	for i := 0; i < 5; i++ {
		f.git("commit", "--allow-empty", "-m", "step")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.StartBisect, Source: f.read().Head, Target: good}); err != nil {
		t.Fatal(err)
	}
	return f, f.read()
}

func TestBisectCandidateWorktreeOutsideLoadedHistory(t *testing.T) {
	f, snapshot := candidateWorkflow(t)
	if snapshot.Bisect.Candidate == snapshot.Head {
		t.Fatal("fixture candidate is not independent of HEAD")
	}
	// A candidate can be outside a truncated/filtered history page.
	snapshot.History = nil
	path := filepath.Join(t.TempDir(), "candidate 空格")
	op := domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "Detached", Source: snapshot.Bisect.Candidate, Path: path, BisectVersion: snapshot.Bisect.Version}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); err != nil {
		t.Fatal(err)
	}
	linked, err := f.g.ReadRepository(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if linked.Head != op.Source || linked.CurrentBranch != "Detached HEAD" || linked.Bisect.Active() {
		t.Fatalf("wrong candidate checkout: %+v", linked.Bisect)
	}
	after := f.read()
	if after.Head != snapshot.Head || after.IndexVersion != snapshot.IndexVersion || after.WorktreeVersion != snapshot.WorktreeVersion || after.Bisect.Version != snapshot.Bisect.Version {
		t.Fatal("candidate creation changed original checkout/search")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); err != nil {
		t.Fatal(err)
	}
}

func TestBisectCandidateWorktreeRejectsChangedSearch(t *testing.T) {
	f, snapshot := candidateWorkflow(t)
	path := filepath.Join(t.TempDir(), "stale")
	op := domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "Detached", Source: snapshot.Bisect.Candidate, Path: path, BisectVersion: snapshot.Bisect.Version}
	if err := f.run(domain.RepositoryOperation{Kind: domain.BisectGood, Source: snapshot.Bisect.Candidate}); err != nil {
		t.Fatal(err)
	}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("changed candidate = %v", err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetBisect}); err != nil {
		t.Fatal(err)
	}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("ended search = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale creation wrote destination", err)
	}
	if len(f.read().Worktrees) != 1 {
		t.Fatal("stale candidate registered a worktree")
	}
}

func TestBisectCandidateWorktreeValidatesRequest(t *testing.T) {
	f, snapshot := candidateWorkflow(t)
	path := filepath.Join(t.TempDir(), "invalid")
	op := domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "Detached", Source: snapshot.Head, Path: path, BisectVersion: snapshot.Bisect.Version}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); err == nil {
		t.Fatal("non-candidate source accepted")
	}
	op.Source = snapshot.Bisect.Candidate
	op.Mode = "New Branch"
	op.Branch = "must-not-exist"
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); err == nil {
		t.Fatal("candidate request created a branch")
	}
	op.Mode = "Detached"
	op.BisectVersion = "wrong-search"
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("wrong search version = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	op.BisectVersion = snapshot.Bisect.Version
	if err := f.g.ExecuteRepositoryOperation(ctx, snapshot, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	if f.read().Refs["refs/heads/must-not-exist"] != "" {
		t.Fatal("invalid candidate created a branch")
	}
}

func TestBisectCandidateWorktreeRejectsIdenticalRestart(t *testing.T) {
	f, snapshot := candidateWorkflow(t)
	op := domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "Detached", Source: snapshot.Bisect.Candidate, Path: filepath.Join(t.TempDir(), "old-dialog"), BisectVersion: snapshot.Bisect.Version}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetBisect}); err != nil {
		t.Fatal(err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.StartBisect, Source: snapshot.Bisect.Bad, Target: snapshot.Bisect.Good[0]}); err != nil {
		t.Fatal(err)
	}
	if f.read().Bisect.Candidate != snapshot.Bisect.Candidate {
		t.Fatal("fixture did not restart the identical candidate")
	}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("old dialog after identical restart = %v", err)
	}
}
