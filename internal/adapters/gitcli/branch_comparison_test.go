package gitcli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestBranchComparisonFilesAndWorkingSnapshot(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("delete.txt", "deleted on topic\n")
	f.write("main.txt", "base\n")
	f.git("add", ".")
	f.git("commit", "-m", "common")
	f.git("checkout", "-b", "topic")
	f.git("mv", "文件 name.txt", "renamed space.txt")
	f.git("rm", "delete.txt")
	f.write("binary.dat", "binary\x00bytes")
	f.git("add", ".")
	f.git("commit", "-m", "topic")
	f.git("checkout", "main")
	f.write("main.txt", "main\n")
	f.git("commit", "-am", "main")
	r := f.read()
	ctx := context.Background()
	comparison, err := f.g.ReadBranchComparison(ctx, r, "refs/heads/topic", false)
	if err != nil {
		t.Fatal(err)
	}
	if comparison.Left != r.Refs["refs/heads/topic"] || comparison.Right != r.Head || len(comparison.Files) != 4 {
		t.Fatalf("wrong comparison: %+v", comparison)
	}
	var rename domain.File
	for _, file := range comparison.Files {
		if file.Status == "R" {
			rename = file
		}
	}
	if rename.Path != "文件 name.txt" || rename.OldPath != "renamed space.txt" {
		t.Fatalf("rename lost: %+v", rename)
	}
	file, err := f.g.ReadFile(ctx, r.Root, comparison.Source(), rename)
	if err != nil || file.Old != "base\n" || file.New != "base\n" {
		t.Fatalf("rename preview: %+v %v", file, err)
	}
	added, _ := matchHistoryFile(comparison.Files, "delete.txt")
	file, err = f.g.ReadFile(ctx, r.Root, comparison.Source(), added)
	if err != nil || file.Old != "" || file.New != "deleted on topic\n" {
		t.Fatalf("addition direction: %+v %v", file, err)
	}
	if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion {
		t.Fatal("comparison changed checkout")
	}
	f.write("main.txt", "working\n")
	r = f.read()
	working, err := f.g.ReadBranchComparison(ctx, r, "refs/heads/topic", true)
	if err != nil || working.Right != "" || working.WorktreeVersion != r.WorktreeVersion {
		t.Fatalf("working query: %+v %v", working, err)
	}
	modified, _ := matchHistoryFile(working.Files, "main.txt")
	file, err = f.g.ReadFile(ctx, r.Root, working.Source(), modified)
	if err != nil || file.New != "working\n" {
		t.Fatalf("working preview: %+v %v", file, err)
	}
	f.write("main.txt", "external\n")
	if _, err := f.g.ReadFile(ctx, r.Root, working.Source(), modified); err == nil {
		t.Fatal("opened stale working comparison")
	}
	if _, err := f.g.ReadBranchComparison(ctx, r, "refs/heads/topic", true); err == nil {
		t.Fatal("accepted stale working snapshot")
	}
}

