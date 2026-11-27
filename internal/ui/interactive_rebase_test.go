package ui

import (
	"context"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func interactiveRebaseUIFixture(t *testing.T, theme string) (*Workspace, *native.Tester, domain.Repository, []string) {
	t.Helper()
	workspace := New(application.New(), application.DefaultLayout())
	workspace.Layout.Dark = theme == "dark"
	repo := sessionRepository(t.TempDir())
	ids := []string{strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40), strings.Repeat("d", 40)}
	repo.CurrentBranch, repo.Head = "feature", ids[2]
	repo.Working, repo.Staged = nil, nil
	repo.History = []domain.Commit{
		{Hash: ids[2], Subject: "descendant"},
		{Hash: ids[1], Subject: "second"},
		{Hash: ids[0], Subject: "first"},
		{Hash: ids[3], Subject: "base"},
	}
	workspace.Model.Switch(workspace.installRepository(repo))
	workspace.repo().logChosen.Add(ids[0])
	workspace.repo().logChosen.Add(ids[1])
	workspace.PreviewCommitRewrite = func(_ context.Context, _ domain.Repository, kind domain.RepositoryOperationKind, selection []string, done func(domain.CommitRewritePreview, error)) {
		if kind != domain.InteractiveRebase || len(selection) != 2 {
			t.Fatalf("unexpected preview: %v %v", kind, selection)
		}
		done(domain.CommitRewritePreview{
			Version: "interactive-rebase:v1", Selected: []string{ids[1], ids[0]}, Replay: []string{ids[2]},
			RebaseCommits: []domain.InteractiveRebaseCommit{{Commit: ids[0], Message: "first\n"}, {Commit: ids[1], Message: "second\n"}},
		}, nil)
	}
	workspace.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		t.Fatal("unexpected repository write")
	}
	tester := native.NewTester(workspace.View, 1440, 900)
	if err := tester.RightClick("Log commit " + ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := tester.ChooseMenuItem("Interactively Rebase Selected Commits…"); err != nil {
		t.Fatal(err)
	}
	if workspace.interactiveRebase == nil {
		t.Fatal("Log command did not open Interactive Rebase")
	}
	if workspace.interactiveRebase.plan.Version == "" {
		t.Fatalf("Interactive Rebase preview missing: %s", workspace.interactiveRebase.problem)
	}
	return workspace, tester, repo, ids
}

func TestInteractiveRebaseNativeKeyboardActionsAndConfirmation(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			workspace, tester, repo, ids := interactiveRebaseUIFixture(t, theme)
			var operation domain.RepositoryOperation
			var finish func(domain.Repository, error, error)
			workspace.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				operation, finish = op, done
			}
			if !tester.HasText("Final sequence · 2 selected commits · 1 affected descendants replayed afterward") || !tester.HasText("Pick "+ids[2]) {
				t.Fatal("confirmation does not show affected descendants")
			}
			if err := tester.Click("Pick " + ids[0] + " first"); err != nil {
				t.Fatal(err)
			}
			tester.Key(native.Alt, native.KeyDown)
			dialog := workspace.interactiveRebase
			if dialog.entries[0].Commit != ids[1] || dialog.entries[1].Commit != ids[0] || dialog.selected != 1 {
				t.Fatalf("Alt+Down did not reorder: %+v", dialog.entries)
			}
			if err := tester.Click("Rebase action " + ids[1]); err != nil {
				t.Fatal(err)
			}
			tester.Key(0, native.KeyDown)
			tester.Key(0, native.KeyEnter)
			if dialog.entries[0].Action != "Reword" {
				t.Fatalf("action selector did not choose Reword: %+v", dialog.entries[0])
			}
			if err := tester.Click("Reword message " + ids[1]); err != nil {
				t.Fatal(err)
			}
			tester.Key(native.Ctrl, native.KeyA)
			tester.Type("new message 中文")
			if err := tester.Click("Rewrite this final sequence and its affected descendants"); err != nil {
				t.Fatal(err)
			}
			exportSessionScreenshot(t, tester, "interactive-rebase-"+theme)
			if err := tester.Click("Run Interactive Rebase"); err != nil {
				t.Fatal(err)
			}
			if finish == nil || operation.Kind != domain.InteractiveRebase || !operation.ConfirmRewrite || operation.RebaseVersion != "interactive-rebase:v1" {
				t.Fatalf("missing confirmed operation: %+v", operation)
			}
			expected := []domain.InteractiveRebaseStep{{Commit: ids[1], Action: domain.RebaseReword, Message: "new message 中文"}, {Commit: ids[0], Action: domain.RebasePick}}
			if len(operation.RebaseSteps) != 2 || operation.RebaseSteps[0] != expected[0] || operation.RebaseSteps[1] != expected[1] {
				t.Fatalf("wrong final plan: %+v", operation.RebaseSteps)
			}
			finish(repo, nil, nil)
			if workspace.interactiveRebase != nil || workspace.repo().logChosen.Len() != 0 {
				t.Fatal("completion left the dialog or stale Log selection")
			}
		})
	}
}

