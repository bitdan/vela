package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestPhase3BranchAndRemoteOperations(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	local := filepath.Join(root, "local")
	remote := filepath.ToSlash(filepath.Join(root, "remote.git"))
	other := filepath.Join(root, "other")
	if err := os.Mkdir(local, 0755); err != nil {
		t.Fatal(err)
	}
	g := New()
	ctx := context.Background()
	if _, err := g.Initialize(ctx, local, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Vela Test", "-c", "user.email=vela@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git(local, "init", "--bare", "--initial-branch=main", remote)
	git(local, "remote", "add", "origin", remote)
	git(local, "commit", "--allow-empty", "-m", "initial")
	read := func() domain.Repository {
		t.Helper()
		repo, err := g.ReadRepository(ctx, local)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	run := func(req domain.RepositoryOperation) {
		t.Helper()
		if err := g.ExecuteRepositoryOperation(ctx, read(), req); err != nil {
			t.Fatalf("operation %+v: %v", req, err)
		}
	}
	run(domain.RepositoryOperation{Kind: domain.PushBranch, Branch: "main", Remote: "origin", Target: "main", SetUpstream: true})
	run(domain.RepositoryOperation{Kind: domain.CreateBranch, Branch: "feature/测试", Source: "refs/heads/main"})
	run(domain.RepositoryOperation{Kind: domain.CheckoutBranch, Source: "refs/heads/feature/测试"})
	if got := git(local, "symbolic-ref", "--short", "HEAD"); got != "feature/测试" {
		t.Fatalf("checkout got %q", got)
	}
	run(domain.RepositoryOperation{Kind: domain.CheckoutBranch, Source: "refs/heads/main"})
	git(root, "clone", remote, other)
	git(other, "commit", "--allow-empty", "-m", "remote advance")
	git(other, "push", "origin", "main")
	remoteHead := git(other, "rev-parse", "HEAD")
	oldHead := git(local, "rev-parse", "HEAD")
	run(domain.RepositoryOperation{Kind: domain.FetchRemote, Remote: "origin"})
	if git(local, "rev-parse", "HEAD") != oldHead || git(local, "rev-parse", "refs/remotes/origin/main") != remoteHead {
		t.Fatal("Fetch changed HEAD or did not update the remote-tracking branch")
	}
	run(domain.RepositoryOperation{Kind: domain.CheckoutBranch, Source: "refs/heads/feature/测试"})
	run(domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "main", Remote: "origin", Target: "main"})
	if git(local, "rev-parse", "HEAD") != oldHead || git(local, "rev-parse", "main") != remoteHead {
		t.Fatal("updating a non-current branch changed HEAD or did not fast-forward")
	}
	run(domain.RepositoryOperation{Kind: domain.PullBranch, Branch: "feature/测试", Remote: "origin", Target: "main"})
	if git(local, "rev-parse", "HEAD") != remoteHead || git(local, "symbolic-ref", "--short", "HEAD") != "feature/测试" {
		t.Fatal("Pull did not fast-forward the checked-out branch")
	}
	run(domain.RepositoryOperation{Kind: domain.CheckoutBranch, Source: "refs/remotes/origin/main", Branch: "review"})
	if git(local, "rev-parse", "--abbrev-ref", "@{upstream}") != "origin/main" {
		t.Fatal("remote checkout did not configure upstream")
	}
	git(local, "commit", "--allow-empty", "-m", "local divergence")
	git(other, "commit", "--allow-empty", "-m", "remote divergence")
	git(other, "push", "origin", "main")
	localHead := git(local, "rev-parse", "HEAD")
	if err := g.ExecuteRepositoryOperation(ctx, read(), domain.RepositoryOperation{
		Kind: domain.PullBranch, Branch: "review", Remote: "origin", Target: "main",
	}); err == nil || !strings.Contains(err.Error(), "Fetched origin/main.") || git(local, "rev-parse", "HEAD") != localHead {
		t.Fatalf("divergent Pull failed to preserve HEAD and report its fetch: %v", err)
	}
	if err := g.ExecuteRepositoryOperation(ctx, read(), domain.RepositoryOperation{
		Kind: domain.PushBranch, Branch: "review", Remote: "origin", Target: "main",
	}); err == nil || git(other, "rev-parse", "HEAD") != git(local, "rev-parse", "refs/remotes/origin/main") {
		t.Fatalf("divergent Push was not rejected: %v", err)
	}
	linked := filepath.Join(root, "linked")
	git(local, "worktree", "add", linked, "main")
	if err := g.ExecuteRepositoryOperation(ctx, read(), domain.RepositoryOperation{
		Kind: domain.UpdateTrackedBranch, Branch: "main", Remote: "origin", Target: "main",
	}); err == nil || !strings.Contains(err.Error(), "checked out in a worktree") {
		t.Fatalf("Update should protect linked worktrees: %v", err)
	}
	snapshot := read()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := g.ExecuteRepositoryOperation(canceled, snapshot, domain.RepositoryOperation{
		Kind: domain.FetchRemote, Remote: "origin",
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Fetch returned %v", err)
	}
	unborn := filepath.Join(root, "unborn")
	if err := os.Mkdir(unborn, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Initialize(ctx, unborn, "start"); err != nil {
		t.Fatal(err)
	}
	git(unborn, "remote", "add", "origin", remote)
	initial, err := g.ReadRepository(ctx, unborn)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.ExecuteRepositoryOperation(ctx, initial, domain.RepositoryOperation{
		Kind: domain.PullBranch, Branch: "start", Remote: "origin", Target: "main",
	}); err != nil || git(unborn, "rev-parse", "HEAD") != git(other, "rev-parse", "HEAD") {
		t.Fatalf("Pull did not populate the unborn branch: %v", err)
	}
}
