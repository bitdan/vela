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

func updateFixture(t *testing.T) (*Executor, domain.Repository, string) {
	t.Helper()
	g, repo, target := backgroundFixture(t)
	backgroundGit(t, repo.Root, "config", "core.autocrlf", "false")
	var err error
	repo, err = g.ReadRepository(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	return g, repo, target
}

func TestUpdateRetainsShelfUntilExplicitRestore(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"Fast-forward", "Merge", "Rebase"} {
		t.Run(method, func(t *testing.T) {
			g, repo, target := updateFixture(t)
			git := func(args ...string) string { return backgroundGit(t, repo.Root, args...) }
			if method != "Fast-forward" {
				oid := git("commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", "local divergence")
				git("update-ref", "refs/heads/topic", oid, repo.Head)
			}
			var err error
			repo, err = g.ReadRepository(context.Background(), repo.Root)
			if err != nil {
				t.Fatal(err)
			}
			index, stash := git("show", ":tracked 中文.txt"), git("stash", "list")
			result, err := g.UpdateRepository(context.Background(), repo, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "topic", Remote: "origin", Target: "topic", UpdateMethod: method, UpdateCleanup: "Shelve", ConfirmUpdateCleanup: true})
			if err != nil || !result.Fetched || !result.Cleaned || !result.Integrated || result.Restored || result.BackupKind != "Shelve" || result.BackupOID == "" || result.BackupID == "" {
				t.Fatalf("Update must retain an unapplied Shelf: %+v %v", result, err)
			}
			updated, err := g.ReadRepository(context.Background(), repo.Root)
			if err != nil {
				t.Fatal(err)
			}
			if git("write-tree") != git("rev-parse", "HEAD^{tree}") || git("show", ":tracked 中文.txt") != "base" || git("stash", "list") != stash || len(updated.Shelf) != 1 || updated.Shelf[0].OID != result.BackupOID {
				t.Fatal("Update did not leave clean tracked state and retained Shelf")
			}
			data, _ := os.ReadFile(filepath.Join(repo.Root, "tracked 中文.txt"))
			if string(data) != "base\n" {
				t.Fatalf("backup auto-restored: %q", data)
			}
			data, _ = os.ReadFile(filepath.Join(repo.Root, "中文 draft.txt"))
			if string(data) != "keep\r\n" {
				t.Fatal("untracked draft changed")
			}
			if method == "Fast-forward" && updated.Head != target {
				t.Fatal("did not fast-forward")
			}
			parents := strings.Fields(git("rev-list", "--parents", "-n", "1", "HEAD"))
			if method == "Merge" && (len(parents) != 3 || parents[1] != repo.Head || parents[2] != target) {
				t.Fatalf("merge parents: %v", parents)
			}
			if method == "Rebase" && (len(parents) != 2 || parents[1] != target || updated.Head == repo.Head) {
				t.Fatalf("rebase parents: %v", parents)
			}
			if err := g.ExecuteRepositoryOperation(context.Background(), updated, domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: result.BackupOID, Target: result.BackupID, RestoreIndex: true}); err != nil {
				t.Fatal(err)
			}
			data, _ = os.ReadFile(filepath.Join(repo.Root, "tracked 中文.txt"))
			if string(data) != "working\n" || git("show", ":tracked 中文.txt") != index || git("rev-parse", "HEAD") != updated.Head || git("stash", "list") != stash {
				t.Fatal("explicit Unshelve lost staged/worktree distinctions or moved HEAD/Stash")
			}
			kept, err := g.ReadRepository(context.Background(), repo.Root)
			if err != nil || len(kept.Shelf) != 1 || kept.Shelf[0].OID != result.BackupOID {
				t.Fatal("Unshelve removed retained backup")
			}
		})
	}
}

func TestUpdateRejectsStashCleanup(t *testing.T) {
	t.Parallel()
	g, repo, _ := updateFixture(t)
	index := backgroundGit(t, repo.Root, "write-tree")
	status := backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z")
	result, err := g.UpdateRepository(context.Background(), repo, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "topic", Remote: "origin", Target: "topic", UpdateMethod: "Fast-forward", UpdateCleanup: "Stash", ConfirmUpdateCleanup: true})
	if err == nil || result.Fetched || result.BackupOID != "" {
		t.Fatalf("legacy Stash cleanup accepted: %+v %v", result, err)
	}
	if backgroundGit(t, repo.Root, "rev-parse", "HEAD") != repo.Head || backgroundGit(t, repo.Root, "write-tree") != index || backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z") != status {
		t.Fatal("rejected Update changed repository")
	}
}

