package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestUpdateShelfOnlyNativeConfirmation(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a, tt := advancedWorkspace(t)
			a.Layout.Dark = theme == "dark"
			a.Layout.VersionControl.Git.CleanWorkingTree = "Stash"
			a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
				t.Fatal("Update bypassed updater")
			}
			repo := a.Model.Active().Repository
			repo.CurrentBranch = "topic"
			repo.Refs = map[string]string{"refs/heads/topic": repo.Head}
			repo.Upstreams = map[string]domain.Upstream{"topic": {Remote: "origin", Branch: "topic"}}
			repo.Remotes = []domain.Remote{{Name: "origin"}}
			repo.Working = []domain.File{{Path: "中文 space.txt", Status: "M"}}
			a.Model.Active().Repository = repo
			calls := 0
			a.UpdateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, done func(domain.Repository, domain.UpdateResult, error, error)) {
				calls++
				if op.UpdateCleanup != "Shelve" || !op.ConfirmUpdateCleanup {
					t.Fatalf("wrong cleanup: %+v", op)
				}
				done(repo, domain.UpdateResult{Method: op.UpdateMethod, Fetched: true, Cleaned: true, Integrated: true, BackupKind: "Shelve", BackupID: "saved", BackupOID: "oid", BackupName: "Update backup"}, nil, nil)
			}
			a.run(updateProject, commandTarget{})
			tt.Frame()
			if a.repositoryOperation == nil || a.repositoryOperation.op.UpdateCleanup != "Shelve" {
				t.Fatal("legacy setting selected Stash")
			}
			if _, ok := tt.Access("Update working tree cleanup"); ok {
				t.Fatal("obsolete cleanup chooser exposed")
			}
			if button, ok := tt.Access("Run Update Branch"); !ok || !button.Disabled {
				t.Fatal("Update enabled before backup confirmation")
			}
			if err := tt.Click("Update integration method"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, native.KeyDown)
			tt.Key(0, native.KeyEnter)
			if a.repositoryOperation.op.UpdateMethod != "Rebase" {
				t.Fatal("keyboard method selection failed")
			}
			if err := tt.Click("Save tracked changes to Shelf; restore manually after Update"); err != nil {
				t.Fatal(err)
			}
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				writeScreenshot(t, tt, filepath.Join(dir, theme+"-shelf-update.png"))
			}
			if err := tt.Click("Run Update Branch"); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !strings.Contains(a.status, "Unshelve") || strings.Contains(a.status, "Stash") {
				t.Fatalf("missing manual recovery: %s", a.status)
			}
		})
	}
}

func TestShelfOnlyMenusHideStash(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	tt := native.NewTester(func(c *native.Context) {
		native.Button(c, "Git").Menu(a.mainGitMenu)
		native.Button(c, "Actions").Menu(func(m *native.Menu) { a.editorGitMenu(m, commandTarget{}) })
	}, 1440, 900)
	if err := tt.Click("Git"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Stash Changes…", "Unstash Changes…", "Drop Stash…"} {
		if err := tt.ChooseMenuItem("Uncommitted Changes", label); err == nil || !strings.Contains(err.Error(), "no item") {
			t.Fatalf("Stash entry still discoverable: %s, %v", label, err)
		}
	}
	if err := tt.ChooseMenuItem("Uncommitted Changes", commandLabel(openShelf)); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyEscape)
	tt.Key(0, native.KeyEscape)
	if err := tt.Click("Actions"); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{"Stash Changes…", "Unstash Changes…"} {
		if err := tt.ChooseMenuItem(label); err == nil || !strings.Contains(err.Error(), "no item") {
			t.Fatalf("context menu exposes %s", label)
		}
	}
}

func TestShelfOnlyGitSettings(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Layout.VersionControl.Git.CleanWorkingTree = "Stash"
	a.settings.page = "Git"
	a.run(showSettings, commandTarget{})
	tt := native.NewTester(a.View, 1440, 900)
	if _, ok := tt.Access("Clean working tree using"); ok {
		t.Fatal("settings still offer Stash cleanup")
	}
	if !tt.HasText("Working tree cleanup: Shelf (restore manually with Unshelve)") {
		t.Fatal("Shelf recovery policy missing")
	}
}
