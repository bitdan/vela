package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestShowBlameReadOnlyAndRestorable(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			repo := sessionRepository(t.TempDir())
			repo.Head = strings.Repeat("a", 40)
			repo.History[0].Hash = repo.Head
			a.Model.Switch(a.installRepository(repo))
			var source string
			a.ReadRepositoryBlame = func(ctx context.Context, root, revision, path string, done func(domain.BlameDocument, error)) {
				source = revision
				done(domain.BlameDocument{Path: path, Revision: revision, Lines: []domain.BlameLine{{Commit: strings.Repeat("b", 40), Author: "Test Author", AuthoredAt: time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC), Line: 1, Text: "package main"}}}, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.RightClick("你好 file.go"); err != nil {
				t.Fatal(err)
			}
			if err := tt.ChooseMenuItem("Show Blame"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			entry := a.activeDocument()
			if entry == nil || entry.code == nil || !entry.code.ReadOnly || !strings.HasPrefix(entry.source, "blame:") || source != repo.Head || !strings.Contains(entry.file.New, "Test Author") || !strings.Contains(entry.file.New, "package main") {
				t.Fatalf("blame not opened read-only: %+v", entry)
			}
			if len(a.searchDocuments(false)) != 1 {
				t.Fatal("Blame attribution polluted source search")
			}
			exportSessionScreenshot(t, tt, "blame-"+map[bool]string{false: "light", true: "dark"}[dark])
			snapshot := a.SessionSnapshot()
			b := New(application.New(), layout)
			b.RestoreSession(snapshot)
			b.ReadRepository = func(ctx context.Context, path string, done func(domain.Repository, error)) { done(repo, nil) }
			b.ReadRepositoryBlame = a.ReadRepositoryBlame
			b.StartSessionRestore()
			if b.activeDocument() == nil || !strings.HasPrefix(b.activeDocument().source, "blame:") {
				t.Fatal("blame tab identity did not restore")
			}
		})
	}
}

func TestBlameLateReadIsDiscarded(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.Head = strings.Repeat("a", 40)
	a.Model.Switch(a.installRepository(repo))
	var finish func(domain.BlameDocument, error)
	var request context.Context
	a.ReadRepositoryBlame = func(ctx context.Context, root, revision, path string, done func(domain.BlameDocument, error)) {
		request, finish = ctx, done
	}
	a.run(showBlame, commandTarget{source: "project", file: repo.Project[0]})
	a.run(selectRepo, commandTarget{index: 1})
	finish(domain.BlameDocument{Path: repo.Project[0].Path, Revision: repo.Head}, nil)
	if request.Err() != context.Canceled || a.Model.Active().Repository.Name != "sandbox" || a.activeDocument() != nil {
		t.Fatal("late blame polluted another repository")
	}
}
