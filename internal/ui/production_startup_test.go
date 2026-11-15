package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestProductionWelcomeAndExplicitDemoIsolation(t *testing.T) {
	a := New(application.NewWorkspace(), application.DefaultLayout())
	a.ReadRepository = func(context.Context, string, func(domain.Repository, error)) {}
	a.PickRepositoryDirectory = func(func(string, error)) {}
	a.CloneRepository = func(context.Context, string, string, func(string, error)) {}
	a.InitializeRepository = func(context.Context, string, string, func(string, error)) {}
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Open Demo Workspace"); err != nil {
		t.Fatal(err)
	}
	if a.Model.Active().Repository.Name != "vela" || !a.Model.Production {
		t.Fatal("explicit demo did not open")
	}
	if saved := a.SessionSnapshot(); len(saved.Repositories) != 0 || saved.ActiveRepository != "" {
		t.Fatal("demo leaked into production session JSON")
	}
	a.run(leaveDemo, commandTarget{})
	if !a.welcomeActive() || a.disabledFor(stageChanges, commandTarget{}) == "" {
		t.Fatal("welcome permits simulated writes")
	}
	r := sessionRepository(t.TempDir())
	r.IndexVersion = "index"
	a.Model.Switch(a.installRepository(r))
	a.Model.Active().Message = "real draft"
	a.run(openDemo, commandTarget{})
	a.run(leaveDemo, commandTarget{})
	if a.Model.Active().Repository.Root != r.Root || a.Model.Active().Message != "real draft" {
		t.Fatal("demo lost real session or draft")
	}
}

func TestProductionRestoreFailureAndDemoCancelNeverFallbackToMock(t *testing.T) {
	a := New(application.NewWorkspace(), application.DefaultLayout())
	root := t.TempDir()
	a.RestoreSession(application.WorkspaceSnapshot{ActiveRoot: root, ActiveRepository: "vela", Repositories: []application.RepositorySnapshot{{Root: root, Name: "missing"}, {Name: "vela"}}})
	var complete func(domain.Repository, error)
	var queryContext context.Context
	a.ReadRepository = func(ctx context.Context, _ string, done func(domain.Repository, error)) {
		complete = done
		queryContext = ctx
	}
	a.StartSessionRestore()
	complete(domain.Repository{}, errors.New("repository unavailable"))
	if !a.welcomeActive() || !strings.Contains(a.status, "repository unavailable") || a.SessionSnapshot().ActiveRoot != root {
		t.Fatal("failed real restore fell back to mock or lost retry identity")
	}
	a.run(openRecentRepository, commandTarget{source: root})
	late := complete
	a.run(openDemo, commandTarget{})
	if queryContext.Err() == nil {
		t.Fatal("explicit demo did not cancel startup read")
	}
	r := sessionRepository(root)
	late(r, nil)
	if !a.demoActive() {
		t.Fatal("late startup result replaced explicitly selected demo")
	}
	a.run(leaveDemo, commandTarget{})
	if !a.welcomeActive() {
		t.Fatal("missing real root should return to welcome")
	}
}

func TestProductionIgnoresLegacyAutomaticMockRestore(t *testing.T) {
	a := New(application.NewWorkspace(), application.DefaultLayout())
	a.RestoreSession(application.WorkspaceSnapshot{ActiveRepository: "vela", Repositories: []application.RepositorySnapshot{{Name: "vela", Message: "demo"}, {Name: "real", Root: "F:/missing-real"}}, ActiveRoot: "F:/missing-real"})
	var path string
	a.ReadRepository = func(_ context.Context, root string, _ func(domain.Repository, error)) { path = root }
	a.StartSessionRestore()
	if path != "F:/missing-real" || !a.welcomeActive() || len(a.Model.Sessions) != 1 {
		t.Fatal("startup restored mock instead of real root")
	}
	if len(a.SessionSnapshot().Repositories) != 1 {
		t.Fatal("real recent identity lost")
	}
}

func TestProductionInitializeOpensOnlyWhenNavigationUnchanged(t *testing.T) {
	for _, mode := range []string{"success", "navigate", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			a := New(application.NewWorkspace(), application.DefaultLayout())
			directory := t.TempDir()
			var complete func(string, error)
			a.InitializeRepository = func(_ context.Context, _ string, _ string, done func(string, error)) { complete = done }
			var opened string
			a.ReadRepository = func(_ context.Context, root string, done func(domain.Repository, error)) {
				opened = root
				done(sessionRepository(root), nil)
			}
			a.run(initializeRepository, commandTarget{})
			s := a.repositoryInit
			s.directory = directory
			s.branch = "main"
			a.initializeRepositoryFromDialog(s)
			if mode == "navigate" {
				r := sessionRepository(t.TempDir())
				a.selectRepository(a.installRepository(r))
			}
			if mode == "cancel" {
				s.cancel()
			}
			complete(directory, nil)
			if mode == "success" {
				if opened != directory || a.Model.Active().Repository.Root != directory {
					t.Fatal("initialized repository was not opened")
				}
			} else if opened != "" {
				t.Fatal("late or cancelled initialization replaced user navigation")
			}
		})
	}
}
