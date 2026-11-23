package ui

import (
	"context"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRealCommitMessageInspections(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Staged = []domain.File{{Path: "a.go", Status: "A"}}
	a.Model.Switch(a.installRepository(repo))
	a.CommitRepository = func(ctx context.Context, repo domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
		t.Fatal("invalid commit was submitted")
	}
	a.Layout.VersionControl.Commit = application.CommitSettings{LimitSubject: true, SubjectLength: 5}
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Commit message"); err != nil {
		t.Fatal(err)
	}
	tt.Type("too long")
	if button, ok := tt.Access("Create Commit"); !ok || !button.Disabled {
		t.Fatal("invalid subject did not disable commit")
	}
	if !strings.Contains(a.disabledFor(commitRepository, commandTarget{}), "Subject exceeds") {
		t.Fatal("missing inspection reason")
	}
}

func TestDefaultProtectedBranchStillBlocksHistoryRewrite(t *testing.T) {
	a, _ := advancedWorkspace(t)
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		t.Fatal("protected reset dispatched")
	}
	a.CommitRepository = func(context.Context, domain.Repository, string, bool, func(domain.Repository, error, error)) {
		t.Fatal("protected amend dispatched")
	}
	s := a.Model.Active()
	s.Message, s.Amend = "rewrite", true
	if !strings.Contains(a.disabledFor(resetRevision, commandTarget{source: "selected"}), "protected") || !strings.Contains(a.commitDisabled(), "protected") {
		t.Fatal("default main protection was weakened")
	}
	a.run(resetRevision, commandTarget{source: "selected"})
	a.run(commitRepository, commandTarget{})
	if a.repositoryOperation != nil || a.commitConfirmation != nil {
		t.Fatal("protected rewrite confirmation opened")
	}
}

func TestProtectedResetToHeadStillRequiresHardAcknowledgment(t *testing.T) {
	a, tt := advancedWorkspace(t)
	a.Model.Active().Repository.Conflicts = []domain.MergeConflict{{Path: "conflict.txt"}}
	calls := 0
	a.OperateRepository = func(_ context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		if op.Kind != domain.ResetRevision || op.Source != repo.Head || op.Mode != "Hard" {
			t.Fatalf("wrong recovery operation: %+v", op)
		}
		calls++
		done(repo, nil, nil)
	}
	a.run(resetRevision, commandTarget{source: "head"})
	tt.Frame()
	if a.repositoryOperation == nil {
		t.Fatal("protected HEAD recovery is unavailable")
	}
	if err := tt.Click("Reset mode"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyDown)
	tt.Key(0, native.KeyEnter)
	if access, ok := tt.Access("Run Reset"); !ok || !access.Disabled {
		t.Fatal("Hard recovery did not require acknowledgment")
	}
	if err := tt.Click("I understand Hard reset discards tracked changes"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Run Reset"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("confirmed HEAD recovery did not dispatch exactly once")
	}
}

func TestResetProtectsUnsavedSourcesBeforeOpenAndDispatch(t *testing.T) {
	a, _ := advancedWorkspace(t)
	calls := 0
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		calls++
	}
	a.openCode("project", domain.File{Path: "source.go", New: "original\n"})
	buffer := a.repo().tabs[a.repo().active].code.Buffer
	buffer.Set("unsaved\n")
	target := commandTarget{source: "head"}
	if !strings.Contains(a.disabledFor(resetRevision, target), "Save or discard") {
		t.Fatal("reset allowed an unsaved source document")
	}
	a.run(resetRevision, target)
	if a.repositoryOperation != nil {
		t.Fatal("reset confirmation opened with unsaved source")
	}
	buffer.Reset("original\n")
	a.run(resetRevision, target)
	if a.repositoryOperation == nil {
		t.Fatal("clean source blocked reset confirmation")
	}
	buffer.Set("new unsaved edit\n")
	a.applyRepositoryOperation(a.repositoryOperation)
	if calls != 0 || !strings.Contains(a.repositoryOperation.message, "Save or discard") || buffer.String() != "new unsaved edit\n" {
		t.Fatal("reset failed to recheck unsaved sources before dispatch")
	}
}
