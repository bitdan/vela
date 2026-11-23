package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func advancedWorkspace(t *testing.T) (*Workspace, *native.Tester) {
	t.Helper()
	a := New(application.New(), application.DefaultLayout())
	repo := domain.Repository{Name: "sample", Root: t.TempDir(), CurrentBranch: "main", Head: "head", IndexVersion: "index", WorktreeVersion: "worktree",
		History: []domain.Commit{{Hash: "selected", Subject: "Selected commit"}, {Hash: "head", Subject: "Current tip"}}}
	a.Model.Switch(a.installRepository(repo))
	return a, native.NewTester(a.View, 1440, 900)
}

func TestAdvancedResetConfirmation(t *testing.T) {
	a, tt := advancedWorkspace(t)
	a.Model.Active().Repository.CurrentBranch = "feature/reset"
	calls := 0
	a.OperateRepository = func(_ context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		calls++
		if op.Kind != domain.ResetRevision || op.Source != "selected" || op.Mode != "Hard" {
			t.Fatalf("wrong reset %+v", op)
		}
		done(repo, nil, nil)
	}
	a.run(resetRevision, commandTarget{source: "selected"})
	tt.Frame()
	if a.repositoryOperation == nil {
		t.Fatal("reset command did not open confirmation")
	}
	if !tt.HasText("selected") || !tt.HasText("feature/reset") {
		t.Fatal("confirmation omitted target")
	}
	if err := tt.Click("Reset mode"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyDown)
	tt.Key(0, native.KeyEnter)
	if button, ok := tt.Access("Run Reset"); !ok || !button.Disabled {
		t.Fatal("Hard reset allowed without acknowledgment")
	}
	if err := tt.Click("I understand Hard reset discards tracked changes"); err != nil {
		t.Fatal(err)
	}
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for _, theme := range []string{"light", "dark"} {
			a.Layout.Dark = theme == "dark"
			tt.Frame()
			writeScreenshot(t, tt, filepath.Join(dir, "reset-hard-"+theme+".png"))
		}
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("cancel executed reset")
	}
	a.run(resetRevision, commandTarget{source: "selected"})
	tt.Frame()
	a.repositoryOperation.op.Mode = "Hard"
	a.repositoryOperation.acknowledged = true
	tt.Frame()
	if err := tt.Click("Run Reset"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("reset did not execute once")
	}
}

func TestAdvancedStashControls(t *testing.T) {
	a, tt := advancedWorkspace(t)
	a.Model.Active().Repository.Stashes = []domain.StashEntry{{OID: "saved-oid", Selector: "stash@{0}", Subject: "Saved change"}}
	calls := 0
	a.OperateRepository = func(_ context.Context, repo domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		calls++
		if op.Kind != domain.ApplyStash || op.Source != "saved-oid" || op.Target != "stash@{0}" {
			t.Fatalf("wrong stash %+v", op)
		}
		done(repo, nil, nil)
	}
	a.run(unstashChanges, commandTarget{})
	tt.Frame()
	if a.repositoryOperation == nil || !tt.HasText("Apply retains the stash. Drop is a separate confirmed operation.") {
		t.Fatal("no stash management")
	}
	if err := tt.Click("Run Apply Stash"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(a.Model.Active().Repository.Stashes) != 1 {
		t.Fatal("apply lost entry or did not run")
	}
	a.run(dropStash, commandTarget{source: "saved-oid"})
	tt.Frame()
	if !tt.HasText("Delete this stash entry permanently. Working files are kept.") {
		t.Fatal("drop has no impact confirmation")
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("cancel dropped stash")
	}
}

func TestAdvancedWorktreeManagement(t *testing.T) {
	a, tt := advancedWorkspace(t)
	repo := &a.Model.Active().Repository
	repo.Worktrees = []domain.Worktree{{Path: repo.Root, Head: "head", Branch: "refs/heads/main"}, {Path: filepath.Join(t.TempDir(), "linked"), Head: "other", Branch: "refs/heads/feature"}}
	calls := 0
	a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		calls++
		if op.Kind != domain.RemoveWorktree || op.Path != snapshot.Worktrees[1].Path {
			t.Fatalf("wrong removal %+v", op)
		}
		done(snapshot, nil, nil)
	}
	a.run(showWorktrees, commandTarget{})
	tt.Frame()
	if a.repositoryOperation == nil {
		t.Fatal("worktree manager did not open")
	}
	if button, ok := tt.Access("Run Remove Worktree"); !ok || !button.Disabled {
		t.Fatal("main worktree removal enabled")
	}
	if err := tt.Click("Worktree path"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyDown)
	tt.Key(0, native.KeyEnter)
	if err := tt.Click("Run Remove Worktree"); err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("removal did not require separate confirmation")
	}
	if !tt.HasText("Remove this clean linked worktree directory. Its branch is kept. Dirty or locked worktrees are refused.") {
		t.Fatal("missing removal impact")
	}
	if err := tt.Click("Run Remove Worktree"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("removal did not execute")
	}
	a.run(newWorktree, commandTarget{})
	tt.Frame()
	if button, ok := tt.Access("Run New Worktree"); !ok || !button.Disabled {
		t.Fatal("creation enabled without path")
	}
}

