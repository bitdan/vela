package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

type workflowFixture struct {
	t    *testing.T
	root string
	g    *Executor
}

func newWorkflow(t *testing.T) *workflowFixture {
	t.Helper()
	f := &workflowFixture{t: t, root: t.TempDir(), g: New()}
	f.git("init", "--initial-branch=main")
	f.git("config", "user.name", "Vela Test")
	f.git("config", "user.email", "vela@example.test")
	f.git("config", "commit.gpgsign", "false")
	f.git("config", "core.autocrlf", "false")
	f.write("文件 name.txt", "base\n")
	f.git("add", ".")
	f.git("commit", "-m", "base")
	return f
}
func (f *workflowFixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.root
	cmd.Env = repositoryEnvironment()
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
func (f *workflowFixture) write(path, text string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.root, path), []byte(text), 0600); err != nil {
		f.t.Fatal(err)
	}
}
func (f *workflowFixture) read() domain.Repository {
	f.t.Helper()
	r, err := f.g.ReadRepository(context.Background(), f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	return r
}
func (f *workflowFixture) run(op domain.RepositoryOperation) error {
	f.t.Helper()
	return f.g.ExecuteRepositoryOperation(context.Background(), f.read(), op)
}

func TestAdvancedResetModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"Soft", "Mixed", "Hard"} {
		t.Run(mode, func(t *testing.T) {
			f := newWorkflow(t)
			base := f.git("rev-parse", "HEAD")
			f.write("文件 name.txt", "second\n")
			f.git("commit", "-am", "second")
			f.write("文件 name.txt", "working\n")
			f.write("keep.txt", "untracked\n")
			if err := f.run(domain.RepositoryOperation{Kind: domain.ResetRevision, Source: base, Mode: mode}); err != nil {
				t.Fatal(err)
			}
			if f.git("rev-parse", "HEAD") != base {
				t.Fatal("HEAD did not reset")
			}
			index := f.git("show", ":文件 name.txt")
			want := "base"
			if mode == "Soft" {
				want = "second"
			}
			if index != want {
				t.Fatalf("index = %q", index)
			}
			data, err := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
			if err != nil {
				t.Fatal(err)
			}
			want = "working\n"
			if mode == "Hard" {
				want = "base\n"
			}
			if string(data) != want {
				t.Fatalf("worktree = %q", data)
			}
			if _, err := os.Stat(filepath.Join(f.root, "keep.txt")); err != nil {
				t.Fatal("untracked file lost", err)
			}
		})
	}
}

func TestAdvancedResetRejectsStaleAndCanceled(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "first dirty\n")
	snapshot := f.read()
	f.write("文件 name.txt", "second dirty\n")
	op := domain.RepositoryOperation{Kind: domain.ResetRevision, Source: snapshot.Head, Mode: "Hard"}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, op); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale reset = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), op); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetRevision, Source: snapshot.Head, Mode: "bogus"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "second dirty\n" {
		t.Fatal("rejected reset changed data")
	}
}

func TestAdvancedResetProtectsIgnoredFiles(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("blocked.txt", "tracked\n")
	f.git("add", ".")
	f.git("commit", "-m", "with file")
	target := f.git("rev-parse", "HEAD")
	f.git("rm", "blocked.txt")
	f.write(".gitignore", "blocked.txt\n")
	f.git("add", ".")
	f.git("commit", "-m", "ignore file")
	f.write("blocked.txt", "precious\n")
	head := f.git("rev-parse", "HEAD")
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetRevision, Source: target, Mode: "Hard"}); err == nil {
		t.Fatal("reset overwrote ignored file")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "blocked.txt"))
	if string(data) != "precious\n" || f.git("rev-parse", "HEAD") != head {
		t.Fatal("failed reset changed repository")
	}
}

