package ui

import (
	"context"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestUndoCommitNativeConfirmation(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.CurrentBranch, r.Head, r.Working = "feature", strings.Repeat("a", 40), nil
			parent := strings.Repeat("b", 40)
			r.History = []domain.Commit{{Hash: r.Head, Subject: "latest"}, {Hash: parent, Subject: "parent"}}
			a.Model.Switch(a.installRepository(r))
			a.PreviewCommitRewrite = func(_ context.Context, _ domain.Repository, kind domain.RepositoryOperationKind, ids []string, done func(domain.CommitRewritePreview, error)) {
				if kind != domain.UndoCommit || len(ids) != 1 || ids[0] != r.Head {
					t.Fatalf("unexpected preview: %v %v", kind, ids)
				}
				done(domain.CommitRewritePreview{Version: "undo:v1", Parent: parent, Selected: ids}, nil)
			}
			var operation domain.RepositoryOperation
			a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				operation = op
				done(r, nil, nil)
			}
			if a.disabledFor(undoCommit, commandTarget{source: parent}) == "" {
				t.Fatal("prior commit enabled")
			}
			tt := native.NewTester(a.View, 1440, 900)
			_ = tt.RightClick("Log commit " + r.Head)
			if err := tt.ChooseMenuItem("Undo Commit…"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("Move the branch to " + parent) {
				t.Fatal("missing Undo impact")
			}
			_ = tt.Click("Run History Rewrite")
			if operation.Kind != 0 {
				t.Fatal("ran without confirmation")
			}
			_ = tt.Click("Undo the latest commit and keep its changes staged")
			exportSessionScreenshot(t, tt, "undo-commit-"+theme)
			if err := tt.Click("Run History Rewrite"); err != nil {
				t.Fatal(err)
			}
			if operation.Kind != domain.UndoCommit || !operation.ConfirmRewrite || operation.RewriteVersion != "undo:v1" {
				t.Fatalf("wrong operation: %+v", operation)
			}
		})
	}
}
