package ui

import (
	"context"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRefManagementDialogs(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			repo := sessionRepository(t.TempDir())
			repo.IndexVersion = "index"
			repo.Refs = map[string]string{"refs/heads/main": "abc", "refs/heads/topic": "def"}
			repo.Branches = []string{"main", "topic"}
			a := New(application.New(), layout)
			a.Model.Switch(a.installRepository(repo))
			var request domain.RepositoryOperation
			calls := 0
			a.OperateRepository = func(ctx context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				calls++
				request = op
				done(repo, nil, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			a.run(renameBranch, commandTarget{source: "topic"})
			tt.Frame()
			if a.repositoryOperation == nil {
				t.Fatal("rename remains disabled")
			}
			click("Local branch name")
			tt.Key(native.Ctrl, native.KeyA)
			tt.Type("feature/renamed")
			exportSessionScreenshot(t, tt, "rename-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Run Rename Branch")
			if request.Kind != domain.RenameBranch || request.Source != "refs/heads/topic" || request.Branch != "feature/renamed" {
				t.Fatalf("wrong rename %+v", request)
			}
			tt.Frame()
			a.run(deleteBranch, commandTarget{source: "topic"})
			tt.Frame()
			click("Cancel")
			tt.Frame()
			if calls != 1 {
				t.Fatal("cancel deleted branch")
			}
			a.run(newTag, commandTarget{source: "def"})
			tt.Frame()
			click("Tag name")
			tt.Type("v1.0")
			exportSessionScreenshot(t, tt, "tag-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Run Create Tag")
			if request.Kind != domain.CreateTag || request.Source != "def" || request.Target != "v1.0" {
				t.Fatalf("wrong tag %+v", request)
			}
			if a.disabledFor(deleteBranch, commandTarget{source: "main"}) == "" || a.disabledFor(renameBranch, commandTarget{source: "origin/main"}) == "" {
				t.Fatal("unsupported branch operation enabled")
			}
		})
	}
}
