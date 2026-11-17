package bootstrap

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"vela/internal/domain"
	velaruntime "vela/internal/runtime"
)

// repositoryWatch is configured on the main thread; workers retain only copied
// paths and contexts, never Workspace state. Only the active repository is held.
type repositoryWatch struct {
	root, gitDir, commonDir string
	cancel                  context.CancelFunc
}

func (w *repositoryWatch) close() {
	if w.cancel != nil {
		w.cancel()
	}
	w.cancel = nil
	w.root, w.gitDir, w.commonDir = "", "", ""
}

func (w *repositoryWatch) configure(repo domain.Repository, pulse func(string, bool, time.Time), report func(string, string)) {
	if w.root == repo.Root && w.gitDir == repo.GitDir && w.commonDir == repo.CommonDir {
		return
	}
	w.close()
	if repo.Root == "" {
		return
	}
	// Watch goroutines must not retain a potentially large repository/log snapshot.
	repo = domain.Repository{Root: repo.Root, GitDir: repo.GitDir, CommonDir: repo.CommonDir}
	w.root, w.gitDir, w.commonDir = repo.Root, repo.GitDir, repo.CommonDir
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	changes := make(chan struct{}, 1)
	hint := func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	}
	go velaruntime.RunRefreshLoop(ctx, changes, func(dirty bool, now time.Time) {
		mygo.RunOnMain(func() {
			if ctx.Err() == nil {
				pulse(repo.Root, dirty, now)
			}
		})
	})
	paths := []string{repo.Root}
	for _, path := range []string{repo.GitDir, repo.CommonDir} {
		if path == "" {
			continue
		}
		duplicate := false
		for _, other := range paths {
			duplicate = duplicate || filepath.Clean(other) == filepath.Clean(path)
		}
		if !duplicate {
			paths = append(paths, path)
		}
	}
	for i, directory := range paths {
		go func() {
			ignore := func(name string, isDir bool) bool {
				if !isDir {
					return false
				}
				base := strings.ToLower(filepath.Base(name))
				if i == 0 {
					for _, metadata := range []string{repo.GitDir, repo.CommonDir} {
						if rel, err := filepath.Rel(repo.Root, metadata); err == nil && filepath.ToSlash(rel) == name {
							return true
						}
					}
					return base == ".git" || base == "node_modules" || base == "build" || base == "dist" || base == "target" || base == ".cache"
				}
				return base == "objects" || base == "logs" || base == "hooks" || base == "modules" || base == "worktrees"
			}
			watch, err := mygo.WatchFiles(ctx, directory, mygo.FileWatchOptions{Recursive: true, HintsOnly: true, Ignore: ignore, Debounce: 400 * time.Millisecond, MaxDelay: 2 * time.Second, MaxEntries: 8192})
			if err == nil {
				defer watch.Close()
				// Close the read-to-watch enrollment gap with one coalesced refresh.
				hint()
				for {
					var events []mygo.FileEvent
					events, err = watch.Next(ctx)
					if len(events) > 0 {
						hint()
					}
					if err != nil {
						break
					}
				}
			}
			if ctx.Err() == nil && err != nil {
				message := "File watch unavailable; refresh manually · " + err.Error()
				if errors.Is(err, io.EOF) {
					message = "Watched directory disappeared; reopen the repository"
				}
				mygo.RunOnMain(func() {
					if ctx.Err() == nil {
						report(repo.Root, message)
					}
				})
			}
		}()
	}
}
