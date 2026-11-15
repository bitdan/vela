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

func TestRealShelfConfirmAndRepositoryIsolation(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Working = []domain.File{{Path: "a.go", Status: "M"}}
	a.Model.Switch(a.installRepository(repo))
	var operation domain.RepositoryOperation
	var done func(domain.Repository, error, error)
	a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, callback func(domain.Repository, error, error)) {
		operation = op
		done = callback
	}
	a.run(shelve, commandTarget{source: "working", file: repo.Working[0]})
	if a.repositoryOperation == nil || a.repositoryOperation.op.Kind != domain.SaveShelf || done != nil {
		t.Fatal("shelve did not wait for confirmation")
	}
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Shelf name"); err != nil {
		t.Fatal(err)
	}
	tt.Key(native.Ctrl, native.KeyA)
	tt.Type("Saved work")
	if err := tt.Click("Run Shelve"); err != nil {
		t.Fatal(err)
	}
	if operation.Kind != domain.SaveShelf || len(operation.Paths) != 1 || operation.Paths[0] != "a.go" || operation.Message != "Saved work" {
		t.Fatalf("wrong save operation: %+v", operation)
	}
	a.run(selectRepo, commandTarget{index: 0})
	repo.Working = nil
	repo.Shelf = []domain.Shelf{{ID: strings.Repeat("a", 32), OID: strings.Repeat("b", 40), Title: "Saved work"}}
	done(repo, nil, nil)
	if a.Model.Index != 0 {
		t.Fatal("old shelf callback changed active repository")
	}
	found := false
	for _, s := range a.Model.Sessions {
		if s.Repository.Root == repo.Root {
			found = len(s.Repository.Shelf) == 1
		}
	}
	if !found {
		t.Fatal("old repository shelf refresh lost")
	}
}

func TestRealShelfDirtyProtectionAndTargetIdentity(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	repo.Working = []domain.File{{Path: "a.go", Status: "M"}}
	repo.Shelf = []domain.Shelf{{ID: strings.Repeat("a", 32), OID: strings.Repeat("b", 40), Title: "Saved", Files: []domain.File{{Path: "a.go"}}}}
	a.Model.Switch(a.installRepository(repo))
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	a.openCode("project", domain.File{Path: "a.go", New: "disk"})
	a.activeDocument().code.Buffer.Set("unsaved")
	if a.disabledFor(shelve, commandTarget{source: "working", file: repo.Working[0]}) == "" {
		t.Fatal("shelving dirty editor enabled")
	}
	if a.disabledFor(unshelve, commandTarget{index: 0}) == "" {
		t.Fatal("unshelving dirty editor enabled")
	}
	a.activeDocument().code.Buffer.MarkSaved()
	a.Model.Active().Repository.Working = nil
	target := commandTarget{index: 0, shelfID: repo.Shelf[0].ID, shelfOID: repo.Shelf[0].OID}
	a.run(unshelve, target)
	if a.repositoryOperation == nil || a.repositoryOperation.op.Kind != domain.ApplyShelf || !a.repositoryOperation.op.RestoreIndex {
		t.Fatal("real unshelve not configured")
	}
	a.repositoryOperation = nil
	a.Model.Active().Repository.Shelf[0].OID = strings.Repeat("c", 40)
	if a.disabledFor(deleteShelf, target) == "" {
		t.Fatal("stale menu shelf identity accepted")
	}
}

func TestShelfChangelistScreenshots(t *testing.T) {
	dir := os.Getenv("VELA_SCREENSHOTS")
	if dir == "" {
		t.Skip("set VELA_SCREENSHOTS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"dark", "light"} {
		layout := application.DefaultLayout()
		layout.Dark = name == "dark"
		a := New(application.New(), layout)
		repo := sessionRepository(t.TempDir())
		repo.IndexVersion = "index"
		repo.Working = []domain.File{{Path: "feature.go", Status: "M", Old: "package main\n", New: "package main\n\nfunc feature() {}\n"}}
		repo.Staged = []domain.File{{Path: "prepared.go", Status: "A", New: "package main\n"}}
		repo.Shelf = []domain.Shelf{{ID: strings.Repeat("a", 32), OID: strings.Repeat("b", 40), Title: "Saved feature work", Files: []domain.File{{Path: "feature.go", Status: "M"}, {Path: "assets/icon.bin", Status: "M"}}}, {ID: strings.Repeat("c", 32), OID: strings.Repeat("d", 40), Title: "Earlier experiment", Deleted: true, Files: []domain.File{{Path: "notes.txt", Status: "A"}}}}
		a.Model.Switch(a.installRepository(repo))
		a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		}
		lists := &a.Model.Active().Changelists
		id, err := lists.Create("Feature work")
		if err != nil {
			t.Fatal(err)
		}
		if err := lists.Move([]string{"feature.go"}, id); err != nil {
			t.Fatal(err)
		}
		lists.Active = id
		a.run(filterChangelist, commandTarget{source: id})
		a.openDiff("working", repo.Working[0])
		tt := native.NewTester(a.View, 1440, 900)
		writeScreenshot(t, tt, filepath.Join(dir, name+"-changelists.png"))
		a.run(openShelf, commandTarget{})
		a.repo().shelfExpanded[0] = true
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, name+"-shelf.png"))
		a.run(shelve, commandTarget{source: "working", file: repo.Working[0]})
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, name+"-shelve-confirm.png"))
		a.repositoryOperation = nil
		a.Model.Active().Repository.Working = nil
		a.Model.Active().Repository.Staged = nil
		a.run(unshelve, commandTarget{index: 0})
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, name+"-unshelve-confirm.png"))
	}
}
