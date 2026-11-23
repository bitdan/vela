package gitcli

import (
	"context"
	"testing"

	"vela/internal/application"
)

func TestCommitPolicyMessageAndStagedTODO(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "base\n// TODO: staged addition\n")
	f.git("add", ".")
	snapshot := f.read()
	head := f.git("rev-parse", "HEAD")
	index := f.git("write-tree")
	for _, tc := range []struct {
		message string
		prefs   application.CommitSettings
	}{
		{"long subject", application.CommitSettings{LimitSubject: true, SubjectLength: 4}},
		{"subject\nbody", application.CommitSettings{BlankLine: true}},
		{"subject\n\nlong body", application.CommitSettings{LimitBody: true, BodyLength: 4}},
		{"valid message", application.CommitSettings{CheckTODO: true}},
	} {
		if err := f.g.Commit(context.Background(), snapshot, tc.message, false, application.CommitPolicy{Inspections: tc.prefs}); err == nil {
			t.Fatalf("policy ignored for %+v", tc)
		}
		if f.git("rev-parse", "HEAD") != head || f.git("write-tree") != index {
			t.Fatal("failed inspection wrote a commit or index")
		}
	}
	f.write("文件 name.txt", "base\n// staged clean\n")
	f.git("add", ".")
	f.write("文件 name.txt", "base\n// TODO: unstaged only\n")
	if err := f.g.Commit(context.Background(), f.read(), "clean staged change", false, application.CommitPolicy{Inspections: application.CommitSettings{CheckTODO: true}}); err != nil {
		t.Fatalf("unstaged TODO blocked commit: %v", err)
	}
	if f.git("show", "HEAD:文件 name.txt") != "base\n// staged clean" {
		t.Fatal("commit included unstaged content")
	}
}

func TestCommitPolicyExistingTODOIsAllowed(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "// TODO: existing\nbase\n")
	f.git("add", ".")
	f.git("commit", "-m", "existing TODO")
	f.write("文件 name.txt", "// TODO: existing\nchanged\n")
	f.git("add", ".")
	if err := f.g.Commit(context.Background(), f.read(), "unrelated change", false, application.CommitPolicy{Inspections: application.CommitSettings{CheckTODO: true}}); err != nil {
		t.Fatal(err)
	}
}

func TestCommitPolicyTODOWithPlusPrefix(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "base\n++TODO: still a new TODO\n")
	f.git("add", ".")
	if err := f.g.Commit(context.Background(), f.read(), "plus prefixed", false, application.CommitPolicy{Inspections: application.CommitSettings{CheckTODO: true}}); err == nil {
		t.Fatal("plus-prefixed staged TODO bypassed inspection")
	}
}