func TestInteractiveRebaseDisabledReasonsAndCancel(t *testing.T) {
	workspace, tester, _, ids := interactiveRebaseUIFixture(t, "dark")
	dialog := workspace.interactiveRebase
	if err := tester.Click("Pick " + ids[0] + " first"); err != nil {
		t.Fatal(err)
	}
	if reason := workspace.disabledFor(moveRebaseUp, commandTarget{}); !strings.Contains(reason, "edge") {
		t.Fatalf("missing movement boundary reason: %q", reason)
	}
	dialog.entries[0].Action = "Squash"
	dialog.ack = true
	if reason := workspace.disabledFor(confirmInteractiveRebase, commandTarget{}); !strings.Contains(reason, "first commit") {
		t.Fatalf("missing first Squash reason: %q", reason)
	}
	for index := range dialog.entries {
		dialog.entries[index].Action = "Drop"
	}
	dialog.plan.Replay = nil
	dialog.plan.Parent = ""
	if reason := workspace.disabledFor(confirmInteractiveRebase, commandTarget{}); !strings.Contains(reason, "keep at least one") {
		t.Fatalf("missing all-Drop reason: %q", reason)
	}
	dialog.entries[0].Action = "Pick"
	dialog.entries[1].Action = "Reword"
	dialog.entries[1].Message = "changed"
	workspace.Model.Active().Repository.Head = ids[3]
	if reason := workspace.disabledFor(confirmInteractiveRebase, commandTarget{}); !strings.Contains(reason, "Repository changed") {
		t.Fatalf("missing stale repository reason: %q", reason)
	}
	if err := tester.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if workspace.interactiveRebase != nil {
		t.Fatal("Cancel left the dialog open")
	}
	workspace.Model.Active().Repository.Head = ids[2]
	workspace.Layout.VersionControl.Git.ProtectedBranches = "feature"
	if reason := workspace.disabledFor(rebaseFrom, commandTarget{source: ids[0]}); !strings.Contains(reason, "protected") {
		t.Fatalf("missing protected branch reason: %q", reason)
	}
	workspace.Layout.VersionControl.Git.ProtectedBranches = ""
	workspace.Model.Active().Repository.Working = []domain.File{{Path: "dirty.txt", Status: "M"}}
	if reason := workspace.disabledFor(rebaseFrom, commandTarget{source: ids[0]}); !strings.Contains(reason, "clean tracked") {
		t.Fatalf("missing dirty worktree reason: %q", reason)
	}
}

func TestInteractiveRebaseLatePreviewCannotRestoreCanceledDialog(t *testing.T) {
	workspace := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.CurrentBranch, repo.Head = "feature", strings.Repeat("a", 40)
	repo.Working = nil
	repo.History = []domain.Commit{{Hash: repo.Head, Subject: "selected"}}
	workspace.Model.Switch(workspace.installRepository(repo))
	var request context.Context
	var finish func(domain.CommitRewritePreview, error)
	workspace.PreviewCommitRewrite = func(ctx context.Context, _ domain.Repository, _ domain.RepositoryOperationKind, _ []string, done func(domain.CommitRewritePreview, error)) {
		request, finish = ctx, done
	}
	workspace.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	workspace.run(rebaseFrom, commandTarget{source: repo.Head})
	workspace.run(cancelInteractiveRebase, commandTarget{})
	finish(domain.CommitRewritePreview{Version: "late", Selected: []string{repo.Head}, RebaseCommits: []domain.InteractiveRebaseCommit{{Commit: repo.Head}}}, nil)
	if request.Err() != context.Canceled || workspace.interactiveRebase != nil {
		t.Fatal("late preview restored a canceled dialog")
	}
}
