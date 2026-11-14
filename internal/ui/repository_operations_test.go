package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRepositoryOperationDialogs(t *testing.T) {
	head := "71c6e4a33fc596f3686548f1affaac48f8d98dc4"
	repo := domain.Repository{
		Root: t.TempDir(), Name: "sample-repository", Head: head, CurrentBranch: "main", IndexVersion: "index",
		Branches:  []string{"main", "topic", "origin/main"},
		Refs:      map[string]string{"refs/heads/main": head, "refs/heads/topic": head, "refs/remotes/origin/main": head},
		Upstreams: map[string]domain.Upstream{"topic": {Remote: "origin", Branch: "release"}},
		Remotes:   []domain.Remote{{Name: "origin", URL: "https://example.test/project.git"}},
	}
	layout := application.DefaultLayout()
	layout.Dark = false
	a := New(application.New(), layout)
	a.Model.Switch(a.installRepository(repo))
	var request domain.RepositoryOperation
	var pending context.Context
	var finish func(domain.Repository, error, error)
	a.OperateRepository = func(ctx context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		pending, request, finish = ctx, op, done
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	a.run(newBranch, commandTarget{})
	tt.Frame()
	click("Local branch name")
	tt.Type("feature/example")
	click("Checkout after creation")
	click("Run Create Branch")
	if request.Kind != domain.CreateBranch || request.Branch != "feature/example" || !request.Checkout || request.Source != head {
		t.Fatalf("unexpected create branch request: %+v", request)
	}
	finish(repo, nil, nil)
	tt.Frame()

	a.run(updateBranch, commandTarget{source: "topic"})
	tt.Frame()
	click("Run Update Branch")
	if request.Kind != domain.UpdateTrackedBranch || request.Branch != "topic" || request.Remote != "origin" || request.Target != "release" {
		t.Fatalf("Update used the wrong branch or upstream: %+v", request)
	}
	click("Cancel")
	if pending.Err() != context.Canceled {
		t.Fatal("cancel did not reach the operation")
	}
	finish(repo, context.Canceled, nil)
	tt.Frame()
	click("Cancel")
	tt.Frame()

	click("Main menu")
	click("Git menu")
	if err := tt.ChooseMenuItem("Pull…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if out := os.Getenv("VELA_SCREENSHOTS"); out != "" {
		if err := os.MkdirAll(out, 0755); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(out, "phase3-pull.png"))
	}
	click("Run Pull")
	if request.Kind != domain.PullBranch || request.Branch != "main" || request.Target != "main" {
		t.Fatalf("Pull did not use the checked-out branch: %+v", request)
	}
	finish(repo, nil, nil)
	tt.Frame()

	session := a.Model.Active()
	session.Message = "Commit before push"
	session.Repository.Staged = []domain.File{{Path: "demo.txt", Status: "M"}}
	a.CommitRepository = func(ctx context.Context, snapshot domain.Repository, message string, amend bool, done func(domain.Repository, error, error)) {
		updated := snapshot
		updated.Head = strings.Repeat("a", 40)
		updated.Staged = nil
		done(updated, nil, nil)
	}
	a.run(commitAndPush, commandTarget{})
	tt.Frame()
	click("Run Push")
	if request.Kind != domain.PushBranch || request.Branch != "main" || request.Remote != "origin" || request.Target != "main" || !request.SetUpstream {
		t.Fatalf("unexpected push request: %+v", request)
	}
	committed := a.Model.Active().Repository
	finish(committed, errors.New("fixture push rejection"), nil)
	if committed.Head == head || !strings.Contains(a.status, "Commit succeeded.") || !strings.Contains(a.status, "fixture push rejection") {
		t.Fatal("the result did not report the commit and push separately")
	}
}
