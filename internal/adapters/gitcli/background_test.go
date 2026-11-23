package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"vela/internal/application"
	"vela/internal/domain"
)

func backgroundGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=Vela Test", "-c", "user.email=vela@example.test", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func backgroundFixture(t *testing.T) (*Executor, domain.Repository, string) {
	t.Helper()
	base := t.TempDir()
	remote := filepath.Join(base, "remote.git")
	local := filepath.Join(base, "中文 local")
	other := filepath.Join(base, "other")
	backgroundGit(t, base, "init", "--bare", "--initial-branch=topic", remote)
	backgroundGit(t, base, "clone", remote, local)
	backgroundGit(t, local, "commit", "--allow-empty", "-m", "base")
	backgroundGit(t, local, "push", "-u", "origin", "topic")
	backgroundGit(t, base, "clone", remote, other)
	backgroundGit(t, other, "commit", "--allow-empty", "-m", "incoming")
	backgroundGit(t, other, "push", "origin", "topic")
	if err := os.WriteFile(filepath.Join(local, "中文 draft.txt"), []byte("keep\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	g := New()
	repo, err := g.ReadRepository(context.Background(), local)
	if err != nil {
		t.Fatal(err)
	}
	return g, repo, backgroundGit(t, other, "rev-parse", "HEAD")
}

func TestBackgroundRemoteIncomingAndFetch(t *testing.T) {
	g, repo, target := backgroundFixture(t)
	ctx := context.Background()
	before := backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z")
	index := backgroundGit(t, repo.Root, "write-tree")
	tracking := repo.Refs["refs/remotes/origin/topic"]
	result, err := g.CheckRepositoryRemote(ctx, repo, application.BackgroundRemotePolicy{CheckIncoming: true})
	if err != nil || !result.Changed || len(result.Fetched) != 0 {
		t.Fatalf("incoming: %+v %v", result, err)
	}
	if backgroundGit(t, repo.Root, "rev-parse", "origin/topic") != tracking {
		t.Fatal("incoming fetched implicitly")
	}
	result, err = g.CheckRepositoryRemote(ctx, repo, application.BackgroundRemotePolicy{AutoFetch: true})
	if err != nil || !reflect.DeepEqual(result.Fetched, []string{"origin"}) {
		t.Fatalf("fetch: %+v %v", result, err)
	}
	if backgroundGit(t, repo.Root, "rev-parse", "origin/topic") != target || backgroundGit(t, repo.Root, "rev-parse", "HEAD") != repo.Head || backgroundGit(t, repo.Root, "rev-parse", "topic") != repo.Head || backgroundGit(t, repo.Root, "write-tree") != index || backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z") != before {
		t.Fatal("fetch did not preserve local state")
	}
	data, err := os.ReadFile(filepath.Join(repo.Root, "中文 draft.txt"))
	if err != nil || string(data) != "keep\r\n" {
		t.Fatalf("draft changed: %q %v", data, err)
	}
	updated, err := g.ReadRepository(ctx, repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	result, err = g.CheckRepositoryRemote(ctx, updated, application.BackgroundRemotePolicy{CheckIncoming: true})
	if err != nil || result.Changed {
		t.Fatalf("fetched upstream: %+v %v", result, err)
	}
	// A different remote branch must not become the implicit incoming target.
	backgroundGit(t, repo.Root, "config", "branch.topic.merge", "refs/heads/missing")
	result, err = g.CheckRepositoryRemote(ctx, updated, application.BackgroundRemotePolicy{CheckIncoming: true})
	if err != nil || result.Changed {
		t.Fatalf("fixed upstream was replaced: %+v %v", result, err)
	}
}

func TestBackgroundRemoteSkipsAndCancellation(t *testing.T) {
	g, repo, _ := backgroundFixture(t)
	backgroundGit(t, repo.Root, "remote", "set-url", "origin", "ssh://invalid.example/project")
	var err error
	repo, err = g.ReadRepository(context.Background(), repo.Root)
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []application.BackgroundRemotePolicy{{AutoFetch: true}, {CheckIncoming: true}} {
		result, err := g.CheckRepositoryRemote(context.Background(), repo, policy)
		if err != nil || !reflect.DeepEqual(result.Skipped, []string{"origin"}) || len(result.Fetched) != 0 {
			t.Fatalf("skip: %+v %v", result, err)
		}
	}
	for _, deadline := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		want := context.Canceled
		if deadline {
			cancel()
			ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			want = context.DeadlineExceeded
		} else {
			cancel()
		}
		_, err := g.CheckRepositoryRemote(ctx, repo, application.BackgroundRemotePolicy{AutoFetch: true})
		cancel()
		if !errors.Is(err, want) {
			t.Fatalf("cancellation: %v, want %v", err, want)
		}
	}
	if backgroundGit(t, repo.Root, "rev-parse", "HEAD") != repo.Head {
		t.Fatal("canceled check changed HEAD")
	}
}
