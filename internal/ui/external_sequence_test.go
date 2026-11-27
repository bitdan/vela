package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestExternalMultiCommitSequenceNativeRecovery(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.IndexVersion, r.Working = "index", nil
			r.Sequencer = domain.SequencerState{Kind: "revert", Remaining: 3, Commit: strings.Repeat("a", 40), Version: "first"}
			a.Model.Switch(a.installRepository(r))
			next := r
			next.Sequencer.Commit, next.Sequencer.Version, next.Sequencer.Remaining = strings.Repeat("b", 40), "second", 2
			next.Conflicts = []domain.MergeConflict{{Path: "second.txt"}}
			calls := 0
			a.OperateRepository = func(_ context.Context, r domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				calls++
				if op.Kind != domain.ContinueSequence || r.Sequencer.Version != "first" {
					t.Fatalf("wrong recovery: %+v", op)
				}
				done(next, errors.New("fixture: next commit conflicted"), nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			a.run(continueSequence, commandTarget{})
			tt.Frame()
			if a.repositoryOperation == nil || !strings.Contains(a.repositoryOperation.detail, "3 commits remaining") || calls != 0 {
				t.Fatal("missing external sequence confirmation")
			}
			exportSessionScreenshot(t, tt, "sequence-recovery-"+theme)
			if err := tt.Click("Run Continue Revert"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if calls != 1 || !tt.HasText("Revert in progress · 2 commits remaining · 1 file unresolved") {
				t.Fatalf("remaining sequence not updated: %+v", a.Model.Active().Repository.Sequencer)
			}
			if a.disabledFor(continueSequence, commandTarget{}) == "" || a.disabledFor(skipSequence, commandTarget{}) != "" {
				t.Fatal("next conflict presents invalid recovery actions")
			}
		})
	}
}
