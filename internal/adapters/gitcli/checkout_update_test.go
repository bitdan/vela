package gitcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestCheckoutUpdateFastForwardsOnlySelectedBranch(t *testing.T) {
	g, fixture, target := updateFixture(t)
	ctx := context.Background()
	git := func(args ...string) string { return backgroundGit(t, fixture.Root, args...) }
	git("reset", "--hard", "HEAD")
	git("checkout", "-b", "other")
	read := func() domain.Repository {
		r, err := g.ReadRepository(ctx, fixture.Root)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	op := domain.RepositoryOperation{Kind: domain.CheckoutUpdate, Branch: "topic", Remote: "origin", Target: "topic"}
	r := read()
	if err := os.WriteFile(filepath.Join(r.Root, "tracked 中文.txt"), []byte("dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := g.ExecuteRepositoryOperation(ctx, read(), op); err == nil || git("branch", "--show-current") != "other" {
		t.Fatal("dirty branch was switched")
	}
	git("reset", "--hard", "HEAD")
	base := git("rev-parse", "HEAD")
	local := git("commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", "diverged")
	git("update-ref", "refs/heads/topic", local)
	if err := g.ExecuteRepositoryOperation(ctx, read(), op); err == nil || !strings.Contains(err.Error(), "Checked out topic") || git("branch", "--show-current") != "topic" || git("rev-parse", "HEAD") != local {
		t.Fatalf("divergence did not preserve the checkout and tip: %v", err)
	}
	git("checkout", "other")
	git("update-ref", "refs/heads/topic", base)
	hook := filepath.Join(read().GitDir, "hooks", "post-checkout")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := g.ExecuteRepositoryOperation(ctx, read(), op); err == nil || !strings.Contains(err.Error(), "Checked out topic") || git("branch", "--show-current") != "topic" {
		t.Fatalf("checkout hook failure hid the already completed switch: %v", err)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	git("checkout", "other")
	if err := g.ExecuteRepositoryOperation(ctx, read(), op); err != nil {
		t.Fatal(err)
	}
	if git("branch", "--show-current") != "topic" || git("rev-parse", "HEAD") != target || git("rev-parse", "other") != base {
		t.Fatal("checkout did not fast-forward only the selected branch")
	}
}