func TestAdvancedStashRoundTrip(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "staged\n")
	f.git("add", ".")
	f.write("文件 name.txt", "unstaged\n")
	f.write("new 空格.txt", "new\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveStash, Message: "测试 stash", IncludeUntracked: true}); err != nil {
		t.Fatal(err)
	}
	saved := f.read()
	if len(saved.Stashes) != 1 || len(saved.Working) != 0 || len(saved.Staged) != 0 {
		t.Fatalf("save did not clean tree: %+v", saved.Stashes)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyStash, Source: saved.Stashes[0].OID, Target: saved.Stashes[0].Selector, RestoreIndex: true}); err != nil {
		t.Fatal(err)
	}
	if f.git("show", ":文件 name.txt") != "staged" {
		t.Fatal("index was not restored")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "unstaged\n" {
		t.Fatal("working file was not restored")
	}
	if len(f.read().Stashes) != 1 {
		t.Fatal("apply dropped stash")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.DropStash, Source: saved.Stashes[0].OID, Target: saved.Stashes[0].Selector}); err != nil {
		t.Fatal(err)
	}
	if len(f.read().Stashes) != 0 {
		t.Fatal("drop did not remove stash")
	}
}

func TestAdvancedStashStaleAndConflicts(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "stash side\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveStash, Message: "first"}); err != nil {
		t.Fatal(err)
	}
	snapshot := f.read()
	entry := snapshot.Stashes[0]
	f.write("文件 name.txt", "second stash\n")
	f.git("stash", "push", "-m", "second")
	if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.DropStash, Source: entry.OID, Target: entry.Selector}); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("shifted stash = %v", err)
	}
	f.write("文件 name.txt", "new committed side\n")
	f.git("commit", "-am", "advance")
	current := f.read()
	entry = current.Stashes[1]
	if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyStash, Source: entry.OID, Target: entry.Selector}); err == nil {
		t.Fatal("expected apply conflict")
	}
	conflicted := f.read()
	if len(conflicted.Conflicts) == 0 || len(conflicted.Stashes) != 2 {
		t.Fatal("conflict or original stash lost")
	}
	// Stash has no MERGE_HEAD; its unmerged index still needs a usable resolution path.
	if !conflicted.ResolvingConflicts() {
		t.Fatal("standalone stash conflicts have no resolution state")
	}
	doc, err := f.g.ReadMergeDocument(context.Background(), conflicted, "文件 name.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveMergeResult, Source: "文件 name.txt", Result: "resolved\n", Conflict: doc.Conflict, WorktreeVersion: doc.WorktreeVersion}); err != nil {
		t.Fatal(err)
	}
	if len(f.read().Conflicts) != 0 || f.git("show", ":文件 name.txt") != "resolved" || len(f.read().Stashes) != 2 {
		t.Fatal("stash resolution lost data")
	}
}

func TestAdvancedStashPreservesUntrackedByDefaultAndRejectsDirtyApply(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("文件 name.txt", "stashed\n")
	f.write("keep.txt", "keep\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveStash}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "keep.txt")); err != nil {
		t.Fatal(err)
	}
	f.write("文件 name.txt", "dirty\n")
	repo := f.read()
	entry := repo.Stashes[0]
	if err := f.run(domain.RepositoryOperation{Kind: domain.ApplyStash, Source: entry.OID, Target: entry.Selector}); err == nil {
		t.Fatal("dirty apply accepted")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.DropStash, Source: entry.OID, Target: "stash@{99}"}); err == nil {
		t.Fatal("unconfirmed entry accepted")
	}
}

func TestAdvancedWorktrees(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("branch", "feature/测试")
	path := filepath.Join(t.TempDir(), "linked 空格")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateWorktree, Source: "refs/heads/feature/测试", Path: path, Mode: "Branch"}); err != nil {
		t.Fatal(err)
	}
	repo := f.read()
	if len(repo.Worktrees) != 2 {
		t.Fatalf("worktrees = %+v", repo.Worktrees)
	}
	linked, err := f.g.ReadRepository(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if linked.CurrentBranch != "feature/测试" || linked.CommonDir != repo.CommonDir || linked.GitDir == repo.GitDir {
		t.Fatal("worktree identity is incorrect")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: f.root}); err == nil {
		t.Fatal("removed current/main worktree")
	}
	if err := os.WriteFile(filepath.Join(path, "untracked.txt"), []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); err == nil {
		t.Fatal("removed dirty worktree")
	}
	if err := os.Remove(filepath.Join(path, "untracked.txt")); err != nil {
		t.Fatal(err)
	}
	f.git("worktree", "lock", path)
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); err == nil {
		t.Fatal("removed locked worktree")
	}
	f.git("worktree", "unlock", path)
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("worktree directory remains", err)
	}
	if f.git("rev-parse", "refs/heads/feature/测试") != repo.Head {
		t.Fatal("removed branch along with worktree")
	}
}

