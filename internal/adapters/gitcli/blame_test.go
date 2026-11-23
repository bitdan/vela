package gitcli

import (
	"context"
	"errors"
	"testing"
)

func TestBlameCommittedLinesAndRename(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	first := f.git("rev-parse", "HEAD")
	f.write("文件 name.txt", "base\nsecond line\n")
	f.git("add", ".")
	f.git("-c", "user.name=Second Author", "commit", "-m", "second line")
	second := f.git("rev-parse", "HEAD")
	path := "renamed 中文 file.txt"
	f.git("mv", "文件 name.txt", path)
	f.git("commit", "-m", "rename")
	f.write(path, "unstaged replacement\n")
	doc, err := f.g.ReadBlame(context.Background(), f.root, f.git("rev-parse", "HEAD"), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Lines) != 2 || doc.Lines[0].Commit != first || doc.Lines[0].Text != "base" || doc.Lines[1].Commit != second || doc.Lines[1].Author != "Second Author" || doc.Lines[1].Text != "second line" || doc.Lines[1].Line != 2 {
		t.Fatalf("wrong attribution: %+v", doc)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.g.ReadBlame(ctx, f.root, second, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel lost: %v", err)
	}
	for _, bad := range []string{"../escape", ".git/config"} {
		if _, err := f.g.ReadBlame(context.Background(), f.root, second, bad); err == nil {
			t.Fatal("invalid path accepted")
		}
	}
}

func TestBlameNewlinePathInGitObject(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	// Build an object-only fixture. Windows cannot check out a newline name;
	// this tests Git's path protocol without creating that filesystem path.
	f.git("config", "core.protectNTFS", "false")
	path := "object\npath.txt"
	blob := f.git("rev-parse", "HEAD:文件 name.txt")
	f.git("update-index", "--add", "--cacheinfo", "100644,"+blob+","+path)
	f.git("commit", "-m", "index-only newline path")
	doc, err := f.g.ReadBlame(context.Background(), f.root, f.git("rev-parse", "HEAD"), path)
	if err != nil || len(doc.Lines) != 1 || doc.Lines[0].Text != "base" {
		t.Fatalf("newline Git path: %+v %v", doc, err)
	}
}

func TestParseBlameRejectsMalformedOutput(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"garbage\n", "abc 1 1 1\n\ttext\n", "0000000000000000000000000000000000000000 1 0 1\n\ttext\n"} {
		if _, err := parseBlame(value); err == nil {
			t.Fatalf("malformed blame accepted %q", value)
		}
	}
}
