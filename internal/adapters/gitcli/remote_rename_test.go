package gitcli

import (
	"context"
	"testing"

	"vela/internal/domain"
)

func TestRemoteRenamePreservesTrackingAndRejectsStaleRefs(t *testing.T) {
	f := newWorkflow(t)
	ctx := context.Background()
	f.git("remote", "add", "origin", "https://example.test/source.git")
	f.git("remote", "add", "taken", "https://example.test/other.git")
	branch := f.git("branch", "--show-current")
	head := f.git("rev-parse", "HEAD")
	f.git("config", "branch."+branch+".remote", "origin")
	f.git("config", "branch."+branch+".merge", "refs/heads/review")
	f.git("update-ref", "refs/remotes/origin/review", head)
	f.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/review")
	r := f.read()
	op := domain.RepositoryOperation{Kind: domain.RenameRemote, Remote: "origin", Target: "team"}
	bad := op
	bad.Target = "taken"
	if err := f.g.ExecuteRepositoryOperation(ctx, r, bad); err == nil {
		t.Fatal("accepted duplicate remote")
	}
	f.git("update-ref", "refs/remotes/origin/new", head)
	if err := f.g.ExecuteRepositoryOperation(ctx, r, op); err == nil {
		t.Fatal("accepted stale tracking refs")
	}
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), op); err != nil {
		t.Fatal(err)
	}
	if f.git("config", "--get", "branch."+branch+".remote") != "team" ||
		f.git("rev-parse", "refs/remotes/team/review") != head || f.git("symbolic-ref", "refs/remotes/team/HEAD") != "refs/remotes/team/review" ||
		f.git("rev-parse", "HEAD") != head || f.git("for-each-ref", "refs/remotes/origin/") != "" {
		t.Fatal("rename did not migrate tracking while preserving HEAD")
	}
}
