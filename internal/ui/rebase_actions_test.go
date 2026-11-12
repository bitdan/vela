package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRebaseControlsAndConflictEditor(t *testing.T) {
	repo := domain.Repository{Root: t.TempDir(), Name: "sample-repository", CurrentBranch: "topic", Head: "topic-tip",
		IndexVersion: "index", Branches: []string{"main", "topic"}, Refs: map[string]string{"refs/heads/main": "main-tip", "refs/heads/topic": "topic-tip"}}
	paused := repo
	paused.CurrentBranch, paused.Head = "Detached HEAD", "main-tip"
	paused.Rebase = domain.RebaseState{Branch: "topic", Original: "topic-tip", Onto: "main-tip", Stopped: "topic-tip", Step: "1"}
	conflict := domain.MergeConflict{Path: "conflict.txt", Current: "base", Incoming: "topic"}
	paused.Conflicts = []domain.MergeConflict{conflict}
	paused.Working = []domain.File{{Path: conflict.Path, Status: "U"}}
	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	a.Model.Switch(a.installRepository(repo))
	a.ReadRepositoryFile = func(context.Context, string, string, domain.File, func(domain.File, error)) {
		t.Fatal("rebase did not open the three-way editor")
	}
	a.ReadMergeDocument = func(_ context.Context, snapshot domain.Repository, _ string, done func(domain.MergeDocument, error)) {
		done(domain.MergeDocument{Conflict: conflict, Head: snapshot.Head, Rebase: snapshot.Rebase, MarkerSize: 7,
			Current: "base\n", Incoming: "topic\n", Result: "<<<<<<< HEAD\nbase\n=======\ntopic\n>>>>>>> topic\n"}, nil)
	}
	a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		switch op.Kind {
		case domain.StartRebase:
			if op.Source != "refs/heads/main" {
				t.Fatalf("wrong onto target: %+v", op)
			}
			done(paused, errors.New("fixture conflict"), nil)
		case domain.SaveMergeResult:
			if op.Result != "topic\n" {
				t.Fatalf("replayed commit was not applied: %+v", op)
			}
			snapshot.Conflicts, snapshot.Working = nil, nil
			done(snapshot, nil, nil)
		case domain.ContinueRebase:
			done(repo, nil, nil)
		default:
			t.Fatalf("unexpected operation: %+v", op)
		}
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	a.run(rebaseBranch, commandTarget{source: "main"})
	tt.Frame()
	click("Run Rebase")
	if button, ok := tt.Access("Continue Rebase"); !ok || !button.Disabled {
		t.Fatal("continue was available with unresolved conflicts")
	}
	click("Skip Commit")
	click("Cancel")
	click(conflict.Path)
	if !tt.HasText("Rebased") || !tt.HasText("Replayed Commit") {
		t.Fatal("rebase conflict sides are not identified")
	}
	if out := os.Getenv("VELA_SCREENSHOTS"); out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(out, "rebase-conflict-light.png"))
		a.Layout.Dark = true
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(out, "rebase-conflict-dark.png"))
	}
	click("Apply Replayed Commit Block")
	click("Apply Changes")
	click("Continue Rebase")
	click("Run Continue Rebase")
	if a.Model.Active().Repository.Rebase.Active() {
		t.Fatal("completed rebase did not clear the workspace state")
	}
}
