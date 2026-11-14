package ui

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestCloneDialogCancellationAndOpen(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			var finish func(string, error)
			var request context.Context
			target := filepath.Join(t.TempDir(), "new clone")
			a.CloneRepository = func(ctx context.Context, source, destination string, done func(string, error)) {
				if source != "https://example.test/team/repo.git" || destination != target {
					t.Fatal("wrong clone input")
				}
				request, finish = ctx, done
			}
			a.ReadRepository = func(ctx context.Context, root string, done func(domain.Repository, error)) {
				done(sessionRepository(root), nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			fill := func(label, value string) { t.Helper(); click(label); tt.Key(native.Ctrl, native.KeyA); tt.Type(value) }
			click("Main menu")
			click("Git menu")
			if err := tt.ChooseMenuItem("Clone…"); err != nil {
				t.Fatal(err)
			}
			if b, ok := tt.Access("Clone Repository"); !ok || !b.Disabled {
				t.Fatal("empty clone enabled")
			}
			fill("Clone source", "https://example.test/team/repo.git")
			fill("Clone destination", target)
			exportSessionScreenshot(t, tt, "clone-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Clone Repository")
			if finish == nil || a.repositoryWrite == "" {
				t.Fatal("clone not started")
			}
			click("Cancel")
			if request.Err() != context.Canceled {
				t.Fatal("cancel lost")
			}
			finish(target, context.Canceled)
			tt.Frame()
			if a.Model.Active().Repository.Root != "" || a.repositoryWrite != "" {
				t.Fatal("canceled clone switched repo or stayed busy")
			}
			click("Cancel")
			tt.Frame()
			a.run(cloneRepo, commandTarget{})
			tt.Frame()
			fill("Clone source", "https://example.test/team/repo.git")
			fill("Clone destination", target)
			click("Clone Repository")
			finish(target, nil)
			tt.Frame()
			if a.Model.Active().Repository.Root != target || a.repositoryClone != nil {
				t.Fatal("successful clone did not open")
			}
		})
	}
}

func TestCloneFailureAndNavigationDoNotDisplaceWorkspace(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	var finish func(string, error)
	a.CloneRepository = func(ctx context.Context, source, target string, done func(string, error)) { finish = done }
	a.ReadRepository = func(ctx context.Context, root string, done func(domain.Repository, error)) {
		t.Fatal("late clone must not open after navigation")
	}
	a.run(cloneRepo, commandTarget{})
	s := a.repositoryClone
	s.source = "https://example.test/repo.git"
	s.target = filepath.Join(t.TempDir(), "new")
	a.cloneFromDialog(s)
	finish(s.target, errors.New("could not clone; partial directory retained"))
	if s.message == "" || a.Model.Active().Repository.Root != "" {
		t.Fatal("failure not shown")
	}
	a.cloneFromDialog(s)
	a.run(selectRepo, commandTarget{index: 1})
	finish(s.target, nil)
	if a.Model.Active().Repository.Name != "sandbox" {
		t.Fatal("late clone replaced user selection")
	}
}

func TestCloneQueuedSuccessAfterCancelDoesNotAutoOpen(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	var finish func(string, error)
	a.CloneRepository = func(ctx context.Context, source, target string, done func(string, error)) { finish = done }
	a.ReadRepository = func(ctx context.Context, root string, done func(domain.Repository, error)) {
		t.Fatal("canceled clone stole focus after queued success")
	}
	a.run(cloneRepo, commandTarget{})
	s := a.repositoryClone
	s.source = "https://example.test/repo.git"
	s.target = filepath.Join(t.TempDir(), "new")
	a.cloneFromDialog(s)
	s.cancel()
	finish(s.target, nil)
	if a.Model.Active().Repository.Root != "" || a.repositoryWrite != "" {
		t.Fatal("canceled clone displaced workspace")
	}
}
