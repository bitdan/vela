//go:build windows

package gitcli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"vela/internal/application"
)

func TestHTTPRemoteCancellationReapsHelpers(t *testing.T) {
	t.Parallel()
	for _, fetch := range []bool{false, true} {
		for _, deadline := range []bool{false, true} {
			name := "incoming"
			if fetch {
				name = "fetch"
			}
			if deadline {
				name += "-timeout"
			} else {
				name += "-cancel"
			}
			t.Run(name, func(t *testing.T) {
				g, repo, _ := backgroundFixture(t)
				started, disconnected, release := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{})
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case started <- struct{}{}:
					default:
					}
					select {
					case <-r.Context().Done():
						disconnected <- struct{}{}
					case <-release:
					}
				}))
				defer server.Close()
				defer func() {
					close(release)
					server.Close()
					limit := time.Now().Add(5 * time.Second)
					for time.Now().Before(limit) {
						if err := os.Rename(repo.Root, repo.Root+"-cleanup"); err == nil {
							if err := os.Rename(repo.Root+"-cleanup", repo.Root); err != nil {
								t.Error(err)
							}
							return
						}
						time.Sleep(20 * time.Millisecond)
					}
				}()
				backgroundGit(t, repo.Root, "remote", "set-url", "origin", server.URL+"/repo.git")
				var err error
				repo, err = g.ReadRepository(context.Background(), repo.Root)
				if err != nil {
					t.Fatal(err)
				}
				git := func(args ...string) string { return backgroundGit(t, repo.Root, args...) }
				index, status, tracking := git("write-tree"), git("status", "--porcelain=v1", "-z"), git("rev-parse", "origin/topic")
				ctx, cancel := context.WithCancel(context.Background())
				want := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
					want = context.DeadlineExceeded
				}
				defer cancel()
				done := make(chan error, 1)
				go func() {
					_, err := g.CheckRepositoryRemote(ctx, repo, application.BackgroundRemotePolicy{AutoFetch: fetch, CheckIncoming: !fetch})
					done <- err
				}()
				select {
				case <-started:
				case <-time.After(30 * time.Second):
					t.Fatal("HTTP request never started")
				}
				if !deadline {
					cancel()
				}
				select {
				case err := <-done:
					if !errors.Is(err, want) {
						t.Fatalf("request returned %v", err)
					}
				case <-time.After(30 * time.Second):
					t.Fatal("canceled request did not finish")
				}
				select {
				case <-disconnected:
				case <-time.After(time.Second):
					t.Fatal("HTTP helper survived completed cancellation")
				}
				if err := os.Rename(repo.Root, repo.Root+"-probe"); err != nil {
					t.Fatalf("completed cancellation still holds worktree: %v", err)
				}
				if err := os.Rename(repo.Root+"-probe", repo.Root); err != nil {
					t.Fatal(err)
				}
				if git("rev-parse", "HEAD") != repo.Head || git("write-tree") != index || git("status", "--porcelain=v1", "-z") != status || git("rev-parse", "origin/topic") != tracking {
					t.Fatal("cancellation changed repository state")
				}
			})
		}
	}
}
