package gitcli

import (
	"context"
	"testing"

	"vela/internal/domain"
)

type comparisonChangesReader interface {
	ReadComparisonChanges(context.Context, string, domain.RevisionComparison, []string) ([]domain.ComparisonChangeGroup, error)
}

func TestComparisonSelectionNetChangesAndGaps(t *testing.T) {
	f := newWorkflow(t)
	reader, ok := any(f.g).(comparisonChangesReader)
	if !ok {
		t.Fatal("comparison selection reader is missing")
	}
	f.git("checkout", "-b", "net")
	var ids []string
	for _, value := range []string{"added", "reverted", "last"} {
		if value == "added" {
			f.write("temporary.txt", "temporary\n")
		}
		if value == "reverted" {
			f.git("rm", "temporary.txt")
		}
		if value == "last" {
			f.write("final 文件.txt", "final\n")
		}
		f.git("add", "-A")
		f.git("commit", "-m", value)
		ids = append(ids, f.git("rev-parse", "HEAD"))
	}
	f.git("checkout", "main")
	r := f.read()
	c, err := f.g.ReadMissingBranchComparison(context.Background(), r, "refs/heads/net")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := reader.ReadComparisonChanges(context.Background(), r.Root, c, []string{ids[1], ids[2], ids[0]})
	if err != nil || len(groups) != 1 || len(groups[0].Files) != 1 || groups[0].Files[0].Path != "final 文件.txt" {
		t.Fatalf("net: %+v %v", groups, err)
	}
	left, right, valid := domain.ParseComparisonSource(groups[0].Source)
	if !valid || left != r.Head || right != ids[2] {
		t.Fatalf("wrong net diff identity: %+v", groups)
	}
	groups, err = reader.ReadComparisonChanges(context.Background(), r.Root, c, []string{ids[2], ids[0]})
	if err != nil || len(groups) != 2 {
		t.Fatalf("gaps must retain separate groups: %+v %v", groups, err)
	}
	if _, err = reader.ReadComparisonChanges(context.Background(), r.Root, c, []string{r.Head}); err == nil {
		t.Fatal("accepted a commit outside the fixed range")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = reader.ReadComparisonChanges(ctx, r.Root, c, ids); err == nil {
		t.Fatal("ignored cancellation")
	}
	if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion || after.WorktreeVersion != r.WorktreeVersion {
		t.Fatal("query changed repository")
	}
}

func TestComparisonMergeGroupsKeepDistinctParentSources(t *testing.T) {
	f := newWorkflow(t)
	reader, ok := any(f.g).(comparisonChangesReader)
	if !ok {
		t.Fatal("comparison selection reader is missing")
	}
	f.git("checkout", "-b", "left")
	f.write("left.txt", "left\n")
	f.git("add", "-A")
	f.git("commit", "-m", "left")
	first := f.git("rev-parse", "HEAD")
	f.git("checkout", "-b", "right", "main")
	f.write("right.txt", "right\n")
	f.git("add", "-A")
	f.git("commit", "-m", "right")
	second := f.git("rev-parse", "HEAD")
	f.git("checkout", "left")
	f.git("merge", "--no-ff", "right", "-m", "merge")
	merge := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	r := f.read()
	c, err := f.g.ReadMissingBranchComparison(context.Background(), r, "refs/heads/left")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := reader.ReadComparisonChanges(context.Background(), r.Root, c, []string{merge})
	if err != nil || len(groups) != 2 {
		t.Fatalf("merge groups: %+v %v", groups, err)
	}
	for i, tc := range []struct{ parent, path string }{{first, "right.txt"}, {second, "left.txt"}} {
		left, right, ok := domain.ParseComparisonSource(groups[i].Source)
		if !ok || left != tc.parent || right != merge || len(groups[i].Files) != 1 || groups[i].Files[0].Path != tc.path {
			t.Fatalf("parent %d: %+v", i+1, groups[i])
		}
	}
}

func TestComparisonNetSelectionIncludesRootCommit(t *testing.T) {
	f := newWorkflow(t)
	f.git("checkout", "--orphan", "unrelated")
	f.git("rm", "-r", "-f", "--", ".")
	f.write("root.txt", "root\n")
	f.git("add", "-A")
	f.git("commit", "-m", "root")
	first := f.git("rev-parse", "HEAD")
	f.write("second.txt", "second\n")
	f.git("add", "-A")
	f.git("commit", "-m", "second")
	last := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	r := f.read()
	c, err := f.g.ReadMissingBranchComparison(context.Background(), r, "refs/heads/unrelated")
	if err != nil {
		t.Fatal(err)
	}
	groups, err := f.g.ReadComparisonChanges(context.Background(), r.Root, c, []string{last, first})
	if err != nil || len(groups) != 1 || len(groups[0].Files) != 2 {
		t.Fatalf("root net changes: %+v %v", groups, err)
	}
	for _, file := range groups[0].Files {
		if file.Status != "A" {
			t.Fatalf("root net should contain additions: %+v", file)
		}
		loaded, err := f.g.ReadFile(context.Background(), r.Root, groups[0].Source, file)
		if err != nil || loaded.Old != "" || loaded.New == "" {
			t.Fatalf("root net diff: %+v %v", loaded, err)
		}
	}
}
