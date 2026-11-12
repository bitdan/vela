package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestMergeConflictControls(t *testing.T) {
	head, incoming := strings.Repeat("a", 40), strings.Repeat("b", 40)
	repo := domain.Repository{Root: t.TempDir(), Name: "sample-repository", CurrentBranch: "main", Head: head, IndexVersion: "index",
		Branches: []string{"main", "topic"}, Refs: map[string]string{"refs/heads/main": head, "refs/heads/topic": incoming}}
	conflicted := repo
	conflicted.MergeHeads = []string{incoming}
	conflicted.Conflicts = []domain.MergeConflict{{Path: "merge.txt", Current: head, Incoming: incoming}}
	conflicted.Working = []domain.File{{Path: "merge.txt", Status: "U"}}
	resolved := conflicted
	resolved.Conflicts, resolved.Working = nil, nil
	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	a.Model.Switch(a.installRepository(repo))
	a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		switch op.Kind {
		case domain.StartMerge:
			if op.Source != "refs/heads/topic" {
				t.Fatalf("incorrect merge source: %+v", op)
			}
			done(conflicted, errors.New("fixture conflict"), nil)
		case domain.AcceptIncoming:
			if op.Source != "merge.txt" {
				t.Fatalf("incorrect resolution target: %+v", op)
			}
			done(resolved, nil, nil)
		case domain.ContinueMerge:
			done(repo, nil, nil)
		default:
			t.Fatalf("unexpected operation: %+v", op)
		}
	}
	a.ReadRepositoryFile = func(_ context.Context, _, source string, file domain.File, done func(domain.File, error)) {
		if source != "merge-conflict" {
			t.Fatalf("incorrect preview source: %s", source)
		}
		file.Old, file.New = "current\n", "incoming\n"
		done(file, nil)
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	a.run(mergeBranch, commandTarget{source: "topic"})
	tt.Frame()
	click("Run Merge")
	if a.repositoryOperation != nil || !strings.Contains(a.status, "Merge paused") {
		t.Fatal("merge conflict did not return to the workspace")
	}
	if button, ok := tt.Access("Continue Merge"); !ok || !button.Disabled {
		t.Fatal("Continue Merge should be disabled with unresolved files")
	}
	click("merge.txt")
	if !tt.HasText("Incoming") || a.repo().preview == nil {
		t.Fatal("clicking a conflict did not open the incoming/current diff")
	}
	if out := os.Getenv("VELA_SCREENSHOTS"); out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(out, "phase4-merge-conflict.png"))
	}
	click("Accept Incoming")
	click("Run Accept Incoming")
	click("Continue Merge")
	click("Run Continue Merge")
	if len(a.Model.Active().Repository.MergeHeads) != 0 {
		t.Fatal("continue did not refresh the merge state")
	}
}
