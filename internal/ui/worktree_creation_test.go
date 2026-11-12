package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

func TestWorktreeNewBranchDialog(t *testing.T) {
	a, tt := advancedWorkspace(t)
	calls := 0
	a.OperateRepository = func(_ context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		calls++
		if op.Kind != domain.CreateWorktree || op.Mode != "New Branch" || op.Branch != "review/测试" || op.Source != "head" || op.Path == "" {
			t.Fatalf("wrong creation %+v", op)
		}
		done(repo, nil, nil)
	}
	a.run(newWorktree, commandTarget{})
	tt.Frame()
	if err := tt.Click("Worktree mode"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyEnd)
	tt.Key(0, native.KeyEnter)
	if _, ok := tt.Find("New worktree branch"); !ok {
		t.Fatal("new branch field unavailable")
	}
	if button, ok := tt.Access("Run New Worktree"); !ok || !button.Disabled {
		t.Fatal("incomplete new branch creation enabled")
	}
	if err := tt.Click("New worktree directory"); err != nil {
		t.Fatal(err)
	}
	tt.Type(filepath.Join(t.TempDir(), "review"))
	if button, ok := tt.Access("Run New Worktree"); !ok || !button.Disabled {
		t.Fatal("new branch creation enabled without a name")
	}
	if err := tt.Click("New worktree branch"); err != nil {
		t.Fatal(err)
	}
	tt.Type("review/测试")
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, theme := range []string{"light", "dark"} {
			a.Layout.Dark = theme == "dark"
			tt.Frame()
			writeScreenshot(t, tt, filepath.Join(dir, "worktree-new-branch-"+theme+".png"))
		}
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("Cancel created branch")
	}
	a.run(newWorktree, commandTarget{})
	a.repositoryOperation.op.Mode = "New Branch"
	a.repositoryOperation.op.Path = filepath.Join(t.TempDir(), "review")
	a.repositoryOperation.op.Branch = "review/测试"
	tt.Frame()
	if err := tt.Click("Run New Worktree"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("creation was not submitted")
	}
}

func TestBisectCandidateWorktreeDialog(t *testing.T) {
	a, tt := advancedWorkspace(t)
	repo := &a.Model.Active().Repository
	repo.Bisect = domain.BisectState{Version: "search-step", Original: "main", Candidate: "outside-log-candidate", Supported: true, NoCheckout: true}
	calls := 0
	a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		calls++
		if op.Kind != domain.CreateWorktree || op.Mode != "Detached" || op.Source != "outside-log-candidate" || op.BisectVersion != "search-step" || snapshot.Bisect.Version != "search-step" {
			t.Fatalf("wrong candidate operation %+v", op)
		}
		done(snapshot, nil, nil)
	}
	tt.Frame()
	if err := tt.Click("Create Candidate Worktree…"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("outside-log-candidate") {
		t.Fatal("candidate OID not shown")
	}
	if button, ok := tt.Access("Run Create Candidate Worktree"); !ok || !button.Disabled {
		t.Fatal("missing path did not disable submit")
	}
	if _, ok := tt.Find("Worktree mode"); ok {
		t.Fatal("candidate mode can be changed")
	}
	if _, ok := tt.Find("Worktree source"); ok {
		t.Fatal("candidate source can be changed")
	}
	if err := tt.Click("New worktree directory"); err != nil {
		t.Fatal(err)
	}
	tt.Type(filepath.Join(t.TempDir(), "candidate"))
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, theme := range []string{"light", "dark"} {
			a.Layout.Dark = theme == "dark"
			tt.Frame()
			writeScreenshot(t, tt, filepath.Join(dir, "bisect-candidate-worktree-"+theme+".png"))
		}
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("Cancel created candidate checkout")
	}
	// The Git menu and Commit controls must reach the same command.
	a.mainMenuExpanded = true
	tt.Frame()
	if err := tt.Click("Git menu"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Bisect", "Create Candidate Worktree…"); err != nil {
		t.Fatal(err)
	}
	// Updating visible metadata must not change the request's confirmation identity.
	a.Model.Active().Repository.Bisect.Version = "newer-step"
	a.repositoryOperation.op.Path = filepath.Join(t.TempDir(), "candidate")
	tt.Frame()
	if err := tt.Click("Run Create Candidate Worktree"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("candidate request did not use existing operation pipeline")
	}
}

func TestBisectCandidateWorktreeAvailability(t *testing.T) {
	a, _ := advancedWorkspace(t)
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	if a.disabledFor(createBisectWorktree, commandTarget{}) == "" {
		t.Fatal("candidate creation enabled outside Bisect")
	}
	a.Model.Active().Repository.Bisect = domain.BisectState{Version: "step", Candidate: "candidate", Supported: true, NoCheckout: true}
	if reason := a.disabledFor(createBisectWorktree, commandTarget{}); reason != "" {
		t.Fatal(reason)
	}
	a.Model.Active().Repository.Bisect.Ambiguous = true
	if a.disabledFor(createBisectWorktree, commandTarget{}) == "" {
		t.Fatal("ambiguous search has no exact candidate to test")
	}
}

func TestBisectControlsKeepCommitActionsVisible(t *testing.T) {
	a, tt := advancedWorkspace(t)
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	a.Model.Active().Repository.Bisect = domain.BisectState{Version: "step", Candidate: "head", Supported: true, NoCheckout: true}
	tt.Frame()
	commit, hasCommit := tt.Find("Create Commit")
	gitTab, hasLog := tt.Find("Log")
	if !hasCommit || !hasLog || commit.H < 24 || commit.Y+commit.H > gitTab.Y {
		t.Fatalf("Bisect controls clipped Commit actions: commit=%+v, log=%+v", commit, gitTab)
	}
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, theme := range []string{"light", "dark"} {
			a.Layout.Dark = theme == "dark"
			tt.Frame()
			writeScreenshot(t, tt, filepath.Join(dir, "bisect-controls-"+theme+".png"))
		}
	}
}
