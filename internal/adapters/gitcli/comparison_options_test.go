package gitcli

import (
	"context"
	"encoding/json"
	"testing"

	"vela/internal/domain"
)

func TestComparisonHashRegexAndCaseOptions(t *testing.T) {
	f := newWorkflow(t)
	f.git("checkout", "-b", "options")
	var ids []string
	for _, message := range []string{"Fix One", "fix two", "Fix Three"} {
		f.write("文件 name.txt", message+"\n")
		f.git("add", "--", "文件 name.txt")
		f.git("commit", "-m", message)
		ids = append(ids, f.git("rev-parse", "HEAD"))
	}
	f.git("checkout", "main")
	r := f.read()
	c, err := f.g.ReadMissingBranchComparison(context.Background(), r, "refs/heads/options")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, filter string
		want         []string
	}{
		{"case sensitive", `{"Message":"Fix","CaseSensitive":true}`, []string{ids[2], ids[0]}},
		{"regex", `{"Message":"^fix (one|three)$","Regex":true}`, []string{ids[2], ids[0]}},
		{"hash", `{"Hash":"` + ids[1][:9] + `"}`, []string{ids[1]}},
		{"combined", `{"Message":"fix","Hash":"` + ids[0] + `"}`, []string{ids[0]}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c.Filter = domain.ComparisonLogFilter{}
			if err := json.Unmarshal([]byte(tc.filter), &c.Filter); err != nil {
				t.Fatal(err)
			}
			for i, want := range tc.want {
				page, err := f.g.ReadComparisonCommits(context.Background(), r.Root, c, i, 1)
				if err != nil || len(page.Commits) != 1 || page.Commits[0].Hash != want || page.HasMore != (i+1 < len(tc.want)) {
					t.Fatalf("page %d: %+v %v; want %s", i, page, err, want)
				}
			}
		})
	}
	if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion || after.WorktreeVersion != r.WorktreeVersion {
		t.Fatal("query changed repository")
	}
}
