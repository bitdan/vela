package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters/gitcli"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestOpenLocalRepositoryAndPreview(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sample-repository")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	git := gitcli.New()
	if _, err := git.Initialize(context.Background(), dir, "main"); err != nil {
		t.Fatal(err)
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "preview.txt"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("committed content\n")
	command := exec.Command("git", "-c", "user.name=Vela Test", "-c", "user.email=vela@example.test", "-c", "commit.gpgsign=false", "add", "--", "preview.txt")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("stage fixture: %s: %v", output, err)
	}
	command = exec.Command("git", "-c", "user.name=Vela Test", "-c", "user.email=vela@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "Add preview fixture")
	command.Dir = dir
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("commit fixture: %s: %v", output, err)
	}
	write("worktree content\n")
	a := New(application.New(), application.DefaultLayout())
	a.PickRepositoryDirectory = func(done func(string, error)) { done(dir, nil) }
	a.ReadRepository = func(ctx context.Context, dir string, done func(domain.Repository, error)) {
		repo, err := git.ReadRepository(ctx, dir)
		done(repo, err)
	}
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, file domain.File, done func(domain.File, error)) {
		resolved, err := git.ReadFile(ctx, root, source, file)
		done(resolved, err)
	}
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Switch repository"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Open Local Repository…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	repo := a.Model.Active().Repository
	if repo.Root == "" || len(repo.Working) != 1 || repo.Working[0].Path != "preview.txt" {
		t.Fatalf("window did not load the real repository: %+v", repo.Working)
	}
	target := commandTarget{source: "working", file: repo.Working[0]}
	if err := tt.Click("preview.txt"); err != nil {
		t.Fatal(err)
	}
	if preview := a.repo().preview; preview == nil || preview.file.Old != "committed content\n" || preview.file.New != "worktree content\n" {
		t.Fatal("real working diff did not open")
	}
	if !tt.HasText("Add preview fixture") {
		t.Fatal("window did not render the real commit log")
	}
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writeScreenshot(t, tt, filepath.Join(dir, "phase2-dark.png"))
		a.Layout.Dark = false
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, "phase2-light.png"))
	}
	a.run(fileHistory, target)
	tt.Frame()
	if a.repo().logHistoryPath != "preview.txt" || !tt.HasText("Add preview fixture") {
		t.Fatal("file history did not match the real commit")
	}
	a.run(openCode, target)
	entry := a.activeDocument()
	if entry == nil || entry.code == nil || !entry.code.ReadOnly {
		t.Fatal("working file should open read-only")
	}
	if a.disabledFor(stageChanges, target) == "" {
		t.Fatal("real repository should not offer staging")
	}
	a.run(refreshChanges, commandTarget{})
	if entry := a.activeDocument(); entry == nil || entry.path != "preview.txt" {
		t.Fatal("refresh lost the open editor tab")
	}
	a.UpdateRepositoryIndex = func(repo domain.Repository, stage bool, files []domain.File, done func(domain.Repository, error, error)) {
		err := git.UpdateIndex(context.Background(), repo, stage, files)
		updated, refreshErr := git.ReadRepository(context.Background(), repo.Root)
		done(updated, err, refreshErr)
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
	}
	tt.Frame()
	click("preview.txt")
	click("Stage Changes")
	if repo := a.Model.Active().Repository; len(repo.Staged) != 1 || len(repo.Working) != 0 {
		t.Fatalf("stage button did not apply and refresh changes: staged %v working %v", repo.Staged, repo.Working)
	}
	write("later working content\n")
	a.run(refreshChanges, commandTarget{})
	tt.Frame()
	staged := a.Model.Active().Repository.Staged[0]
	a.run(unstageChanges, commandTarget{source: "staged", file: staged})
	tt.Frame()
	if a.indexConfirmation == nil {
		t.Fatal("unstaging with separate working changes must ask for confirmation")
	}
	if screenshots := os.Getenv("VELA_SCREENSHOTS"); screenshots != "" {
		writeScreenshot(t, tt, filepath.Join(screenshots, "phase3-unstage-confirmation.png"))
	}
	click("Cancel")
	if a.indexConfirmation != nil || len(a.Model.Active().Repository.Staged) != 1 {
		t.Fatal("cancel changed the index or left the dialog open")
	}
	a.run(unstageChanges, commandTarget{source: "staged", file: staged})
	tt.Frame()
	click("Unstage")
	if repo := a.Model.Active().Repository; len(repo.Staged) != 0 || len(repo.Working) != 1 {
		t.Fatalf("unstage did not apply and refresh changes: staged %v working %v", repo.Staged, repo.Working)
	}
	if content, err := os.ReadFile(filepath.Join(dir, "preview.txt")); err != nil || string(content) != "later working content\n" {
		t.Fatalf("unstage changed worktree: %q, %v", content, err)
	}
}

func TestRepositorySwitchDiscardsStaleReads(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	var finish func(domain.Repository, error)
	var request context.Context
	a.ReadRepository = func(ctx context.Context, dir string, done func(domain.Repository, error)) {
		request, finish = ctx, done
	}
	a.loadRepository("first", false)
	first, firstContext := finish, request
	a.loadRepository("second", false)
	if !errors.Is(firstContext.Err(), context.Canceled) {
		t.Fatal("replaced request was not canceled")
	}
	finish(domain.Repository{Root: "second", Name: "second", CurrentBranch: "main"}, nil)
	first(domain.Repository{Root: "first", Name: "first", CurrentBranch: "main"}, nil)
	if a.Model.Active().Repository.Root != "second" || len(a.Model.Sessions) != 3 {
		t.Fatal("stale repository changed the workspace")
	}
	var finishFile func(domain.File, error)
	var fileContext context.Context
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, file domain.File, done func(domain.File, error)) {
		fileContext, finishFile = ctx, done
	}
	file := domain.File{Path: "late.txt", Status: "?"}
	a.run(previewDiff, commandTarget{source: "working", file: file})
	a.run(selectRepo, commandTarget{index: 1})
	finishFile(file, nil)
	if !errors.Is(fileContext.Err(), context.Canceled) || a.repo().preview != nil || a.Model.Active().Repository.Name != "sandbox" {
		t.Fatal("late file preview affected a different repository")
	}
}