func TestAdvancedWorktreeBoundaries(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	repo := f.read()
	for _, path := range []string{f.root, filepath.Join(f.root, "nested"), "relative"} {
		if err := f.run(domain.RepositoryOperation{Kind: domain.CreateWorktree, Source: repo.Head, Mode: "Detached", Path: path}); err == nil {
			t.Fatalf("invalid path %q accepted", path)
		}
	}
	path := filepath.Join(t.TempDir(), "detached")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateWorktree, Source: repo.Head, Mode: "Detached", Path: path}); err != nil {
		t.Fatal(err)
	}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), repo, domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale list = %v", err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); err != nil {
		t.Fatal(err)
	}
}

func TestAdvancedWorktreePorcelainPaths(t *testing.T) {
	t.Parallel()
	entries, err := parseWorktrees("worktree /repo\x00HEAD aaaa\x00branch refs/heads/main\x00\x00worktree /repo with\nnewline\x00HEAD bbbb\x00detached\x00locked reason\x00\x00")
	if err != nil || len(entries) != 2 || entries[1].Path != "/repo with\nnewline" || !entries[1].Locked || !entries[1].Detached {
		t.Fatalf("parse = %+v, %v", entries, err)
	}
	if _, err := parseWorktrees("HEAD missing-path\x00\x00"); err == nil {
		t.Fatal("malformed worktree accepted")
	}
}

func TestAdvancedBisectNoCheckout(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	good := f.git("rev-parse", "HEAD")
	for i := 0; i < 7; i++ {
		f.git("commit", "--allow-empty", "-m", "step")
	}
	bad := f.git("rev-parse", "HEAD")
	if err := f.run(domain.RepositoryOperation{Kind: domain.StartBisect, Source: bad, Target: good}); err != nil {
		t.Fatal(err)
	}
	state := f.read().Bisect
	if !state.Active() || !state.NoCheckout || state.Candidate == "" {
		t.Fatalf("missing bisect state %+v", state)
	}
	if f.git("rev-parse", "HEAD") != bad {
		t.Fatal("no-checkout bisect changed HEAD")
	}
	stale := f.read()
	if err := f.run(domain.RepositoryOperation{Kind: domain.BisectGood, Source: state.Candidate}); err != nil {
		t.Fatal(err)
	}
	if err := f.g.ExecuteRepositoryOperation(context.Background(), stale, domain.RepositoryOperation{Kind: domain.BisectBad, Source: state.Candidate}); !errors.Is(err, ErrStaleIndex) {
		t.Fatalf("stale classification = %v", err)
	}
	for i := 0; i < 10; i++ {
		repo := f.read()
		if repo.Bisect.FirstBad != "" {
			break
		}
		if err := f.run(domain.RepositoryOperation{Kind: domain.BisectGood, Source: repo.Bisect.Candidate}); err != nil {
			t.Fatal(err)
		}
	}
	if f.read().Bisect.FirstBad != bad {
		t.Fatalf("first bad = %+v", f.read().Bisect)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetBisect}); err != nil {
		t.Fatal(err)
	}
	if f.read().Bisect.Active() || f.git("rev-parse", "HEAD") != bad {
		t.Fatal("bisect reset changed HEAD or left session")
	}
}

func TestAdvancedBisectSkipCancelAndInvalidRange(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	good := f.git("rev-parse", "HEAD")
	f.git("commit", "--allow-empty", "-m", "middle")
	f.git("commit", "--allow-empty", "-m", "bad")
	bad := f.git("rev-parse", "HEAD")
	if err := f.run(domain.RepositoryOperation{Kind: domain.StartBisect, Source: good, Target: bad}); err == nil {
		t.Fatal("reversed range accepted")
	}
	if f.read().Bisect.Active() {
		t.Fatal("invalid range left session")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.StartBisect, Source: bad, Target: good}); err != nil {
		t.Fatal(err)
	}
	state := f.read().Bisect
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ExecuteRepositoryOperation(ctx, f.read(), domain.RepositoryOperation{Kind: domain.BisectGood, Source: state.Candidate}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	// All remaining candidates are either bad or skipped; Git may return its ambiguous-result exit code.
	_ = f.run(domain.RepositoryOperation{Kind: domain.BisectSkip, Source: state.Candidate})
	if !f.read().Bisect.Active() {
		t.Fatal("skip lost bisect recovery state")
	}
	if !f.read().Bisect.Ambiguous {
		t.Fatal("skipped range did not report an inconclusive result")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetRevision, Source: bad, Mode: "Soft"}); err == nil {
		t.Fatal("reset allowed during bisect")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetBisect}); err != nil {
		t.Fatal(err)
	}
}

