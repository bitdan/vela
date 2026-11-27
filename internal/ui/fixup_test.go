package ui

import (
	"context"
	native "github.com/egoist/mygo/ui"
	"strings"
	"testing"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestFixupNativeReadonlyMessage(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.CurrentBranch = "feature"
			r.Working = nil
			r.Head = strings.Repeat("a", 40)
			other := strings.Repeat("b", 40)
			r.History = []domain.Commit{{Hash: r.Head, Subject: "fixup"}, {Hash: other, Subject: "original"}}
			a.Model.Switch(a.installRepository(r))
			a.PreviewCommitRewrite = func(_ context.Context, _ domain.Repository, k domain.RepositoryOperationKind, ids []string, done func(domain.CommitRewritePreview, error)) {
				done(domain.CommitRewritePreview{Version: "fixup:v1", Selected: ids, Message: "Original 中文\n\nbody\n", Author: "Author"}, nil)
			}
			var op domain.RepositoryOperation
			a.OperateRepository = func(_ context.Context, _ domain.Repository, input domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				op = input
				done(r, nil, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			_ = tt.Click("Log commit " + r.Head)
			_ = tt.ClickWith(native.Ctrl, "Log commit "+other)
			a.run(fixupCommit, commandTarget{source: r.Head})
			tt.Frame()
			if a.squash == nil {
				t.Fatal("Fixup dialog missing")
			}
			if field, ok := tt.Access("History rewrite message"); !ok || !field.Disabled {
				t.Fatal("Fixup message must be read-only")
			}
			_ = tt.Click("Rewrite the selected range and its later commits")
			exportSessionScreenshot(t, tt, "fixup-"+theme)
			if err := tt.Click("Run History Rewrite"); err != nil {
				t.Fatal(err)
			}
			if op.Kind != domain.FixupCommits || !op.ConfirmRewrite || len(op.RewriteSelection) != 2 {
				t.Fatal("wrong Fixup request")
			}
		})
	}
}
