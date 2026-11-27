package gitcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "internal-rebase-editor" {
		if err := RunInteractiveRebaseEditor(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func interactiveRebaseFixture(t *testing.T, count int) (*workflowFixture, []string) {
	t.Helper()
	fixture := newWorkflow(t)
	fixture.git("checkout", "-b", "feature")
	ids := []string{fixture.git("rev-parse", "HEAD")}
	for index := 1; index <= count; index++ {
		fixture.write(fmt.Sprintf("file %d 中文.txt", index), fmt.Sprintf("content %d\n", index))
		fixture.git("add", ".")
		fixture.git("-c", fmt.Sprintf("user.name=Author %d", index), "commit", "--date="+fmt.Sprintf("2001-01-%02dT12:00:00+0000", index), "-m", fmt.Sprintf("commit %d", index))
		ids = append(ids, fixture.git("rev-parse", "HEAD"))
	}
	return fixture, ids
}

func interactiveRebaseOperation(t *testing.T, fixture *workflowFixture, selection []string, steps []domain.InteractiveRebaseStep) error {
	t.Helper()
	preview, err := fixture.g.PreviewCommitRewrite(context.Background(), fixture.read(), domain.InteractiveRebase, selection)
	if err != nil {
		t.Fatal(err)
	}
	return fixture.g.ExecuteRepositoryOperation(context.Background(), fixture.read(), domain.RepositoryOperation{
		Kind: domain.InteractiveRebase, RebaseSteps: steps, RebaseVersion: preview.Version, ConfirmRewrite: true,
	})
}

func TestInteractiveRebaseActionsAndFinalTrees(t *testing.T) {
	for _, action := range []domain.InteractiveRebaseAction{domain.RebasePick, domain.RebaseReword, domain.RebaseSquash, domain.RebaseFixup, domain.RebaseDrop} {
		action := action
		t.Run(string(action), func(t *testing.T) {
			t.Parallel()
			fixture, ids := interactiveRebaseFixture(t, 3)
			selection := []string{ids[1]}
			steps := []domain.InteractiveRebaseStep{{Commit: ids[1], Action: action}}
			if action == domain.RebaseSquash || action == domain.RebaseFixup {
				selection = []string{ids[1], ids[2]}
				steps = []domain.InteractiveRebaseStep{{Commit: ids[1], Action: domain.RebasePick}, {Commit: ids[2], Action: action}}
			}
			if action == domain.RebaseReword {
				steps[0].Message = "rewritten 中文\n\nbody"
			}
			originalTree := fixture.git("rev-parse", "HEAD^{tree}")
			if err := interactiveRebaseOperation(t, fixture, selection, steps); err != nil {
				t.Fatal(err)
			}
			if fixture.read().Rebase.Active() || fixture.git("status", "--porcelain=v1") != "" {
				t.Fatal("successful Rebase left an operation or dirty checkout")
			}
			switch action {
			case domain.RebaseReword:
				if fixture.git("log", "--format=%B", "-1", "HEAD~2") != "rewritten 中文\n\nbody" {
					t.Fatal("Reword message was not applied")
				}
			case domain.RebaseSquash:
				message := fixture.git("log", "--format=%B", "-1", "HEAD~1")
				if !strings.Contains(message, "commit 1") || !strings.Contains(message, "commit 2") || fixture.git("rev-list", "--count", "HEAD") != "3" {
					t.Fatal("Squash did not combine the selected messages")
				}
			case domain.RebaseFixup:
				message := fixture.git("log", "--format=%B", "-1", "HEAD~1")
				if !strings.Contains(message, "commit 1") || strings.Contains(message, "commit 2") || fixture.git("rev-list", "--count", "HEAD") != "3" {
					t.Fatal("Fixup did not retain only the first message")
				}
			case domain.RebaseDrop:
				if _, err := os.Stat(filepath.Join(fixture.root, "file 1 中文.txt")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("Drop retained the removed file", err)
				}
			default:
				if fixture.git("rev-parse", "HEAD^{tree}") != originalTree {
					t.Fatal("Pick changed the final tree")
				}
			}
		})
	}
}

func TestInteractiveRebaseCombinedReorderAuthorsAndDescendant(t *testing.T) {
	t.Parallel()
	fixture, ids := interactiveRebaseFixture(t, 6)
	selection := slices.Clone(ids[1:6])
	steps := []domain.InteractiveRebaseStep{
		{Commit: ids[3], Action: domain.RebaseReword, Message: "combined message"},
		{Commit: ids[4], Action: domain.RebaseSquash},
		{Commit: ids[1], Action: domain.RebasePick},
		{Commit: ids[2], Action: domain.RebaseDrop},
		{Commit: ids[5], Action: domain.RebaseFixup},
	}
	if err := interactiveRebaseOperation(t, fixture, selection, steps); err != nil {
		t.Fatal(err)
	}
	subjects := fixture.git("log", "--reverse", "--format=%s", "HEAD")
	if subjects != "base\ncombined message\ncommit 1\ncommit 6" {
		t.Fatalf("unexpected final sequence:\n%s", subjects)
	}
	for _, path := range []string{"file 1 中文.txt", "file 3 中文.txt", "file 4 中文.txt", "file 5 中文.txt", "file 6 中文.txt"} {
		if _, err := os.Stat(filepath.Join(fixture.root, path)); err != nil {
			t.Fatalf("final tree lost %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(fixture.root, "file 2 中文.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("combined Drop retained its file", err)
	}
	combined := fixture.git("rev-parse", "HEAD~2")
	if fixture.git("show", "-s", "--format=%an", combined) != "Author 3" || fixture.git("show", "-s", "--format=%aI", combined) != fixture.git("show", "-s", "--format=%aI", ids[3]) {
		t.Fatal("Squash/Fixup did not preserve the first commit author and author date")
	}
	for _, commit := range []string{fixture.git("rev-parse", "HEAD~1"), fixture.git("rev-parse", "HEAD")} {
		old := ids[1]
		if commit == fixture.git("rev-parse", "HEAD") {
			old = ids[6]
		}
		if fixture.git("show", "-s", "--format=%an", commit) != fixture.git("show", "-s", "--format=%an", old) || fixture.git("show", "-s", "--format=%aI", commit) != fixture.git("show", "-s", "--format=%aI", old) {
			t.Fatal("Pick/Rebase changed author or author date")
		}
	}
}

func TestInteractiveRebaseRootAndInvalidPlans(t *testing.T) {
	t.Parallel()
	fixture, ids := interactiveRebaseFixture(t, 2)
	rootSteps := []domain.InteractiveRebaseStep{{Commit: ids[0], Action: domain.RebasePick}, {Commit: ids[1], Action: domain.RebaseReword, Message: "rewritten first"}}
	if err := interactiveRebaseOperation(t, fixture, ids[:2], rootSteps); err != nil {
		t.Fatal(err)
	}
	if fixture.git("log", "--reverse", "--format=%s") != "base\nrewritten first\ncommit 2" {
		t.Fatal("root range or descendants were not rewritten")
	}
	fixture, ids = interactiveRebaseFixture(t, 2)
	preview, err := fixture.g.PreviewCommitRewrite(context.Background(), fixture.read(), domain.InteractiveRebase, ids[1:])
	if err != nil {
		t.Fatal(err)
	}
	original := fixture.read().Head
	invalid := [][]domain.InteractiveRebaseStep{
		{{Commit: ids[1], Action: domain.RebaseSquash}, {Commit: ids[2], Action: domain.RebasePick}},
		{{Commit: ids[1], Action: domain.RebaseFixup}, {Commit: ids[2], Action: domain.RebasePick}},
		{{Commit: ids[1], Action: domain.RebasePick}, {Commit: ids[1], Action: domain.RebasePick}},
		{{Commit: ids[1], Action: domain.RebasePick}},
		{{Commit: ids[1], Action: domain.RebaseReword, Message: " "}, {Commit: ids[2], Action: domain.RebasePick}},
	}
	for _, steps := range invalid {
		err = fixture.g.ExecuteRepositoryOperation(context.Background(), fixture.read(), domain.RepositoryOperation{Kind: domain.InteractiveRebase, RebaseSteps: steps, RebaseVersion: preview.Version, ConfirmRewrite: true})
		if err == nil || fixture.git("rev-parse", "HEAD") != original {
			t.Fatalf("invalid plan executed or moved HEAD: %+v, %v", steps, err)
		}
	}
}

func TestInteractiveRebaseAllDropHandling(t *testing.T) {
	t.Parallel()
	fixture, ids := interactiveRebaseFixture(t, 2)
	steps := []domain.InteractiveRebaseStep{{Commit: ids[1], Action: domain.RebaseDrop}, {Commit: ids[2], Action: domain.RebaseDrop}}
	if err := interactiveRebaseOperation(t, fixture, ids[1:], steps); err != nil {
		t.Fatal(err)
	}
	if fixture.git("rev-parse", "HEAD") != ids[0] || fixture.git("status", "--porcelain=v1") != "" {
		t.Fatal("dropping the selected suffix did not restore its parent")
	}
	repo := fixture.read()
	preview, err := fixture.g.PreviewCommitRewrite(context.Background(), repo, domain.InteractiveRebase, []string{ids[0]})
	if err != nil {
		t.Fatal(err)
	}
	allRoot := []domain.InteractiveRebaseStep{{Commit: ids[0], Action: domain.RebaseDrop}}
	err = fixture.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.InteractiveRebase, RebaseSteps: allRoot, RebaseVersion: preview.Version, ConfirmRewrite: true})
	if err == nil || !strings.Contains(err.Error(), "entire branch") || fixture.git("rev-parse", "HEAD") != ids[0] {
		t.Fatalf("all-Drop root plan was not safely rejected: %v", err)
	}
}

func TestInteractiveRebaseRejectsUnsafeStaleAndCanceled(t *testing.T) {
	t.Parallel()
	fixture, ids := interactiveRebaseFixture(t, 2)
	selection := ids[1:]
	steps := []domain.InteractiveRebaseStep{{Commit: ids[1], Action: domain.RebasePick}, {Commit: ids[2], Action: domain.RebaseReword, Message: "new message"}}
	repo := fixture.read()
	preview, err := fixture.g.PreviewCommitRewrite(context.Background(), repo, domain.InteractiveRebase, selection)
	if err != nil {
		t.Fatal(err)
	}
	op := domain.RepositoryOperation{Kind: domain.InteractiveRebase, RebaseSteps: steps, RebaseVersion: preview.Version, ConfirmRewrite: true}
	protected := op
	protected.Safety.ProtectedBranches = "feature"
	if err := fixture.g.ExecuteRepositoryOperation(context.Background(), repo, protected); err == nil {
		t.Fatal("protected branch was rewritten")
	}
	unconfirmed := op
	unconfirmed.ConfirmRewrite = false
	if err := fixture.g.ExecuteRepositoryOperation(context.Background(), repo, unconfirmed); err == nil {
		t.Fatal("unconfirmed plan was executed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := fixture.g.ExecuteRepositoryOperation(ctx, repo, op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	fixture.write("file 1 中文.txt", "dirty\n")
	if _, err := fixture.g.PreviewCommitRewrite(context.Background(), fixture.read(), domain.InteractiveRebase, selection); err == nil {
		t.Fatal("dirty worktree was accepted")
	}
	fixture.write("file 1 中文.txt", "content 1\n")
	remote := filepath.Join(t.TempDir(), "remote.git")
	fixture.git("init", "--bare", remote)
	fixture.git("remote", "add", "origin", remote)
	fixture.git("push", "origin", "HEAD:published")
	fixture.git("fetch", "origin")
	if _, err := fixture.g.PreviewCommitRewrite(context.Background(), fixture.read(), domain.InteractiveRebase, selection); err == nil {
		t.Fatal("published range was accepted")
	}
	fixture.git("update-ref", "-d", "refs/remotes/origin/published")
	fixture.git("update-ref", "refs/remotes/origin/unrelated", ids[0])
	if err := fixture.g.ExecuteRepositoryOperation(context.Background(), fixture.read(), op); err == nil {
		t.Fatal("stale remote-ref preview was accepted")
	}
	fixture.git("update-ref", "-d", "refs/remotes/origin/unrelated")
	fixture.write("new.txt", "new\n")
	fixture.git("add", ".")
	fixture.git("commit", "-m", "new head")
	if err := fixture.g.ExecuteRepositoryOperation(context.Background(), fixture.read(), op); err == nil {
		t.Fatal("stale HEAD preview was accepted")
	}
	if fixture.git("rev-parse", "HEAD") == repo.Head {
		t.Fatal("test setup did not advance HEAD")
	}
}

func TestInteractiveRebaseReorderConflictContinueAndAbort(t *testing.T) {
	for _, mode := range []string{"continue", "reword-continue", "abort"} {
		mode := mode
		abort, reword := mode == "abort", mode == "reword-continue"
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			fixture := newWorkflow(t)
			fixture.git("checkout", "-b", "feature")
			var ids []string
			for _, value := range []string{"first\n", "second\n"} {
				fixture.write("文件 name.txt", value)
				fixture.git("commit", "-am", strings.TrimSpace(value))
				ids = append(ids, fixture.git("rev-parse", "HEAD"))
			}
			fixture.write("later.txt", "later\n")
			fixture.git("add", ".")
			fixture.git("commit", "-m", "later")
			repo := fixture.read()
			originalTree := fixture.git("write-tree")
			preview, err := fixture.g.PreviewCommitRewrite(context.Background(), repo, domain.InteractiveRebase, ids)
			if err != nil {
				t.Fatal(err)
			}
			steps := []domain.InteractiveRebaseStep{{Commit: ids[1], Action: domain.RebasePick}, {Commit: ids[0], Action: domain.RebasePick}}
			wantLog := "later\nfirst\nsecond"
			if reword {
				// The conflict stops on a Reword step: Continue must still apply
				// the confirmed replacement message through the recovery editor.
				steps[0] = domain.InteractiveRebaseStep{Commit: ids[1], Action: domain.RebaseReword, Message: "renamed second\n"}
				wantLog = "later\nfirst\nrenamed second"
			}
			err = fixture.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.InteractiveRebase, RebaseSteps: steps, RebaseVersion: preview.Version, ConfirmRewrite: true})
			paused := fixture.read()
			if err == nil || !paused.Rebase.Active() || len(paused.Conflicts) == 0 || paused.Rebase.Original != repo.Head {
				t.Fatalf("missing native reorder conflict: %v, %+v", err, paused.Rebase)
			}
			if abort {
				if err := fixture.g.ExecuteRepositoryOperation(context.Background(), paused, domain.RepositoryOperation{Kind: domain.AbortRebase}); err != nil {
					t.Fatal(err)
				}
				if fixture.git("rev-parse", "HEAD") != repo.Head || fixture.git("write-tree") != originalTree {
					t.Fatal("Abort did not restore the original branch and tree")
				}
				return
			}
			fixture.write("文件 name.txt", "resolved\n")
			fixture.git("add", ".")
			if unresolved := fixture.git("diff", "--name-only", "--diff-filter=U"); unresolved != "" || fixture.git("show", ":文件 name.txt") != "resolved" {
				t.Fatalf("resolution was not staged: %q", unresolved)
			}
			stopped, _ := os.ReadFile(filepath.Join(fixture.root, ".git", "rebase-merge", "stopped-sha"))
			planPath := filepath.Join(fixture.root, ".git", "rebase-merge", rebasePlanFile)
			if _, err := os.Stat(planPath); err != nil || strings.TrimSpace(string(stopped)) != ids[1] {
				t.Fatalf("missing controlled recovery plan or last completed commit: %v, stopped=%q second=%q", err, stopped, ids[1])
			}
			// Reordering two edits of the same line conflicts twice: replaying
			// "second" onto the base, then replaying "first" onto the resolution.
			err = fixture.g.ExecuteRepositoryOperation(context.Background(), fixture.read(), domain.RepositoryOperation{Kind: domain.ContinueRebase})
			again := fixture.read()
			if err == nil || !again.Rebase.Active() || len(again.Conflicts) == 0 {
				t.Fatalf("expected the second native reorder conflict: %v, %+v", err, again.Rebase)
			}
			fixture.write("文件 name.txt", "final\n")
			fixture.git("add", ".")
			if err := fixture.g.ExecuteRepositoryOperation(context.Background(), fixture.read(), domain.RepositoryOperation{Kind: domain.ContinueRebase}); err != nil {
				t.Fatal(err)
			}
			if fixture.read().Rebase.Active() || fixture.git("show", "HEAD~1:文件 name.txt") != "final" || fixture.git("show", "HEAD~2:文件 name.txt") != "resolved" || fixture.git("log", "--format=%s", "-3") != wantLog {
				t.Fatalf("Continue did not finish the reordered sequence: active=%v file=%q log=%q", fixture.read().Rebase.Active(), fixture.git("show", "HEAD~1:文件 name.txt"), fixture.git("log", "--format=%s", "-3"))
			}
			if _, err := os.Stat(planPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("completed Rebase retained its private recovery plan")
			}
		})
	}
}

