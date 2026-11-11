package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters/gitcli"
	"vela/internal/application"
)

func TestInitializeRepositoryFromGitMenu(t *testing.T) {
	dir := t.TempDir()
	a := New(application.New(), application.DefaultLayout())
	a.PickRepositoryDirectory = func(done func(string, error)) { done(dir, nil) }
	var finish func()
	a.InitializeRepository = func(ctx context.Context, directory, branch string, done func(string, error)) {
		finish = func() {
			path, err := gitcli.New().Initialize(ctx, directory, branch)
			done(path, err)
		}
	}
	tt := native.NewTester(a.View, 1440, 900)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	open := func() {
		t.Helper()
		click("Git menu")
		if err := tt.ChooseMenuItem("Initialize Git Repository…"); err != nil {
			t.Fatal(err)
		}
	}
	click("Main menu")
	open()
	if button, ok := tt.Access("Initialize"); !ok || !button.Disabled {
		t.Fatal("initialize should be disabled before a directory is selected")
	}
	click("Browse…")
	click("Initialize")
	if finish == nil || a.repositoryInit.cancel == nil {
		t.Fatal("expected an initialization in progress")
	}
	finish()
	tt.Frame()
	if a.repositoryInit != nil || !strings.Contains(a.status, "Initialized Git repository") {
		t.Fatal("successful initialization did not dismiss the dialog")
	}
	head, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil || strings.TrimSpace(string(head)) != "ref: refs/heads/main" {
		t.Fatalf("expected a real repository on main, got %q, %v", head, err)
	}
	open()
	click("Browse…")
	click("Initialize")
	finish()
	tt.Frame()
	if a.repositoryInit == nil || !strings.Contains(a.repositoryInit.message, "already inside a Git repository") {
		t.Fatal("expected the existing repository error to stay in the dialog")
	}
	click("Cancel")
	tt.Frame()
	if a.repositoryInit != nil {
		t.Fatal("cancel did not dismiss the dialog")
	}
}
