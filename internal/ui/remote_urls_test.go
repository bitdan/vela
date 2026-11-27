package ui

import (
	"context"
	"slices"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRemoteURLListsNativeEditor(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.IndexVersion = "index"
			first, second, push := "https://example.test/one.git", "https://example.test/two.git", "https://example.test/push.git"
			r.Remotes = []domain.Remote{{Name: "origin", URL: first, FetchURLs: []string{first, second}, PushURLs: []string{push}}}
			a.Model.Switch(a.installRepository(r))
			var op domain.RepositoryOperation
			a.OperateRepository = func(_ context.Context, _ domain.Repository, v domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				op = v
				done(r, nil, nil)
			}
			a.openRemotes()
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			click("origin")
			click("Edit remote URLs")
			if a.remotes.editor == nil || a.remotes.editor.fetch != first+"\n"+second || a.remotes.editor.push != push {
				t.Fatal("wrong initial list order")
			}
			click("Fetch URL list")
			tt.Key(native.Ctrl, native.KeyA)
			tt.Type(second)
			tt.Key(0, native.KeyEnter)
			tt.Type(first)
			exportSessionScreenshot(t, tt, "remote-urls-"+theme)
			click("Save remote URLs")
			if op.Kind != 0 {
				t.Fatal("URLs saved before confirmation")
			}
			click("Run Edit Remote URLs")
			if op.Kind != domain.EditRemoteURLs || !slices.Equal(op.FetchURLs, []string{second, first}) || !slices.Equal(op.PushURLs, []string{push}) {
				t.Fatalf("unexpected URL edit: %+v", op)
			}
		})
	}
}
