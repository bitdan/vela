package bootstrap

import (
	"context"
	"testing"
	"vela/internal/domain"
)

func TestRepositoryWatchReplacementCancelsOldOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := repositoryWatch{root: "old", gitDir: "old/.git", commonDir: "old/.git", cancel: cancel}
	// The empty active repository must tear down monitoring without starting IO.
	w.configure(domain.Repository{}, nil, nil)
	if ctx.Err() != context.Canceled || w.cancel != nil || w.root != "" || w.gitDir != "" || w.commonDir != "" {
		t.Fatal("old watch remained active")
	}
	w.close()
}

func TestBackgroundTaskKeysSeparateRepositoriesAndPolicies(t *testing.T) {
	keys := []string{taskKey("remote", "repo-a", true, false), taskKey("remote", "repo-b", true, false), taskKey("remote", "repo-a", false, true), taskKey("refresh", "repo-a", true, false)}
	seen := map[string]bool{}
	for _, key := range keys {
		if seen[key] {
			t.Fatal("distinct background queries deduplicated")
		}
		seen[key] = true
	}
	if taskKey("remote", "repo-a", true, false) != keys[0] {
		t.Fatal("identical request lost deduplication")
	}
}