func TestAdvancedBisectControls(t *testing.T) {
	a, tt := advancedWorkspace(t)
	repo := &a.Model.Active().Repository
	repo.Bisect = domain.BisectState{Version: "search-version", Original: "main", Candidate: "selected", Bad: "head", Good: []string{"good"}, NoCheckout: true, Supported: true}
	calls := 0
	a.OperateRepository = func(_ context.Context, snapshot domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		calls++
		if op.Kind != domain.BisectGood || op.Source != "selected" || snapshot.Bisect.Version != "search-version" {
			t.Fatalf("wrong bisect %+v", op)
		}
		snapshot.Bisect.FirstBad = "head"
		done(snapshot, nil, nil)
	}
	a.run(bisectGood, commandTarget{})
	tt.Frame()
	if a.repositoryOperation == nil || !tt.HasText("selected") {
		t.Fatal("candidate missing from confirmation")
	}
	if err := tt.Click("Run Mark Good"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("classification did not execute")
	}
	if a.disabledFor(bisectBad, commandTarget{}) == "" {
		t.Fatal("classification enabled after finding first bad")
	}
	a.run(resetBisect, commandTarget{})
	tt.Frame()
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("cancel ended bisect")
	}
	if a.disabledFor(resetRevision, commandTarget{source: "head"}) == "" {
		t.Fatal("Reset enabled during Bisect")
	}
}

func TestAdvancedStashConflictRecovery(t *testing.T) {
	a, tt := advancedWorkspace(t)
	a.Model.Active().Repository.Stashes = []domain.StashEntry{{OID: "saved", Selector: "stash@{0}", Subject: "Saved"}}
	a.OperateRepository = func(_ context.Context, repo domain.Repository, _ domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		repo.Conflicts = []domain.MergeConflict{{Path: "conflict.txt", Current: "ours", Incoming: "theirs"}}
		repo.Working = []domain.File{{Path: "conflict.txt", Status: "U"}}
		done(repo, errors.New("fixture conflict"), nil)
	}
	a.run(unstashChanges, commandTarget{})
	tt.Frame()
	if err := tt.Click("Run Apply Stash"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.repositoryOperation != nil {
		t.Fatal("Stash conflict trapped behind operation dialog")
	}
	if !a.Model.Active().Repository.ResolvingConflicts() {
		t.Fatal("no conflict recovery state")
	}
	if a.disabledFor(resetRevision, commandTarget{source: "head"}) != "" {
		t.Fatal("no explicit discard path for standalone conflict")
	}
}

func TestAdvancedWorkflowScreenshots(t *testing.T) {
	dir := os.Getenv("VELA_SCREENSHOTS")
	if dir == "" {
		t.Skip("set VELA_SCREENSHOTS to export PNGs")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"light", "dark"} {
		for _, feature := range []string{"reset", "stash", "worktrees", "bisect"} {
			t.Run(theme+"-"+feature, func(t *testing.T) {
				a, tt := advancedWorkspace(t)
				a.Layout.Dark = theme == "dark"
				repo := &a.Model.Active().Repository
				a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
				}
				switch feature {
				case "reset":
					a.run(resetRevision, commandTarget{source: "selected"})
				case "stash":
					repo.Stashes = []domain.StashEntry{{OID: "0123456789abcdef0123456789abcdef01234567", Selector: "stash@{0}", Subject: "On main: Save native workspace changes"}}
					a.run(unstashChanges, commandTarget{})
				case "worktrees":
					repo.Worktrees = []domain.Worktree{{Path: repo.Root, Head: "head", Branch: "refs/heads/main"}, {Path: "F:/work/vela-review", Head: "selected", Branch: "refs/heads/review"}}
					a.run(showWorktrees, commandTarget{})
				case "bisect":
					repo.Bisect = domain.BisectState{Version: "search", Original: "main", Candidate: "selected", Bad: "head", Good: []string{"good"}, Supported: true, NoCheckout: true}
				}
				tt.Frame()
				writeScreenshot(t, tt, filepath.Join(dir, feature+"-"+theme+".png"))
			})
		}
	}
}