func TestUpdateCanceledAfterShelfPublicationRetainsBackup(t *testing.T) {
	t.Parallel()
	g, repo, _ := updateFixture(t)
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := application.WithTaskProgress(parent, func(phase string) {
		if phase == "Tracked changes cleaned; backup retained" {
			cancel()
		}
	})
	result, err := g.UpdateRepository(ctx, repo, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "topic", Remote: "origin", Target: "topic", UpdateMethod: "Merge", UpdateCleanup: "Shelve", ConfirmUpdateCleanup: true})
	if !errors.Is(err, context.Canceled) || !result.Cleaned || result.Integrated || result.Restored || result.BackupOID == "" {
		t.Fatalf("cancel checkpoints: %+v %v", result, err)
	}
	updated, err := g.ReadRepository(context.Background(), repo.Root)
	if err != nil || updated.Head != repo.Head || len(updated.Shelf) != 1 || updated.Shelf[0].OID != result.BackupOID {
		t.Fatalf("cancel lost backup/head: %+v %v", updated.Shelf, err)
	}
	if err := g.ExecuteRepositoryOperation(context.Background(), updated, domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: result.BackupOID, Target: result.BackupID, RestoreIndex: true}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(repo.Root, "tracked 中文.txt"))
	if string(data) != "working\n" || backgroundGit(t, repo.Root, "show", ":tracked 中文.txt") != "staged" {
		t.Fatal("cancel backup is not restorable")
	}
}

func TestUpdateConflictKeepsRestorableShelf(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"Merge", "Rebase"} {
		t.Run(method, func(t *testing.T) {
			g, repo, _ := updateFixture(t)
			git := func(args ...string) string { return backgroundGit(t, repo.Root, args...) }
			git("stash", "push", "-m", "existing external stash")
			name := "tracked 中文.txt"
			write := func(dir, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(repo.Root, "local committed\n")
			git("add", "--", name)
			git("commit", "-m", "local conflict")
			other := filepath.Join(filepath.Dir(repo.Root), "other")
			write(other, "remote committed\n")
			backgroundGit(t, other, "add", "--", name)
			backgroundGit(t, other, "commit", "-m", "remote conflict")
			backgroundGit(t, other, "push", "origin", "topic")
			write(repo.Root, "staged pending\n")
			git("add", "--", name)
			write(repo.Root, "working pending\n")
			var err error
			repo, err = g.ReadRepository(context.Background(), repo.Root)
			if err != nil {
				t.Fatal(err)
			}
			stash := git("stash", "list")
			result, err := g.UpdateRepository(context.Background(), repo, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "topic", Remote: "origin", Target: "topic", UpdateMethod: method, UpdateCleanup: "Shelve", ConfirmUpdateCleanup: true})
			if err == nil || !result.Fetched || !result.Cleaned || result.Integrated || result.Restored || result.BackupOID == "" {
				t.Fatalf("conflict checkpoints: %+v %v", result, err)
			}
			paused, err := g.ReadRepository(context.Background(), repo.Root)
			if err != nil || len(paused.Conflicts) == 0 || len(paused.Shelf) != 1 || paused.Shelf[0].OID != result.BackupOID {
				t.Fatalf("conflict/backup missing: %+v %v", paused.Shelf, err)
			}
			if method == "Merge" {
				if len(paused.MergeHeads) == 0 || paused.Head != repo.Head {
					t.Fatal("merge did not pause at original HEAD")
				}
				git("merge", "--abort")
			} else {
				if !paused.Rebase.Active() {
					t.Fatal("rebase did not pause")
				}
				git("rebase", "--abort")
			}
			active, err := g.ReadRepository(context.Background(), repo.Root)
			if err != nil || active.Head != repo.Head {
				t.Fatalf("abort did not restore HEAD: %v", err)
			}
			if err := g.ExecuteRepositoryOperation(context.Background(), active, domain.RepositoryOperation{Kind: domain.ApplyShelf, Source: result.BackupOID, Target: result.BackupID, RestoreIndex: true}); err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(filepath.Join(repo.Root, name))
			if string(data) != "working pending\n" || git("show", ":"+name) != "staged pending" || git("stash", "list") != stash {
				t.Fatal("conflict backup lost data or changed existing Stash")
			}
		})
	}
}

