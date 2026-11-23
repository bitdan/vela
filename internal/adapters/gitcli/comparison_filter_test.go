package gitcli

import (
	"context"
	"testing"

	"vela/internal/domain"
)

func TestComparisonFiltersUseLiteralPathsAndPaginateMatches(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("checkout", "-b", "filtered-topic")
	f.write("literal[1].txt", "first\n")
	f.git("add", "--", "literal[1].txt")
	f.git("commit", "--author", "Filter Author <filter@example.test>", "-m", "Needle [text]")
	first := f.git("rev-parse", "HEAD")
	f.write("literal1.txt", "different\n")
	f.git("add", "--", "literal1.txt")
	f.git("commit", "--author", "Other <other@example.test>", "-m", "Needle [text]")
	f.write("literal[1].txt", "last\n")
	f.git("add", "--", "literal[1].txt")
	f.git("commit", "--author", "Filter Author <filter@example.test>", "-m", "Needle [text]")
	last := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	r := f.read()
	comparison, err := f.g.ReadMissingBranchComparison(context.Background(), r, "refs/heads/filtered-topic")
	if err != nil {
		t.Fatal(err)
	}
	comparison.Filter = domain.ComparisonLogFilter{Message: "NEEDLE [", Author: "filter author", Path: "literal[1].txt"}
	for offset, want := range []string{last, first} {
		page, err := f.g.ReadComparisonCommits(context.Background(), r.Root, comparison, offset, 1)
		if err != nil || len(page.Commits) != 1 || page.Commits[0].Hash != want || page.HasMore != (offset == 0) {
			t.Fatalf("filtered page %d: %+v %v", offset, page, err)
		}
	}
	comparison.Filter.Since = "not a date"
	if _, err := f.g.ReadComparisonCommits(context.Background(), r.Root, comparison, 0, 100); err == nil {
		t.Fatal("invalid date reached Git")
	}
	if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion || after.WorktreeVersion != r.WorktreeVersion {
		t.Fatal("comparison filtering changed repository")
	}
}
