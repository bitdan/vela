package ui

import (
	"context"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRealRemoteManagementConfirmation(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			repo := sessionRepository(t.TempDir())
			repo.IndexVersion = "index"
			repo.Remotes = []domain.Remote{{Name: "origin", URL: "https://example.test/original.git"}}
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			a.Model.Switch(a.installRepository(repo))
			var request domain.RepositoryOperation
			var finish func(domain.Repository, error, error)
			var requestContext context.Context
			a.OperateRepository = func(ctx context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				requestContext, request, finish = ctx, op, done
			}
			a.openRemotes()
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			fill := func(label, value string) { t.Helper(); click(label); tt.Key(native.Ctrl, native.KeyA); tt.Type(value) }
			click("Add remote")
			fill("Remote name", "upstream")
			fill("Remote URL", "https://example.test/new.git")
			click("Save remote")
			if finish != nil || a.repositoryOperation == nil || len(a.Model.Active().Repository.Remotes) != 1 {
				t.Fatal("remote save must require confirmation before writing")
			}
			exportSessionScreenshot(t, tt, "remote-add-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Cancel")
			tt.Frame()
			if finish != nil {
				t.Fatal("cancel wrote the remote")
			}
			a.openRemotes()
			tt.Frame()
			click("origin")
			click("Edit remote")
			fill("Remote URL", "https://example.test/edited.git")
			click("Save remote")
			click("Run Edit Remote")
			if request.Kind != domain.EditRemote || request.Remote != "origin" || request.Target != "https://example.test/edited.git" {
				t.Fatalf("wrong edit: %+v", request)
			}
			click("Cancel")
			if requestContext.Err() != context.Canceled {
				t.Fatal("cancel did not cancel request")
			}
			finish(repo, context.Canceled, nil)
			tt.Frame()
			click("Cancel")
			tt.Frame()
			a.openRemotes()
			tt.Frame()
			click("origin")
			click("Remove remote")
			if !tt.HasText("Remove Remote") {
				t.Fatal("remove did not open confirmation")
			}
			click("Run Remove Remote")
			if request.Kind != domain.RemoveRemote || request.Remote != "origin" {
				t.Fatalf("wrong remove: %+v", request)
			}
			a.run(selectRepo, commandTarget{index: 1})
			finish(repo, nil, nil)
			tt.Frame()
			if a.Model.Active().Repository.Name != "sandbox" {
				t.Fatal("late remote completion switched repositories")
			}
		})
	}
}
