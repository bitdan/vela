package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"vela/internal/domain"
)

func TestRemoteURLListsAtomicAndOrdered(t *testing.T) {
	f := newWorkflow(t)
	ctx := context.Background()
	key := "remote.origin."
	old := "https://example.test/first.git"
	second := "ssh://git@example.test/second.git"
	push := "https://example.test/push.git"
	f.git("remote", "add", "origin", old)
	f.git("config", "--add", key+"url", second)
	f.git("config", "--add", key+"pushurl", push)
	r := f.read()
	if len(r.Remotes) != 1 || r.Remotes[0].URL != old || !slices.Equal(r.Remotes[0].FetchURLs, []string{old, second}) || !slices.Equal(r.Remotes[0].PushURLs, []string{push}) {
		t.Fatalf("URL order lost on read: %+v", r.Remotes)
	}
	op := domain.RepositoryOperation{Kind: domain.EditRemoteURLs, Remote: "origin", FetchURLs: []string{second, old}, PushURLs: []string{push, old}}
	bad := op
	bad.PushURLs = []string{"https://username:password@example.test/unsupported"}
	if err := f.g.ExecuteRepositoryOperation(ctx, r, bad); err == nil {
		t.Fatal("stored unsupported URL")
	}
	lock := filepath.Join(r.CommonDir, "config.lock")
	if err := os.WriteFile(lock, []byte("other Git writer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.g.ExecuteRepositoryOperation(ctx, r, op); err == nil {
		t.Fatal("replaced concurrent lock")
	}
	if bytes, err := os.ReadFile(lock); err != nil || string(bytes) != "other Git writer" {
		t.Fatal("another writer's lock changed")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(f.read().Remotes[0].FetchURLs, []string{old, second}) {
		t.Fatal("failed saves changed the URL list")
	}
	if err := f.g.ExecuteRepositoryOperation(ctx, r, op); err != nil {
		t.Fatal(err)
	}
	updated := f.read()
	if !slices.Equal(updated.Remotes[0].FetchURLs, op.FetchURLs) || !slices.Equal(updated.Remotes[0].PushURLs, op.PushURLs) || updated.Head != r.Head {
		t.Fatalf("wrong saved configuration: %+v", updated.Remotes)
	}
	if err := f.g.verifyRemote(ctx, updated, "origin", true); err == nil {
		t.Fatal("multiple push destinations enabled Push")
	}
	if err := f.g.ExecuteRepositoryOperation(ctx, r, op); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale edit not rejected: %v", err)
	}
	op.PushURLs = nil
	if err := f.g.ExecuteRepositoryOperation(ctx, updated, op); err != nil {
		t.Fatal(err)
	}
	if got := f.read().Remotes[0]; len(got.PushURLs) != 0 || !slices.Equal(got.FetchURLs, op.FetchURLs) {
		t.Fatalf("could not clear explicit push URLs: %+v", got)
	}
}