func TestAdvancedResetDetectsHiddenTrackedEdit(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			f := newWorkflow(t)
			f.git("update-index", flag, "--", "文件 name.txt")
			snapshot := f.read()
			f.write("文件 name.txt", "external precious edit\n")
			if err := f.g.ExecuteRepositoryOperation(context.Background(), snapshot, domain.RepositoryOperation{Kind: domain.ResetRevision, Source: snapshot.Head, Mode: "Hard"}); !errors.Is(err, ErrStaleIndex) {
				t.Fatalf("hidden tracked edit accepted: %v", err)
			}
			data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
			if string(data) != "external precious edit\n" {
				t.Fatal("hidden tracked edit lost")
			}
		})
	}
}

func TestAdvancedWorktreeRemovalPreservesIgnoredFiles(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	path := filepath.Join(t.TempDir(), "linked")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CreateWorktree, Source: f.read().Head, Mode: "Detached", Path: path}); err != nil {
		t.Fatal(err)
	}
	excludes := filepath.Join(t.TempDir(), "excludes")
	if err := os.WriteFile(excludes, []byte("precious.local\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f.git("config", "core.excludesFile", filepath.ToSlash(excludes))
	precious := filepath.Join(path, "precious.local")
	if err := os.WriteFile(precious, []byte("precious"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RemoveWorktree, Path: path}); err == nil {
		t.Fatal("removed worktree with ignored data")
	}
	if data, err := os.ReadFile(precious); err != nil || string(data) != "precious" {
		t.Fatal("ignored file was lost", err)
	}
}

func TestAdvancedBisectExternalCheckoutRecovery(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	good := f.git("rev-parse", "HEAD")
	f.write("文件 name.txt", "middle\n")
	f.git("commit", "-am", "middle")
	f.write("文件 name.txt", "bad\n")
	f.git("commit", "-am", "bad")
	bad := f.git("rev-parse", "HEAD")
	f.git("bisect", "start", bad, good)
	f.git("update-index", "--assume-unchanged", "--", "文件 name.txt")
	f.write("文件 name.txt", "hidden precious\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetBisect}); err == nil {
		t.Fatal("external reset overwrote hidden tracked changes")
	}
	f.git("update-index", "--no-assume-unchanged", "--", "文件 name.txt")
	f.git("restore", "--worktree", "--", "文件 name.txt")
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetBisect}); err != nil {
		t.Fatal(err)
	}
	if f.git("rev-parse", "HEAD") != bad {
		t.Fatal("external reset did not restore original checkout")
	}
}

func TestAdvancedBisectRestrictedSearch(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	good := f.git("rev-parse", "HEAD")
	for _, text := range []string{"middle\n", "bad\n"} {
		f.write("文件 name.txt", text)
		f.git("commit", "-am", "step")
	}
	bad := f.git("rev-parse", "HEAD")
	f.git("bisect", "start", "--no-checkout", bad, good, "--", "文件 name.txt")
	names, _ := os.ReadFile(filepath.Join(f.root, ".git", "BISECT_NAMES"))
	t.Logf("restricted names = %q", names)
	if f.read().Bisect.Supported {
		t.Fatal("path-limited external search accepted as an unrestricted search")
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.ResetBisect}); err != nil {
		t.Fatal(err)
	}
}

func TestAdvancedStashRefusesHiddenTrackedChanges(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("visible.txt", "base\n")
	f.git("add", ".")
	f.git("commit", "-m", "extra file")
	f.git("update-index", "--assume-unchanged", "--", "文件 name.txt")
	f.write("文件 name.txt", "precious hidden edit\n")
	f.write("visible.txt", "visible edit\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.SaveStash, Message: "save visible"}); err == nil {
		t.Fatal("Stash accepted hidden tracked changes")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "precious hidden edit\n" {
		t.Fatal("Stash lost hidden tracked changes")
	}
}
