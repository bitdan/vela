package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/adapters"
	"vela/internal/adapters/gitcli"
	"vela/internal/application"
	"vela/internal/domain"
)

func sessionRepository(root string) domain.Repository {
	return domain.Repository{Root: root, Name: "same-name", CurrentBranch: "main", Head: "abc",
		Project: []domain.File{{Path: "你好 file.go"}}, Working: []domain.File{{Path: "你好 file.go", Status: "M"}},
		History: []domain.Commit{{Hash: "abc", Subject: "Recent commit"}, {Hash: "def", Subject: "Earlier commit"}}}
}

func TestRealSessionSnapshotRoundtrip(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	root := filepath.Join(t.TempDir(), "仓库 one")
	a.Model.Switch(a.installRepository(sessionRepository(root)))
	a.Model.Active().Message = "commit draft"
	a.Model.Active().Working, a.Model.Active().Staged = "你好 file.go", "staged.txt"
	a.Model.Active().Commit, a.Model.Active().Historical = 1, "old.go"
	a.Model.Active().Branch = "feature/selected"
	a.openCode("project", domain.File{Path: "你好 file.go", New: "package main\n"})
	a.repo().tabs[0].code.Select(8, 8)
	a.repo().tabs[0].pinned = true
	snapshot := a.SessionSnapshot()
	file := filepath.Join(t.TempDir(), "workspace.json")
	if err := adapters.SaveSession(file, snapshot); err != nil {
		t.Fatal(err)
	}
	loaded, err := adapters.LoadSession(file)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveRoot != root {
		t.Fatalf("real active root lost: %+v", loaded)
	}
	var saved *application.RepositorySnapshot
	for i := range loaded.Repositories {
		if loaded.Repositories[i].Root == root {
			saved = &loaded.Repositories[i]
		}
	}
	if saved == nil || saved.Message != "commit draft" || saved.Commit != "def" || saved.Working != "你好 file.go" || saved.Staged != "staged.txt" || saved.Historical != "old.go" || saved.Branch != "feature/selected" || len(saved.Tabs) != 1 || saved.Tabs[0].Cursor != 8 || !saved.Tabs[0].Pinned {
		t.Fatalf("real repository state lost: %+v", saved)
	}
}

func TestRestoreRealSessionAndRecentMenu(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			root, other := filepath.Join(t.TempDir(), "one"), filepath.Join(t.TempDir(), "two")
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			a.RestoreSession(application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{
				{Name: "same-name", Root: root, Message: "restored draft", Branch: "main", Commit: "def", Working: "你好 file.go",
					ActiveTab: "code:project:你好 file.go", Tabs: []application.TabSnapshot{{Source: "project", Path: "你好 file.go", Code: true, Cursor: 8, Pinned: true}}},
				{Name: "same-name", Root: other},
			}})
			var reads []string
			a.ReadRepository = func(ctx context.Context, path string, done func(domain.Repository, error)) {
				reads = append(reads, path)
				done(sessionRepository(path), nil)
			}
			a.ReadRepositoryFile = func(ctx context.Context, root, source string, file domain.File, done func(domain.File, error)) {
				file.New = "package main\n"
				done(file, nil)
			}
			a.StartSessionRestore()
			if len(reads) != 1 || reads[0] != root || a.Model.Active().Repository.Root != root || a.Model.Active().Message != "restored draft" || a.Model.Active().Commit != 1 {
				t.Fatalf("active session not restored: %+v", a.Model.Active())
			}
			entry := a.activeDocument()
			if entry == nil || entry.code == nil || !entry.code.ReadOnly || entry.code.Cursor() != 8 || !entry.pinned {
				t.Fatalf("tab not restored: %+v", entry)
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Switch repository"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("same-name · " + other); err != nil {
				t.Fatal(err)
			}
			if a.Model.Active().Repository.Root != other || len(reads) != 2 {
				t.Fatal("recent repository did not load")
			}
			a.run(selectRepo, commandTarget{index: 2})
			if a.Model.Active().Message != "restored draft" {
				t.Fatal("same-name sessions were merged")
			}
			exportSessionScreenshot(t, tt, map[bool]string{false: "light", true: "dark"}[dark])
		})
	}
}

