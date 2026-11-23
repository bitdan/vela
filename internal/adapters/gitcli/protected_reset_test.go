package gitcli

import (
	"context"
	"testing"

	"vela/internal/domain"
)

func TestProtectedResetToHeadRecoversStandaloneStashConflict(t *testing.T) {
	for _, mode := range []string{"Mixed", "Hard"} {
		t.Run(mode, func(t *testing.T) {
			f := newWorkflow(t)
			base := f.git("rev-parse", "HEAD")
			f.write("文件 name.txt", "stash side\n")
			f.git("stash", "push", "-m", "keep original")
			f.write("文件 name.txt", "new committed side\n")
			f.git("commit", "-am", "advance")
			r := f.read()
			entry := r.Stashes[0]
			if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyStash, Source: entry.OID, Target: entry.Selector}); err == nil {
				t.Fatal("expected conflict")
			}
			r = f.read()
			if len(r.Conflicts) == 0 || len(r.MergeHeads) != 0 {
				t.Fatal("fixture lacks standalone conflict")
			}
			policy := domain.RepositorySafetyPolicy{ProtectedBranches: "main;master"}
			if err := f.g.ExecuteRepositoryOperation(context.Background(), r, domain.RepositoryOperation{Kind: domain.ResetRevision, Source: base, Mode: mode, Safety: policy}); err == nil {
				t.Fatal("protected branch moved to older commit")
			}
			if after := f.read(); after.Head != r.Head || after.IndexVersion != r.IndexVersion || after.WorktreeVersion != r.WorktreeVersion {
				t.Fatal("rejected reset changed repository")
			}
			if err := f.g.ExecuteRepositoryOperation(context.Background(), r, domain.RepositoryOperation{Kind: domain.ResetRevision, Source: r.Head, Mode: mode, Safety: policy}); err != nil {
				t.Fatal(err)
			}
			after := f.read()
			if after.Head != r.Head || len(after.Conflicts) != 0 || len(after.Stashes) != 1 || after.Stashes[0].OID != entry.OID {
				t.Fatal("recovery moved HEAD or lost stash")
			}
			if f.git("show", ":文件 name.txt") != "new committed side" {
				t.Fatal("recovery index differs from HEAD")
			}
		})
	}
}