func TestInteractiveRebaseEditorVerifiesInitialAndFinalTodo(t *testing.T) {
	first, second := strings.Repeat("a", 40), strings.Repeat("b", 40)
	preview := domain.CommitRewritePreview{Selected: []string{second, first}}
	steps := []domain.InteractiveRebaseStep{{Commit: second, Action: domain.RebaseReword, Message: "changed"}, {Commit: first, Action: domain.RebaseDrop}}
	plan, err := prepareInteractiveRebasePlan(preview, steps)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	plan.Token = "token"
	planData, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(directory, "plan.json")
	if err := os.WriteFile(planPath, planData, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(rebasePlanEnvironment, planPath)
	t.Setenv(rebaseTokenEnvironment, "token")
	todo := filepath.Join(directory, "git-rebase-todo")
	if err := os.WriteFile(todo, []byte("pick "+first[:8]+" first\npick "+second[:8]+" second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunInteractiveRebaseEditor([]string{"sequence", todo}); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(todo)
	if string(actual) != plan.Todo {
		t.Fatalf("todo = %q, want %q", actual, plan.Todo)
	}
	if err := os.WriteFile(todo, []byte("pick "+second+"\npick "+first+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunInteractiveRebaseEditor([]string{"sequence", todo}); err == nil {
		t.Fatal("reordered initial todo was accepted")
	}
}
