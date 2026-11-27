package ui

import (
	"context"
	native "github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestDropNativeExplicitImpact(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.CurrentBranch = "feature"
			r.Head = strings.Repeat("a", 40)
			r.Working = nil
			r.History = []domain.Commit{{Hash: r.Head, Subject: "drop me"}}
			a.Model.Switch(a.installRepository(r))
			a.PreviewCommitRewrite = func(_ context.Context, _ domain.Repository, k domain.RepositoryOperationKind, ids []string, done func(domain.CommitRewritePreview, error)) {
				done(domain.CommitRewritePreview{Version: "drop:v1", Selected: ids, Parent: strings.Repeat("b", 40), Affected: []domain.File{{Path: "remove 中文.txt", Status: "A"}}}, nil)
			}
			var op domain.RepositoryOperation
			a.OperateRepository = func(_ context.Context, _ domain.Repository, input domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				op = input
				done(r, nil, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			_ = tt.RightClick("Log commit " + r.Head)
			if err := tt.ChooseMenuItem("Drop Commit"); err != nil {
				t.Fatal(err)
			}
			if a.squash == nil || !tt.HasText("remove 中文.txt") {
				t.Fatal("missing impact preview")
			}
			_ = tt.Click("Run History Rewrite")
			if op.Kind != 0 {
				t.Fatal("unconfirmed Drop executed")
			}
			_ = tt.Click("Remove the selected commit and its changes; replay later commits")
			exportSessionScreenshot(t, tt, "drop-"+theme)
			if err := tt.Click("Run History Rewrite"); err != nil {
				t.Fatal(err)
			}
			if op.Kind != domain.DropCommit || !op.ConfirmRewrite {
				t.Fatal("wrong Drop request")
			}
		})
	}
}
