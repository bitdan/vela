package ui

import (
	"context"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestCommitAndIndexCanBeCanceled(t *testing.T) {
	for _, commit := range []bool{false, true} {
		t.Run(map[bool]string{false: "index", true: "commit"}[commit], func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			repo := sessionRepository(t.TempDir())
			repo.IndexVersion = "index"
			repo.Staged = []domain.File{{Path: "staged.txt", Status: "A"}}
			a.Model.Switch(a.installRepository(repo))
			a.Model.Active().Message = "keep draft"
			var request context.Context
			var finish func(domain.Repository, error, error)
			a.CommitRepository = func(ctx context.Context, r domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
				request, finish = ctx, done
			}
			a.UpdateRepositoryIndex = func(ctx context.Context, r domain.Repository, stage bool, files []domain.File, done func(domain.Repository, error, error)) {
				request, finish = ctx, done
			}
			if commit {
				a.run(commitRepository, commandTarget{})
			} else {
				a.run(stageChanges, commandTarget{source: "working", file: repo.Working[0]})
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Cancel Git Task"); err != nil {
				t.Fatal(err)
			}
			if request.Err() != context.Canceled || a.repositoryWrite == "" {
				t.Fatal("cancellation did not reach worker or released busy state before refresh")
			}
			if a.RequestClose() {
				t.Fatal("window closed before canceled worker finished")
			}
			a.run(selectRepo, commandTarget{index: 1})
			finish(repo, context.Canceled, nil)
			tt.Frame()
			if a.repositoryWrite != "" || a.Model.Active().Repository.Name != "sandbox" || a.Model.Sessions[2].Message != "keep draft" {
				t.Fatal("cancellation lost draft or polluted another session")
			}
			if !strings.Contains(a.repos[2].console[len(a.repos[2].console)-1], "did not confirm") {
				t.Fatal("canceled write reported definite failure instead of unknown completion")
			}
		})
	}
}
