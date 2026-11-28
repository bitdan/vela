package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters/gitcli"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestCommitFormAndAmendConfirmation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-repository")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g := gitcli.New()
	ctx := context.Background()
	if _, err := g.Initialize(ctx, dir, "feature/amend"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"config", "user.name", "Vela Test"},
		{"config", "user.email", "vela@example.test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("configure fixture: %s: %v", output, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "sample.txt"), []byte("index fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, err := g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := g.UpdateIndex(ctx, repo, true, repo.Working); err != nil {
		t.Fatal(err)
	}
	repo, err = g.ReadRepository(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	a.Model.Switch(a.installRepository(repo))
	lists := &a.Model.Active().Changelists
	listID, err := lists.Create("Commit policy")
	if err != nil {
		t.Fatal(err)
	}
	if err := lists.Move([]string{repo.Staged[0].Path}, listID); err != nil {
		t.Fatal(err)
	}
	policy := lists.Policy()
	policy.EmptyInactive = "Keep"
	lists.SetPolicy(policy)
	beforeFailure := application.CloneChangelists(*lists)
	beforeFailure.Normalize(repo)
	failNext := true
	a.CommitRepository = func(ctx context.Context, repo domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
		if failNext {
			failNext = false
			done(repo, errors.New("fixture commit error"), nil)
			return
		}
		err := g.Commit(ctx, repo, message, amend)
		updated, refreshErr := g.ReadRepository(ctx, repo.Root)
		done(updated, err, refreshErr)
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	if button, ok := tt.Access("Create Commit"); !ok || !button.Disabled {
		t.Fatal("commit should be disabled before entering a message")
	}
	click("Commit message")
	tt.Type("Initial commit")
	click("Create Commit")
	if s := a.Model.Active(); s.Message != "Initial commit" || s.Repository.Head != "" || !strings.Contains(a.status, "fixture commit error") {
		t.Fatal("failure lost the draft or failed to report the error")
	}
	if !reflect.DeepEqual(a.Model.Active().Changelists, beforeFailure) {
		t.Fatal("failed real commit changed changelists")
	}
	click("Create Commit")
	s := a.Model.Active()
	if s.Repository.Head == "" || s.Message != "" || len(s.Repository.Staged) != 0 {
		t.Fatal("successful commit did not refresh the index and clear the draft")
	}
	if len(s.Changelists.Lists) != 2 || len(s.Changelists.Lists[1].Paths) != 0 {
		t.Fatalf("Keep policy removed an empty list: %+v", s.Changelists)
	}
	first := s.Repository.Head
	click("Amend last commit")
	if s := a.Model.Active(); !s.Amend || strings.TrimSpace(s.Message) != "Initial commit" {
		t.Fatal("amend did not load the current commit message")
	}
	click("Commit message")
	tt.Key(native.Cmd, native.KeyA)
	tt.Type("Reword initial commit")
	click("Amend Commit")
	if a.commitConfirmation == nil {
		t.Fatal("amend did not ask for confirmation")
	}
	if out := os.Getenv("VELA_SCREENSHOTS"); out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(out, "phase3-amend-confirmation.png"))
	}
	click("Cancel")
	if a.commitConfirmation != nil || a.Model.Active().Repository.Head != first || a.Model.Active().Message != "Reword initial commit" {
		t.Fatal("cancel changed HEAD or lost the amend draft")
	}
	click("Amend Commit")
	click("Replace Commit")
	s = a.Model.Active()
	if s.Repository.Head == first || len(s.Repository.History) != 1 ||
		s.Repository.History[0].Subject != "Reword initial commit" || s.Amend || s.Message != "" {
		t.Fatal("amend did not replace the previous commit and refresh the workspace")
	}
}