func TestUpdateOtherBranchFastForwardOnly(t *testing.T) {
	t.Parallel()
	g, repo, target := updateFixture(t)
	git := func(args ...string) string { return backgroundGit(t, repo.Root, args...) }
	git("branch", "--track", "tracking", "origin/topic")
	var err error
	repo, err = g.ReadRepository(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	index, status := git("write-tree"), git("status", "--porcelain=v1", "-z")
	for _, method := range []string{"Merge", "Rebase"} {
		result, err := g.UpdateRepository(context.Background(), repo, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "tracking", Remote: "origin", Target: "topic", UpdateMethod: method})
		if err == nil || result.Fetched || git("rev-parse", "tracking") != repo.Head {
			t.Fatalf("non-current %s accepted: %+v %v", method, result, err)
		}
	}
	op := domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "tracking", Remote: "origin", Target: "topic", UpdateMethod: "Fast-forward"}
	result, err := g.UpdateRepository(context.Background(), repo, op)
	if err != nil || !result.Integrated || result.Cleaned || result.BackupOID != "" || git("rev-parse", "tracking") != target || git("rev-parse", "HEAD") != repo.Head || git("write-tree") != index || git("status", "--porcelain=v1", "-z") != status {
		t.Fatalf("non-current FF: %+v %v", result, err)
	}
	// A divergent non-current ref must be rejected rather than rewritten.
	oid := git("commit-tree", repo.Head+"^{tree}", "-p", repo.Head, "-m", "divergence")
	git("update-ref", "refs/heads/tracking", oid, target)
	repo, err = g.ReadRepository(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.UpdateRepository(context.Background(), repo, op); err == nil || git("rev-parse", "tracking") != oid {
		t.Fatal("divergent non-current branch was rewritten")
	}
}

func TestUpdatePreCanceledPreservesState(t *testing.T) {
	t.Parallel()
	g, repo, _ := updateFixture(t)
	index := backgroundGit(t, repo.Root, "write-tree")
	status := backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := g.UpdateRepository(ctx, repo, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "topic", Remote: "origin", Target: "topic", UpdateMethod: "Fast-forward", UpdateCleanup: "Shelve", ConfirmUpdateCleanup: true})
	if !errors.Is(err, context.Canceled) || result.Fetched || result.BackupOID != "" || backgroundGit(t, repo.Root, "rev-parse", "HEAD") != repo.Head || backgroundGit(t, repo.Root, "write-tree") != index || backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z") != status {
		t.Fatalf("pre-cancel changed state: %+v %v", result, err)
	}
}

func TestUpdateFailedShelfPublicationDoesNotClaimBackup(t *testing.T) {
	t.Parallel()
	g, repo, _ := updateFixture(t)
	git := func(args ...string) string { return backgroundGit(t, repo.Root, args...) }
	index, status, stash := git("write-tree"), git("status", "--porcelain=v1", "-z"), git("stash", "list")
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := application.WithTaskProgress(parent, func(phase string) {
		if phase == "Saving retained backup and cleaning tracked changes" {
			cancel()
		}
	})
	result, err := g.UpdateRepository(ctx, repo, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch, Branch: "topic", Remote: "origin", Target: "topic", UpdateMethod: "Merge", UpdateCleanup: "Shelve", ConfirmUpdateCleanup: true})
	if !errors.Is(err, context.Canceled) || !result.Fetched || result.Cleaned || result.Integrated || result.BackupName == "" || result.BackupOID != "" || result.BackupID != "" {
		t.Fatalf("prepublication failure: %+v %v", result, err)
	}
	if git("rev-parse", "HEAD") != repo.Head || git("write-tree") != index || git("status", "--porcelain=v1", "-z") != status || git("stash", "list") != stash {
		t.Fatal("failed publication changed original state")
	}
	updated, err := g.ReadRepository(context.Background(), repo.Root)
	if err != nil || len(updated.Shelf) != 0 {
		t.Fatalf("failed publication fabricated Shelf: %+v %v", updated.Shelf, err)
	}
}