func TestRestoreMissingRepositoryAndStaleCallbacks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	saved := application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{{Name: "missing", Root: root, Message: "keep me"}}}
	a := New(application.New(), application.DefaultLayout())
	a.RestoreSession(saved)
	a.ReadRepository = func(ctx context.Context, dir string, done func(domain.Repository, error)) {
		done(domain.Repository{}, errors.New("directory no longer exists"))
	}
	a.StartSessionRestore()
	if a.Model.Active().Repository.Root != "" {
		t.Fatal("failed restore displaced usable workspace")
	}
	found := false
	for _, repo := range a.SessionSnapshot().Repositories {
		if repo.Root == root && repo.Message == "keep me" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing repository was erased instead of remaining retryable")
	}
	var finish func(domain.Repository, error)
	var request context.Context
	a.ReadRepository = func(ctx context.Context, dir string, done func(domain.Repository, error)) {
		request, finish = ctx, done
	}
	a.run(openRecentRepository, commandTarget{source: root})
	a.run(selectRepo, commandTarget{index: 1})
	finish(sessionRepository(root), nil)
	if request.Err() != context.Canceled || a.Model.Active().Repository.Name != "sandbox" {
		t.Fatal("late restore stole user selection")
	}
}

func exportSessionScreenshot(t *testing.T, tt *native.Tester, name string) {
	t.Helper()
	// The shared helper follows the project's opt-in screenshot convention.
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, "session-"+name+".png"))
	}
}

func TestRestoreRealTabsReadFreshGitContent(t *testing.T) {
	root := t.TempDir()
	g := gitcli.New()
	if _, err := g.Initialize(context.Background(), root, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=Vela Test", "-c", "user.email=vela@example.test", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if err := os.WriteFile(filepath.Join(root, "read me.txt"), []byte("committed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial")
	hash := git("rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(root, "read me.txt"), []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	if err := os.WriteFile(filepath.Join(root, "read me.txt"), []byte("fresh disk\n"), 0600); err != nil {
		t.Fatal(err)
	}
	a := New(application.New(), application.DefaultLayout())
	a.RestoreSession(application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{{Root: root, Name: "real",
		Tabs: []application.TabSnapshot{{Source: "project", Path: "read me.txt", Code: true}, {Source: "revision:" + hash, Path: "read me.txt", Code: true}, {Source: "staged", Path: "read me.txt"}, {Source: "local:" + hash, Path: "read me.txt"}}}}})
	a.ReadRepository = func(ctx context.Context, root string, done func(domain.Repository, error)) {
		r, e := g.ReadRepository(ctx, root)
		done(r, e)
	}
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		f, e := g.ReadFile(ctx, root, source, f)
		done(f, e)
	}
	a.StartSessionRestore()
	if len(a.repo().tabs) != 4 {
		t.Fatalf("restored %d/4 tab kinds, pending %+v, status %s", len(a.repo().tabs), a.repo().pendingTabs, a.status+" loaded="+a.Model.Active().Repository.Root+" requested="+root)
	}
	if a.repo().tabs[0].file.New != "fresh disk\n" || a.repo().tabs[1].file.New != "committed\n" || a.repo().tabs[2].file.New != "staged\n" || a.repo().tabs[3].file.New != "fresh disk\n" {
		t.Fatal("tabs did not use their own current/revision/index content")
	}
}

func TestSessionTabRestoreCancelsOnNavigation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "real")
	a := New(application.New(), application.DefaultLayout())
	a.RestoreSession(application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{{Root: root, Name: "real", Tabs: []application.TabSnapshot{{Path: "你好 file.go", Source: "project", Code: true}}}}})
	a.ReadRepository = func(ctx context.Context, root string, done func(domain.Repository, error)) {
		done(sessionRepository(root), nil)
	}
	var request context.Context
	var finish func(domain.File, error)
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		request, finish = ctx, done
	}
	a.StartSessionRestore()
	a.run(selectRepo, commandTarget{index: 1})
	finish(domain.File{Path: "你好 file.go", New: "late"}, nil)
	if request.Err() != context.Canceled || a.Model.Active().Repository.Name != "sandbox" || len(a.repos[2].tabs) != 0 {
		t.Fatal("stale tab restore affected navigation")
	}
	if len(a.repos[2].pendingTabs) != 1 {
		t.Fatal("canceled tab identity was lost")
	}
}

func TestRealSessionSelectionAndDiffLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "real")
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(root)
	repo.Working = append(repo.Working, domain.File{Path: "second.txt", Status: "M"})
	a.Model.Switch(a.installRepository(repo))
	a.Model.Active().Working = "second.txt"
	a.repo().workingChosen.Add("你好 file.go")
	a.repo().workingChosen.Add("second.txt")
	a.openDiff("working", domain.File{Path: "second.txt", Status: "M", Old: "old", New: "new"})
	a.repo().tabs[0].diff.Layout = editor.DiffUnified
	saved := a.SessionSnapshot()
	b := New(application.New(), application.DefaultLayout())
	b.RestoreSession(saved)
	b.ReadRepository = func(ctx context.Context, path string, done func(domain.Repository, error)) { done(repo, nil) }
	b.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		f.Old, f.New = "old", "new"
		done(f, nil)
	}
	b.StartSessionRestore()
	if !b.repo().workingChosen.Has("你好 file.go") || !b.repo().workingChosen.Has("second.txt") || b.repo().tabs[0].diff.Layout != editor.DiffUnified {
		t.Fatal("multi-selection or diff layout not restored")
	}
}

func TestRestoredStagedDiffUsesIndexStatus(t *testing.T) {
	root := filepath.Join(t.TempDir(), "real")
	repo := sessionRepository(root)
	repo.Working = []domain.File{{Path: "a.txt", Status: "D"}}
	repo.Staged = []domain.File{{Path: "a.txt", Status: "M", OldPath: "old.txt"}}
	a := New(application.New(), application.DefaultLayout())
	a.RestoreSession(application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{{Root: root, Tabs: []application.TabSnapshot{{Source: "staged", Path: "a.txt"}}}}})
	a.ReadRepository = func(ctx context.Context, path string, done func(domain.Repository, error)) { done(repo, nil) }
	var actual domain.File
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		actual = f
		done(f, nil)
	}
	a.StartSessionRestore()
	if actual.Status != "M" || actual.OldPath != "old.txt" {
		t.Fatalf("staged tab used working metadata: %+v", actual)
	}
}

func TestRestoringTabsYieldToTabNavigation(t *testing.T) {
	for _, closeAll := range []bool{false, true} {
		t.Run(map[bool]string{false: "click", true: "close-all"}[closeAll], func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "real")
			repo := sessionRepository(root)
			repo.Project = []domain.File{{Path: "first.txt"}, {Path: "second.txt"}}
			a := New(application.New(), application.DefaultLayout())
			a.RestoreSession(application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{{Root: root, ActiveTab: "code:project:second.txt", Tabs: []application.TabSnapshot{{Source: "project", Path: "first.txt", Code: true}, {Source: "project", Path: "second.txt", Code: true}}}}})
			a.ReadRepository = func(ctx context.Context, path string, done func(domain.Repository, error)) { done(repo, nil) }
			var finish func(domain.File, error)
			a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
				if f.Path == "first.txt" {
					done(f, nil)
				} else {
					finish = done
				}
			}
			a.StartSessionRestore()
			snapshot := a.SessionSnapshot()
			if snapshot.Repositories[2].ActiveTab != "code:project:second.txt" {
				t.Fatal("pending active identity lost")
			}
			tt := native.NewTester(a.View, 1440, 900)
			if closeAll {
				a.run(closeAllTabs, commandTarget{})
			} else if err := tt.Click("first.txt"); err != nil {
				t.Fatal(err)
			}
			finish(domain.File{Path: "second.txt", New: "late"}, nil)
			if closeAll {
				if len(a.repo().tabs) != 0 || len(a.repo().pendingTabs) != 0 {
					t.Fatal("closed tabs returned from restoration")
				}
			} else if a.activeDocument().path != "first.txt" || len(a.repo().tabs) != 1 {
				t.Fatal("restore callback stole tab focus")
			}
		})
	}
}