func TestBranchComparisonRemoteTagsAndMissingWithoutTreeLimit(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("checkout", "-b", "topic")
	f.write("文件 name.txt", "topic\n")
	f.git("commit", "-am", "topic")
	f.git("tag", "-a", "release", "-m", "annotated tag")
	remote := t.TempDir()
	f.git("init", "--bare", remote)
	f.git("remote", "add", "origin", remote)
	f.git("push", "origin", "topic")
	f.git("checkout", "main")
	r := f.read()
	for _, ref := range []string{"refs/remotes/origin/topic", "refs/tags/release"} {
		comparison, err := f.g.ReadBranchComparison(context.Background(), r, ref, false)
		if err != nil || len(comparison.Files) != 1 || comparison.Left != r.Refs["refs/heads/topic"] {
			t.Fatalf("comparison %s: %+v %v", ref, comparison, err)
		}
	}
	makeObject := func(input string, args ...string) string {
		out, err := f.g.runInput(context.Background(), f.root, input, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out.stdout)
	}
	blob := makeObject("data\n", "hash-object", "-w", "--stdin")
	var entries strings.Builder
	for i := 0; i < 10001; i++ {
		fmt.Fprintf(&entries, "100644 blob %s\tf%05d.txt\x00", blob, i)
	}
	tree := makeObject(entries.String(), "mktree", "-z")
	oid := makeObject("independent large tree\n", "commit-tree", tree)
	f.git("update-ref", "refs/heads/big", oid)
	r = f.read()
	if _, err := f.g.ReadBranchComparison(context.Background(), r, "refs/heads/big", false); err == nil {
		t.Fatal("file comparison exceeded bound without notice")
	}
	comparison, err := f.g.ReadMissingBranchComparison(context.Background(), r, "refs/heads/big")
	if err != nil {
		t.Fatal(err)
	}
	page, err := f.g.ReadComparisonCommits(context.Background(), r.Root, comparison, 0, 100)
	if err != nil || len(page.Commits) != 1 || page.Commits[0].Hash != oid || page.HasMore {
		t.Fatalf("missing query incorrectly depends on tree size or related history: %+v %v", page, err)
	}
}

func TestBranchComparisonRejectsConflictedWorkingSnapshot(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("checkout", "-b", "topic")
	f.write("文件 name.txt", "incoming\n")
	f.git("commit", "-am", "incoming")
	f.git("checkout", "main")
	f.write("文件 name.txt", "current\n")
	f.git("commit", "-am", "current")
	ctx := context.Background()
	if _, err := f.g.run(ctx, f.root, "merge", "topic"); err == nil {
		t.Fatal("fixture did not conflict")
	}
	r := f.read()
	if !r.ResolvingConflicts() {
		t.Fatal("fixture did not expose a conflict")
	}
	if _, err := f.g.ReadBranchComparison(ctx, r, "refs/heads/topic", true); err == nil {
		t.Fatal("accepted conflicted working comparison")
	}
	if _, err := f.g.ReadBranchComparison(ctx, r, "refs/heads/topic", false); err != nil {
		t.Fatalf("committed comparison should remain readable: %v", err)
	}
	if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion || after.WorktreeVersion != r.WorktreeVersion {
		t.Fatal("read-only comparison altered conflict state")
	}
}

func matchHistoryFile(files []domain.File, path string) (domain.File, bool) {
	for _, file := range files {
		if file.Path == path {
			return file, true
		}
	}
	return domain.File{}, false
}

func TestMissingBranchCommitsCompleteAndFrozen(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("checkout", "-b", "topic")
	for _, message := range []string{"one", "two", "three", "four", "five"} {
		f.git("commit", "--allow-empty", "-m", message)
	}
	f.git("checkout", "main")
	r := f.read()
	r.History = nil
	comparison, err := f.g.ReadBranchComparison(context.Background(), r, "refs/heads/topic", false)
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for offset := 0; offset < 5; offset += 2 {
		page, err := f.g.ReadComparisonCommits(context.Background(), r.Root, comparison, offset, 2)
		if err != nil {
			t.Fatal(err)
		}
		if page.HasMore != (offset < 4) {
			t.Fatal("incorrect missing commit lookahead")
		}
		for _, commit := range page.Commits {
			if slices.Contains(hashes, commit.Hash) {
				t.Fatal("repeated commit")
			}
			hashes = append(hashes, commit.Hash)
		}
	}
	if len(hashes) != 5 {
		t.Fatal("missing commits depends on loaded Log")
	}
	f.git("update-ref", "refs/heads/topic", r.Head)
	page, err := f.g.ReadComparisonCommits(context.Background(), r.Root, comparison, 0, 2)
	if err != nil || len(page.Commits) != 2 || page.Commits[0].Hash != hashes[0] {
		t.Fatal("ref motion shifted snapshot")
	}
	if _, err := f.g.ReadBranchComparison(context.Background(), r, "refs/heads/topic", false); err == nil {
		t.Fatal("accepted stale source ref")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.g.ReadComparisonCommits(ctx, r.Root, comparison, 0, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}
