package ui

import (
	"context"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRemoteRenameDialog(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			a.Layout.Dark = theme == "dark"
			r := sessionRepository(t.TempDir())
			r.IndexVersion = "index"
			r.Remotes = []domain.Remote{{Name: "origin", URL: "https://example.test/repo.git"}, {Name: "taken", URL: "https://example.test/other.git"}}
			a.Model.Switch(a.installRepository(r))
			var result domain.RepositoryOperation
			a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				result = op
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
			fill := func(value string) { click("Remote name"); tt.Key(native.Ctrl, native.KeyA); tt.Type(value) }
			click("origin")
			click("Rename remote")
			fill("taken")
			if btn, ok := tt.Access("Save remote"); !ok || !btn.Disabled {
				t.Fatal("duplicate name was enabled")
			}
			fill("upstream")
			click("Save remote")
			if result.Kind != 0 || !tt.HasText("Rename Remote") {
				t.Fatal("rename skipped confirmation")
			}
			exportSessionScreenshot(t, tt, "remote-rename-"+theme)
			click("Run Rename Remote")
			if result.Kind != domain.RenameRemote || result.Remote != "origin" || result.Target != "upstream" {
				t.Fatalf("wrong rename: %+v", result)
			}
		})
	}
}
