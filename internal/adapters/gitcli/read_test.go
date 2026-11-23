package gitcli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestReadRepository(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx := context.Background()
	g := New()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Test", "-c", "user.email=test@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", args, err, out)
		}
	}
	name := "中文 file.txt"
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	empty, err := g.ReadRepository(ctx, dir)
	if err != nil || empty.CurrentBranch != "main" || len(empty.History) != 0 {
		t.Fatalf("unborn repository: %+v, %v", empty, err)
	}
	write("committed\n")
	git("add", "--", name)
	git("commit", "-m", "First", "-m", "Body")
	git("tag", "-a", "v1", "-m", "Version")
	git("branch", "feature/read")
	git("remote", "add", "upstream", "https://example.test/project.git")
	git("update-ref", "refs/remotes/upstream/main", "HEAD")
	write("staged\n")
	git("add", "--", name)
	write("working\n")
	indexPath := filepath.Join(dir, ".git", "index")
	before, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.History) != 1 || len(repo.Working) != 1 || len(repo.Staged) != 1 || len(repo.Project) != 1 ||
		len(repo.Remotes) != 1 || len(repo.Branches) != 4 || !repo.RemoteBranches["upstream/main"] {
		t.Fatalf("unexpected snapshot: %+v", repo)
	}
	commit := repo.History[0]
	if commit.Subject != "First" || strings.TrimSpace(commit.Body) != "Body" || len(commit.Refs) != 4 ||
		len(commit.Files) != 1 || commit.Files[0].Path != name {
		t.Fatalf("unexpected commit: %+v", commit)
	}
	for _, check := range []struct {
		source   string
		file     domain.File
		old, new string
	}{
		{"working", repo.Working[0], "staged\n", "working\n"},
		{"staged", repo.Staged[0], "committed\n", "staged\n"},
		{commit.Hash, commit.Files[0], "", "committed\n"},
	} {
		file, err := g.ReadFile(ctx, repo.Root, check.source, check.file)
		if err != nil || file.Old != check.old || file.New != check.new {
			t.Fatalf("%s: got %+v, %v", check.source, file, err)
		}
	}
	after, err := os.Stat(indexPath)
	if err != nil || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("read operations modified the index: %v", err)
	}
	git("reset", "--hard", "HEAD")
	git("checkout", "feature/read")
	git("mv", "--", name, "renamed 中文.txt")
	git("commit", "-m", "Rename")
	git("checkout", "main")
	git("commit", "--allow-empty", "-m", "Empty commit")
	git("merge", "--no-ff", "--no-edit", "feature/read")
	repo, err = g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.History) != 4 || !repo.History[0].Merge || len(repo.History[0].Parents) != 2 {
		t.Fatalf("merge history: %+v", repo.History)
	}
	for _, entry := range repo.History {
		switch entry.Subject {
		case "Empty commit":
			if len(entry.Files) != 0 {
				t.Fatalf("empty commit has changes: %+v", entry.Files)
			}
		case "Rename":
			if len(entry.Files) != 1 || entry.Files[0].OldPath != name || entry.Files[0].Path != "renamed 中文.txt" {
				t.Fatalf("rename lost its paths: %+v", entry.Files)
			}
		}
	}
	git("checkout", "--detach", "HEAD")
	repo, err = g.ReadRepository(ctx, dir)
	if err != nil || repo.CurrentBranch != "Detached HEAD" {
		t.Fatalf("detached repository: %q, %v", repo.CurrentBranch, err)
	}
}

func TestParseLargeHistory(t *testing.T) {
	t.Parallel()
	const count = 10000
	var log strings.Builder
	for range count {
		log.WriteString("\x00commit\x00\x00Author\x00test@example.test\x001\x001\x00Subject\x00\x00\nM\x00space and\nnewline.txt\x00")
	}
	history, err := parseHistory(log.String(), nil)
	if err != nil || len(history) != count || history[count-1].Files[0].Path != "space and\nnewline.txt" {
		t.Fatalf("parse %d commits: got %d, %v", count, len(history), err)
	}
}
