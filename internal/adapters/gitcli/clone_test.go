package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneRepositoryFromBareRemote(t *testing.T) {
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(config, []byte("[core]\n autocrlf = false\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	f := newWorkflow(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	f.git("clone", "--bare", f.root, bare)
	target := filepath.Join(t.TempDir(), "克隆 with spaces")
	root, err := f.g.Clone(context.Background(), bare, target)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := f.g.ReadRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Head != f.git("rev-parse", "HEAD") || repo.CurrentBranch != "main" || len(repo.Remotes) != 1 || repo.Remotes[0].Name != "origin" {
		t.Fatalf("wrong clone %+v", repo)
	}
	data, err := os.ReadFile(filepath.Join(root, "文件 name.txt"))
	if err != nil || string(data) != "base\n" {
		t.Fatalf("checkout missing: %q %v", data, err)
	}
}

func TestCloneProtectsTargetAndRejectsInvalidInput(t *testing.T) {
	f := newWorkflow(t)
	existing := t.TempDir()
	sentinel := filepath.Join(existing, "keep.txt")
	if err := os.WriteFile(sentinel, []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ source, target string }{
		{f.root, existing}, {f.root, filepath.Join(f.root, "nested")}, {f.root, "relative"},
		{"ext::arbitrary-helper", filepath.Join(t.TempDir(), "new")}, {"https://token@example.test/repo.git", filepath.Join(t.TempDir(), "new")},
	} {
		if _, err := f.g.Clone(context.Background(), tc.source, tc.target); err == nil {
			t.Fatalf("invalid clone accepted: %+v", tc)
		}
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "user data" {
		t.Fatal("existing target changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	target := filepath.Join(t.TempDir(), "canceled")
	if _, err := f.g.Clone(ctx, f.root, target); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("pre-canceled clone created target")
	}
}
