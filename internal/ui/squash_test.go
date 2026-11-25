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

func TestSquashDialogConfirmationAndEditorProtection(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.CurrentBranch = "feature"
			r.Head = strings.Repeat("a", 40)
			r.IndexVersion = "index"
			r.WorktreeVersion = "work"
			r.Working = nil
			r.Staged = nil
			ids := []string{r.Head, strings.Repeat("b", 40), strings.Repeat("c", 40)}
			r.History = []domain.Commit{{Hash: ids[0], Subject: "second", ParentHashes: ids[1:2]}, {Hash: ids[1], Subject: "first", ParentHashes: ids[2:3]}, {Hash: ids[2], Subject: "base"}}
			a.Model.Switch(a.installRepository(r))
			a.SaveRepositoryFile = func(context.Context, domain.Repository, domain.File, string, func(domain.Repository, domain.File, error, error)) {
			}
			a.openCode("project", domain.File{Path: "file.txt", New: "clean", DiskVersion: "v1"})
			a.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) {
				f.New = "clean"
				f.DiskVersion = "refreshed"
				done(f, nil)
			}
			buffer := a.activeDocument().code
			a.PreviewRepositorySquash = func(_ context.Context, _ domain.Repository, chosen []string, done func(domain.SquashPreview, error)) {
				done(domain.SquashPreview{Version: "preview", Selected: chosen, Author: "Original Author", Message: "first\n\nsecond"}, nil)
			}
			var op domain.RepositoryOperation
			var finish func(domain.Repository, error, error)
			var request context.Context
			a.OperateRepository = func(ctx context.Context, _ domain.Repository, input domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				op, finish, request = input, done, ctx
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Log commit " + ids[0]); err != nil {
				t.Fatal(err)
			}
			if err := tt.ClickWith(native.Ctrl, "Log commit "+ids[1]); err != nil {
				t.Fatal(err)
			}
			if a.repo().logChosen.Len() != 2 {
				t.Fatal("native Log multi-selection did not retain both commits")
			}
			a.run(squashCommit, commandTarget{source: ids[0]})
			tt.Frame()
			_ = tt.Click("Run Squash Commits")
			if finish != nil {
				t.Fatal("unconfirmed history rewrite")
			}
			if err := tt.Click("Squash commit message"); err != nil {
				t.Fatal(err)
			}
			tt.Key(native.Ctrl, native.KeyA)
			tt.Type("combined 中文")
			if err := tt.Click("Rewrite the selected range and its later commits"); err != nil {
				t.Fatal(err)
			}
			exportSessionScreenshot(t, tt, "squash-"+theme)
			if reason := a.disabledFor(confirmSquash, commandTarget{}); reason != "" {
				t.Fatalf("confirmation remains disabled: %s; dialog=%+v", reason, a.squash)
			}
			if err := tt.Click("Run Squash Commits"); err != nil {
				t.Fatal(err)
			}
			if finish == nil || !op.ConfirmSquash || op.SquashVersion != "preview" || len(op.SquashSelection) != 2 || op.Message != "combined 中文" || !buffer.ReadOnly {
				t.Fatalf("wrong squash request/editor protection: %+v", op)
			}
			if err := tt.Click("Cancel Squash"); err != nil {
				t.Fatal(err)
			}
			if request.Err() != context.Canceled {
				t.Fatal("cancel did not signal native Git")
			}
			finish(r, errors.New("canceled"), nil)
			if buffer.ReadOnly || a.Model.Active().Repository.Root != r.Root {
				t.Fatal("completion did not restore editor/repository identity")
			}
			if a.activeDocument().file.DiskVersion != "refreshed" {
				t.Fatal("Squash completion left a stale editor disk version")
			}
		})
	}
}

func TestSquashPreviewNavigationIgnoresLateResult(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	r := sessionRepository(t.TempDir())
	r.CurrentBranch = "feature"
	r.Head = strings.Repeat("a", 40)
	r.IndexVersion = "index"
	r.Working = nil
	r.Staged = nil
	a.Model.Switch(a.installRepository(r))
	a.repo().logChosen.Add(r.Head)
	a.repo().logChosen.Add(strings.Repeat("b", 40))
	r.History = []domain.Commit{{Hash: r.Head}, {Hash: strings.Repeat("b", 40)}}
	a.Model.Active().Repository = r
	var request context.Context
	var finish func(domain.SquashPreview, error)
	a.PreviewRepositorySquash = func(ctx context.Context, _ domain.Repository, _ []string, done func(domain.SquashPreview, error)) {
		request, finish = ctx, done
	}
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	a.run(squashCommit, commandTarget{})
	if finish == nil {
		t.Fatal("preview not started")
	}
	a.run(selectRepo, commandTarget{index: 1})
	finish(domain.SquashPreview{Version: "late"}, nil)
	if request.Err() != context.Canceled || a.squash != nil {
		t.Fatal("late squash preview survived navigation")
	}
}