func TestAdvancedWorktreeOpenPreservesSession(t *testing.T) {
	a, tt := advancedWorkspace(t)
	original := a.Model.Index
	a.Model.Active().Message = "keep draft"
	linked := filepath.Join(t.TempDir(), "linked")
	a.Model.Active().Repository.Worktrees = []domain.Worktree{{Path: a.Model.Active().Repository.Root}, {Path: linked, Head: "other", Branch: "refs/heads/feature"}}
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		t.Fatal("opening a worktree started a write")
	}
	a.ReadRepository = func(_ context.Context, path string, done func(domain.Repository, error)) {
		if path != linked {
			t.Fatalf("opened wrong worktree %q", path)
		}
		done(domain.Repository{Root: path, Name: "linked", CurrentBranch: "feature", Head: "other", IndexVersion: "new-index"}, nil)
	}
	a.run(showWorktrees, commandTarget{})
	tt.Frame()
	if err := tt.Click("Worktree path"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyDown)
	tt.Key(0, native.KeyEnter)
	if err := tt.Click("Open Worktree"); err != nil {
		t.Fatal(err)
	}
	if a.Model.Active().Repository.Root != linked || a.Model.Sessions[original].Message != "keep draft" {
		t.Fatal("opening a worktree lost session state")
	}
}

func TestAdvancedCancellationDoesNotOverwriteAnotherRepository(t *testing.T) {
	a, tt := advancedWorkspace(t)
	a.Model.Active().Repository.CurrentBranch = "feature/reset"
	snapshot := a.Model.Active().Repository
	var pending func(domain.Repository, error, error)
	var operationContext context.Context
	a.OperateRepository = func(ctx context.Context, repo domain.Repository, _ domain.RepositoryOperation, done func(domain.Repository, error, error)) {
		if repo.Head != snapshot.Head || repo.Root != snapshot.Root {
			t.Fatal("confirmation snapshot changed")
		}
		operationContext = ctx
		pending = done
	}
	a.run(resetRevision, commandTarget{source: "selected"})
	tt.Frame()
	if err := tt.Click("Run Reset"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if operationContext.Err() != context.Canceled {
		t.Fatal("Cancel did not cancel context")
	}
	a.run(selectRepo, commandTarget{index: 0})
	status := a.status
	// A canceled Git process may already have changed HEAD; refresh its actual
	// repository without writing into the newly active session.
	snapshot.Head = "actual-result"
	pending(snapshot, context.Canceled, nil)
	tt.Frame()
	if a.Model.Index != 0 || a.Model.Active().Repository.Root != "" || a.status != status || a.repositoryWrite != "" {
		t.Fatal("late operation polluted the other repository")
	}
	found := false
	for _, session := range a.Model.Sessions {
		if session.Repository.Root == snapshot.Root {
			found = session.Repository.Head == "actual-result"
		}
	}
	if !found {
		t.Fatal("original repository did not receive actual post-cancel state")
	}
}
