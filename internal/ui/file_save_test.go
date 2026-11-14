package ui

import (
	"context"
	"errors"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestSourceEditorSavePreservesNewerEditsAndRepository(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			repo := sessionRepository(t.TempDir())
			repo.IndexVersion = "index"
			a.Model.Switch(a.installRepository(repo))
			var finish func(domain.Repository, domain.File, error, error)
			var submitted string
			a.SaveRepositoryFile = func(ctx context.Context, snapshot domain.Repository, file domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
				submitted, finish = text, done
			}
			file := domain.File{Path: "edit.go", New: "package main\r\n", DiskVersion: "first"}
			a.openCode("project", file)
			entry := a.activeDocument()
			if entry.code.ReadOnly {
				t.Fatal("regular source stayed read-only with writer connected")
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Source editor"); err != nil {
				t.Fatal(err)
			}
			tt.Key(native.Ctrl, native.KeyA)
			tt.Type("package changed\n")
			tt.Key(native.Ctrl, native.KeyS)
			if finish == nil || !strings.Contains(submitted, "package changed") || !strings.HasSuffix(submitted, "\r\n") {
				t.Fatalf("Ctrl+S did not save original line ending: %q", submitted)
			}
			entry.code.Buffer.Set("newer edit\n")
			a.run(selectRepo, commandTarget{index: 1})
			saved := file
			saved.New, saved.DiskVersion = submitted, "second"
			finish(repo, saved, nil, nil)
			if a.Model.Active().Repository.Name != "sandbox" {
				t.Fatal("save completion switched active repo")
			}
			a.run(selectRepo, commandTarget{index: 2})
			entry = a.activeDocument()
			if !entry.code.Buffer.Modified() || entry.code.Buffer.String() != "newer edit\n" || entry.file.DiskVersion != "second" {
				t.Fatal("save completion discarded newer editor input")
			}
			a.run(saveSourceFile, commandTarget{})
			saved.New = submitted
			finish(repo, saved, errors.New("disk changed externally"), nil)
			if !entry.code.Buffer.Modified() {
				t.Fatal("failed save cleared modified state")
			}
			a.openCode("project", domain.File{Path: file.Path, New: "external text\n", DiskVersion: "external"})
			if a.activeDocument().code.Buffer.String() != "newer edit\n" {
				t.Fatal("reopen overwrote unsaved buffer")
			}
			exportSessionScreenshot(t, tt, "save-dirty-"+map[bool]string{false: "light", true: "dark"}[dark])
		})
	}
}

func TestSourceSaveMarksCleanAndKeepsHistoryReadOnly(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	a.Model.Switch(a.installRepository(repo))
	a.SaveRepositoryFile = func(ctx context.Context, r domain.Repository, f domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
		f.New, f.DiskVersion = text, "saved"
		done(r, f, nil, nil)
	}
	f := domain.File{Path: "edit.txt", New: "original", DiskVersion: "initial"}
	a.openCode("project", f)
	a.activeDocument().code.Buffer.Set("changed")
	a.run(saveSourceFile, commandTarget{})
	if a.activeDocument().code.Buffer.Modified() {
		t.Fatal("successful unchanged buffer not marked saved")
	}
	a.openCode("revision:abc", f)
	if !a.activeDocument().code.ReadOnly || a.disabledFor(saveSourceFile, commandTarget{}) == "" {
		t.Fatal("history version became writable")
	}
}

func TestSourceReloadKeepsSplitBufferAndProtectsSplitEdits(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	a.Model.Switch(a.installRepository(repo))
	var submitted string
	a.SaveRepositoryFile = func(ctx context.Context, r domain.Repository, f domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
		submitted = text
	}
	f := domain.File{Path: "split.txt", New: "original", DiskVersion: "initial"}
	a.openCode("project", f)
	id := a.activeDocument().id
	a.splitTab(id, false, false)
	f.New, f.DiskVersion = "external reload", "external"
	a.openCode("project", f)
	if a.repo().split.code.Buffer != a.activeDocument().code.Buffer || a.repo().split.file.DiskVersion != "external" {
		t.Fatal("reload detached split from primary buffer/version")
	}
	a.repo().split.code.Buffer.Set("split edit")
	a.focusedEditor = a.repo().split.code
	a.run(saveSourceFile, commandTarget{tabID: id})
	if submitted != "split edit" {
		t.Fatal("split save did not submit focused buffer")
	}
	if a.disabledFor(closeTab, commandTarget{tabID: id}) == "" {
		t.Fatal("dirty split could be lost by closing primary tab")
	}
}
