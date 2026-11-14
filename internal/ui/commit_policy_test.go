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
