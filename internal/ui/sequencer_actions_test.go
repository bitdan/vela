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

func TestSequencerControlsAndConflictEditor(t *testing.T) {
	for _, kind := range []string{"cherry-pick", "revert"} {
		t.Run(kind, func(t *testing.T) {
			repo := domain.Repository{Root: t.TempDir(), Name: "sample", CurrentBranch: "main", Head: "head",
				IndexVersion: "index", History: []domain.Commit{{Hash: "selected", Subject: "Selected change"}}}
			paused := repo
			paused.Sequencer = domain.SequencerState{Kind: kind, Commit: "selected", Version: "paused"}
			conflict := domain.MergeConflict{Path: "conflict.txt", Current: "current", Incoming: "incoming"}
			paused.Conflicts, paused.Working = []domain.MergeConflict{conflict}, []domain.File{{Path: conflict.Path, Status: "U"}}
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = false
			a.Model.Switch(a.installRepository(repo))
			a.ReadRepositoryFile = func(context.Context, string, string, domain.File, func(domain.File, error)) {
				t.Fatal("conflict did not open the three-way editor")
			}
			a.ReadMergeDocument = func(_ context.Context, snapshot domain.Repository, _ string, done func(domain.MergeDocument, error)) {
				done(domain.MergeDocument{Conflict: conflict, Head: snapshot.Head, Sequencer: snapshot.Sequencer,
					MarkerSize: 7, Current: "current\n", Incoming: "incoming\n",
					Result: "<<<<<<< HEAD\ncurrent\n=======\nincoming\n>>>>>>> selected\n"}, nil)
			}
			var operations []domain.RepositoryOperationKind
			a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				operations = append(operations, op.Kind)
				switch op.Kind {
				case domain.StartCherryPick, domain.StartRevert:
					if op.Source != "selected" {
						t.Fatalf("wrong selected commit: %+v", op)
					}
					done(paused, errors.New("fixture conflict"), nil)
				case domain.SaveMergeResult:
					if op.Result != "incoming\n" {
						t.Fatalf("wrong result: %+v", op)
					}
					snapshot.Conflicts, snapshot.Working = nil, nil
					done(snapshot, nil, nil)
				case domain.ContinueSequence:
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
			title, side := "Cherry-Pick", "Picked Commit"
			if kind == "cherry-pick" {
				// The toolbar must invoke the same operation as the context menu.
				a.repo().logQuery = "no matching commit"
				tt.Frame()
				tt.Frame()
				if button, ok := tt.Access("Cherry-Pick"); !ok || !button.Disabled {
					t.Fatal("toolbar accepted a commit hidden by the filter")
				}
				a.repo().logQuery = ""
				tt.Frame()
				tt.Frame()
				click("Cherry-Pick")
			} else {
				title, side = "Revert", "Reverted Parent"
				if err := tt.RightClick("Selected change"); err != nil {
					t.Fatal(err)
				}
				if err := tt.ChooseMenuItem("Revert Commit"); err != nil {
					t.Fatal(err)
				}
			}
			click("Cancel")
			if len(operations) != 0 {
				t.Fatal("cancelling confirmation invoked Git")
			}
			id := cherryPick
			if kind == "revert" {
				id = revertCommit
			}
			a.run(id, commandTarget{source: "selected"})
			tt.Frame()
			click("Run " + title)
			if button, ok := tt.Access("Continue " + title); !ok || !button.Disabled {
				t.Fatal("continue was available before resolving")
			}
			click("Abort " + title)
			click("Cancel")
			click(conflict.Path)
			if !tt.HasText(side) {
				t.Fatal("conflict side is not identified")
			}
			if out := os.Getenv("VELA_SCREENSHOTS"); out != "" {
				if err := os.MkdirAll(out, 0755); err != nil {
					t.Fatal(err)
				}
				writeScreenshot(t, tt, filepath.Join(out, kind+"-light.png"))
				a.Layout.Dark = true
				tt.Frame()
				writeScreenshot(t, tt, filepath.Join(out, kind+"-dark.png"))
			}
			click("Apply " + side + " Block")
			sequence := a.Model.Active().Repository.Sequencer
			a.Model.Active().Repository.Sequencer.Version = "another-step"
			tt.Frame()
			if button, ok := tt.Access("Apply Changes"); !ok || !button.Disabled {
				t.Fatal("a draft from another sequencer step could be applied")
			}
			a.Model.Active().Repository.Sequencer = sequence
			tt.Frame()
			click("Apply Changes")
			click("Continue " + title)
			click("Run Continue " + title)
			if a.Model.Active().Repository.Sequencer.Active() {
				t.Fatal("completed operation did not clear the paused state")
			}
		})
	}
}

func TestMergeCommitRequiresExplicitMainline(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := domain.Repository{Root: t.TempDir(), Name: "sample", CurrentBranch: "main", Head: "head", IndexVersion: "index",
		History: []domain.Commit{{Hash: "merge", Subject: "Merge", Merge: true, ParentHashes: []string{"first", "second"}}}}
	a.Model.Switch(a.installRepository(repo))
	var mainline int
	a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		mainline = op.Mainline
		done(repo, nil, nil)
	}
	tt := native.NewTester(a.View, 1440, 900)
	a.run(cherryPick, commandTarget{source: "merge"})
	tt.Frame()
	if button, ok := tt.Access("Run Cherry-Pick"); !ok || !button.Disabled {
		t.Fatal("merge commit accepted without a mainline parent")
	}
	if err := tt.Click("Choose mainline parent"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyDown)
	tt.Key(0, native.KeyDown)
	tt.Key(0, native.KeyEnter)
	if err := tt.Click("Run Cherry-Pick"); err != nil {
		t.Fatal(err)
	}
	if mainline != 2 {
		t.Fatalf("wrong mainline parent: %d", mainline)
	}
}

func TestSequencerUntrackedFileAndUnchangedRevert(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := domain.Repository{Root: t.TempDir(), Name: "sample", CurrentBranch: "main", Head: "head", IndexVersion: "index",
		Working: []domain.File{{Path: "notes.txt", Status: "?"}}, History: []domain.Commit{{Hash: "commit", Subject: "Change"}}}
	a.Model.Switch(a.installRepository(repo))
	a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		done(repo, domain.ErrRevertUnchanged, nil)
	}
	tt := native.NewTester(a.View, 1440, 900)
	for _, id := range []commandID{cherryPick, revertCommit} {
		if reason := a.disabledFor(id, commandTarget{source: "commit"}); reason != "" {
			t.Fatalf("unrelated untracked file blocked %s: %s", id, reason)
		}
	}
	a.run(revertCommit, commandTarget{source: "commit"})
	tt.Frame()
	if err := tt.Click("Run Revert"); err != nil {
		t.Fatal(err)
	}
	if a.repositoryOperation != nil || a.Model.Active().Repository.Sequencer.Active() || a.status != domain.ErrRevertUnchanged.Error() {
		t.Fatalf("unchanged revert was not reported: %s", a.status)
	}
}
