package ui

import (
	"context"
	native "github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRewordNativeMessageConfirmation(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.CurrentBranch = "feature"
			r.Head = strings.Repeat("a", 40)
			r.Working = nil
			r.History = []domain.Commit{{Hash: r.Head, Subject: "old message"}}
			a.Model.Switch(a.installRepository(r))
			a.PreviewCommitRewrite = func(_ context.Context, _ domain.Repository, k domain.RepositoryOperationKind, ids []string, done func(domain.CommitRewritePreview, error)) {
				done(domain.CommitRewritePreview{Version: "v1", Selected: ids, Message: "old message", Author: "Original Author"}, nil)
			}
			var op domain.RepositoryOperation
			var done func(domain.Repository, error, error)
			a.OperateRepository = func(_ context.Context, _ domain.Repository, input domain.RepositoryOperation, finish func(domain.Repository, error, error)) {
				op = input
				done = finish
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.RightClick("Log commit " + r.Head); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Edit Commit Message…"); err != nil {
				t.Fatal(err)
			}
			if a.squash == nil {
				t.Fatal("missing native rewrite dialog")
			}
			_ = tt.Click("Run History Rewrite")
			if done != nil {
				t.Fatal("unconfirmed write")
			}
			if err := tt.Click("History rewrite message"); err != nil {
				t.Fatal(err)
			}
			tt.Key(native.Ctrl, native.KeyA)
			tt.Type("changed 中文")
			if err := tt.Click("Rewrite the selected range and its later commits"); err != nil {
				t.Fatal(err)
			}
			exportSessionScreenshot(t, tt, "reword-"+theme)
			if err := tt.Click("Run History Rewrite"); err != nil {
				t.Fatal(err)
			}
			if done == nil || op.Kind != domain.RewordCommit || op.Message != "changed 中文" || !op.ConfirmRewrite {
				t.Fatalf("wrong reword request %+v", op)
			}
			done(r, nil, nil)
			if a.squash != nil {
				t.Fatal("completion left dialog")
			}
		})
	}
}

func TestRewordLatePreviewAndMessageDraft(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	r := sessionRepository(t.TempDir())
	r.Head = strings.Repeat("a", 40)
	r.CurrentBranch = "feature"
	r.Working = nil
	r.History = []domain.Commit{{Hash: r.Head}}
	a.Model.Switch(a.installRepository(r))
	var finish func(domain.CommitRewritePreview, error)
	var request context.Context
	a.PreviewCommitRewrite = func(ctx context.Context, _ domain.Repository, _ domain.RepositoryOperationKind, _ []string, done func(domain.CommitRewritePreview, error)) {
		request = ctx
		finish = done
	}
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	a.run(editCommit, commandTarget{source: r.Head})
	if finish == nil {
		t.Fatal("preview missing")
	}
	a.run(selectRepo, commandTarget{index: 0})
	finish(domain.CommitRewritePreview{Version: "late", Selected: []string{r.Head}, Message: "late"}, nil)
	if request.Err() != context.Canceled || a.squash != nil {
		t.Fatal("late preview revived dialog")
	}
}
