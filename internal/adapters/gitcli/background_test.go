package gitcli

import (
	"context"
	"errors"
	"net"
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
	cmd.Env = repositoryEnvironment()
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
	if err := os.WriteFile(filepath.Join(local, "tracked 中文.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	backgroundGit(t, local, "add", "--", "tracked 中文.txt")
	backgroundGit(t, local, "commit", "--allow-empty", "-m", "base")
	backgroundGit(t, local, "push", "-u", "origin", "topic")
	backgroundGit(t, base, "clone", remote, other)
	backgroundGit(t, other, "commit", "--allow-empty", "-m", "incoming")
	backgroundGit(t, other, "push", "origin", "topic")
	if err := os.WriteFile(filepath.Join(local, "中文 draft.txt"), []byte("keep\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "tracked 中文.txt"), []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	backgroundGit(t, local, "add", "--", "tracked 中文.txt")
	if err := os.WriteFile(filepath.Join(local, "tracked 中文.txt"), []byte("working\n"), 0600); err != nil {
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
	t.Parallel()
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
	t.Parallel()
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

func TestBackgroundFixtureIgnoresRepositoryEnvironment(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	other := filepath.Join(base, "other")
	backgroundGit(t, base, "init", "--initial-branch=topic", target)
	backgroundGit(t, base, "init", "--initial-branch=topic", other)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, "custom-index"))
	got := backgroundGit(t, target, "rev-parse", "--absolute-git-dir")
	gotInfo, gotErr := os.Stat(got)
	wantInfo, wantErr := os.Stat(filepath.Join(target, ".git"))
	if gotErr != nil || wantErr != nil || !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("fixture escaped target: %s", got)
	}
}

func TestBackgroundRemoteInFlightCancellationAndTimeout(t *testing.T) {
	t.Parallel()
	for _, autoFetch := range []bool{false, true} {
		for _, timeout := range []bool{false, true} {
			name := "incoming"
			if autoFetch {
				name = "fetch"
			}
			if timeout {
				name += "-timeout"
			} else {
				name += "-cancel"
			}
			t.Run(name, func(t *testing.T) {
				g, repo, _ := backgroundFixture(t)
				started, release := make(chan struct{}, 1), make(chan struct{})
				// Native git:// transport keeps the blocked socket in Git itself.
				// HTTP uses a separate remote-helper process on Windows.
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				defer listener.Close()
				serverDone := make(chan struct{})
				go func() {
					defer close(serverDone)
					conn, err := listener.Accept()
					if err != nil {
						return
					}
					defer conn.Close()
					var packet [4096]byte
					if _, err := conn.Read(packet[:]); err == nil {
						started <- struct{}{}
					}
					<-release
				}()
				defer func() { close(release); listener.Close(); <-serverDone }()
				backgroundGit(t, repo.Root, "remote", "set-url", "origin", "git://"+listener.Addr().String()+"/repo.git")
				repo, err = g.ReadRepository(context.Background(), repo.Root)
				if err != nil {
					t.Fatal(err)
				}
				before, index, tracking := backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z"), backgroundGit(t, repo.Root, "write-tree"), backgroundGit(t, repo.Root, "rev-parse", "origin/topic")
				ctx, cancel := context.WithCancel(context.Background())
				want := context.Canceled
				if timeout {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
					want = context.DeadlineExceeded
				}
				defer cancel()
				done := make(chan error, 1)
				go func() {
					_, err := g.CheckRepositoryRemote(ctx, repo, application.BackgroundRemotePolicy{AutoFetch: autoFetch, CheckIncoming: !autoFetch})
					done <- err
				}()
				select {
				case <-started:
				case <-time.After(30 * time.Second):
					t.Fatal("Git never started remote request")
				}
				if !timeout {
					cancel()
				}
				select {
				case err := <-done:
					if !errors.Is(err, want) {
						t.Fatalf("in-flight result: %v, want %v", err, want)
					}
				case <-time.After(30 * time.Second):
					t.Fatal("Git did not stop")
				}
				if backgroundGit(t, repo.Root, "rev-parse", "HEAD") != repo.Head || backgroundGit(t, repo.Root, "write-tree") != index || backgroundGit(t, repo.Root, "status", "--porcelain=v1", "-z") != before || backgroundGit(t, repo.Root, "rev-parse", "origin/topic") != tracking {
					t.Fatal("interrupted network request changed repository")
				}
			})
		}
	}
}
