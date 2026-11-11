package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeRepository(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "项目 with spaces")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	git := New()
	t.Setenv("GIT_DIR", filepath.Join(parent, "wrong.git"))
	if _, err := git.Initialize(context.Background(), dir, "bad branch"); err == nil {
		t.Fatal("invalid initial branch accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid initialization modified the directory: %v", err)
	}
	if _, err := git.Initialize(context.Background(), dir, "main"); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "--git-dir="+filepath.Join(dir, ".git"), "symbolic-ref", "--short", "HEAD").Output(); err != nil || strings.TrimSpace(string(out)) != "main" {
		t.Fatalf("HEAD = %q, %v", out, err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "keep" {
		t.Fatalf("existing file changed: %q, %v", data, err)
	}
	if _, err := git.Initialize(context.Background(), dir, "different"); !errors.Is(err, ErrExistingRepository) {
		t.Fatalf("reinitialization returned %v", err)
	}
	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Initialize(context.Background(), nested, "main"); !errors.Is(err, ErrExistingRepository) {
		t.Fatalf("nested initialization returned %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := git.Initialize(ctx, parent, "main"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled initialization returned %v", err)
	}
}
