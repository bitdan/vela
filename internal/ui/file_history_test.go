package ui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestFileHistoryPagesAndOpensRenamedVersion(t *testing.T) {
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = dark
			a := New(application.New(), layout)
			repo := sessionRepository(t.TempDir())
			repo.Head = strings.Repeat("a", 40)
			a.Model.Switch(a.installRepository(repo))
			queries := 0
			a.ReadRepositoryFileHistory = func(ctx context.Context, root, revision, path string, offset, limit int, done func(domain.FileHistoryPage, error)) {
				queries++
				if revision != repo.Head || path != "new.txt" || limit != 100 {
					t.Fatal("history query not pinned/bounded")
				}
				if offset == 0 {
					done(domain.FileHistoryPage{Commits: []domain.Commit{{Hash: strings.Repeat("b", 40), Subject: "Renamed file", Author: "Alice", Timestamp: time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC), Files: []domain.File{{Path: "new.txt", OldPath: "old.txt", Status: "R"}}}}, HasMore: true}, nil)
				} else if offset == 1 {
					done(domain.FileHistoryPage{Commits: []domain.Commit{{Hash: strings.Repeat("c", 40), Subject: "Original file", Author: "Bob", Timestamp: time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC), Files: []domain.File{{Path: "old.txt", Status: "A"}}}}}, nil)
				} else {
					t.Fatal("wrong history offset")
				}
			}
			var source, path string
			a.ReadRepositoryFile = func(ctx context.Context, root, s string, f domain.File, done func(domain.File, error)) {
				source, path = s, f.Path
				f.New = "original content\n"
				done(f, nil)
			}
			a.run(fileHistory, commandTarget{source: "project", file: domain.File{Path: "new.txt"}})
			tt := native.NewTester(a.View, 1440, 900)
			tt.Frame()
			click := func(label string) {
				t.Helper()
				if err := tt.Click(label); err != nil {
					t.Fatal(err)
				}
			}
			click("Load More File History")
			tt.Frame()
			if queries != 2 || len(a.fileHistory.commits) != 2 {
				t.Fatal("history not paged")
			}
			click("Original file")
			tt.Frame()
			exportSessionScreenshot(t, tt, "file-history-"+map[bool]string{false: "light", true: "dark"}[dark])
			click("Open Historical Version")
			tt.Frame()
			if source != "revision:"+strings.Repeat("c", 40) || path != "old.txt" || a.activeDocument() == nil || a.activeDocument().source != source {
				t.Fatal("renamed historical version opened with wrong identity")
			}
			b := New(application.New(), layout)
			b.RestoreSession(a.SessionSnapshot())
			b.ReadRepository = func(ctx context.Context, root string, done func(domain.Repository, error)) { done(repo, nil) }
			b.ReadRepositoryFile = a.ReadRepositoryFile
			b.StartSessionRestore()
			if b.activeDocument() == nil || b.activeDocument().path != "old.txt" || b.activeDocument().source != "revision:"+strings.Repeat("c", 40) {
				t.Fatal("historical version outside loaded global log did not restore")
			}
		})
	}
}

func TestHistoryVersionOutsideLoadedLogSupportsBlame(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	a.Model.Switch(a.installRepository(repo))
	oid := strings.Repeat("c", 40)
	var revision string
	a.ReadRepositoryBlame = func(ctx context.Context, root, r, path string, done func(domain.BlameDocument, error)) {
		revision = r
		done(domain.BlameDocument{Path: path, Revision: r}, nil)
	}
	a.run(showBlame, commandTarget{source: "revision:" + oid, file: domain.File{Path: "old.txt"}})
	if revision != oid {
		t.Fatal("historical Blame unnecessarily requires OID in loaded global log")
	}
}

func TestDeletedFileHistoryOpensDiffAndKeepsGlobalLog(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.Head = strings.Repeat("a", 40)
	a.Model.Switch(a.installRepository(repo))
	oid := strings.Repeat("b", 40)
	a.ReadRepositoryFileHistory = func(ctx context.Context, root, revision, path string, offset, limit int, done func(domain.FileHistoryPage, error)) {
		done(domain.FileHistoryPage{Commits: []domain.Commit{{Hash: oid, Subject: "Delete file", Files: []domain.File{{Path: "gone.txt", Status: "D"}}}}}, nil)
	}
	var source string
	a.ReadRepositoryFile = func(ctx context.Context, root, s string, file domain.File, done func(domain.File, error)) {
		source = s
		if file.Status != "D" {
			t.Fatal("deletion identity lost")
		}
		file.Old = "deleted content"
		done(file, nil)
	}
	a.run(fileHistory, commandTarget{source: "project", file: domain.File{Path: "gone.txt"}})
	tt := native.NewTester(a.View, 1440, 900)
	tt.Frame()
	if a.disabledFor(openFileHistoryRevision, commandTarget{}) == "" {
		t.Fatal("deleted version opened as existing file")
	}
	if err := tt.Click("Delete file"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, native.KeyEnter)
	tt.Frame()
	if source != oid || a.repo().preview == nil || a.repo().preview.file.Old != "deleted content" || len(a.Model.Active().Repository.History) != len(repo.History) {
		t.Fatalf("deleted history diff failed/global log changed: source=%q preview=%t history=%d",source,a.repo().preview!=nil,len(a.Model.Active().Repository.History))
	}
}

func TestFileHistoryRetryAndCanceledLatePage(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.Head = strings.Repeat("a", 40)
	a.Model.Switch(a.installRepository(repo))
	attempt := 0
	var finish func(domain.FileHistoryPage, error)
	var request context.Context
	a.ReadRepositoryFileHistory = func(ctx context.Context, root, revision, path string, offset, limit int, done func(domain.FileHistoryPage, error)) {
		attempt++
		if attempt == 1 {
			done(domain.FileHistoryPage{}, errors.New("temporary failure"))
		} else {
			request = ctx
			finish = done
		}
	}
	a.run(fileHistory, commandTarget{source: "project", file: domain.File{Path: "file.txt"}})
	tt := native.NewTester(a.View, 1440, 900)
	tt.Frame()
	if a.fileHistory == nil || a.fileHistory.message == "" {
		t.Fatal("query failure not shown")
	}
	if err := tt.Click("Load More File History"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Close File History"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	finish(domain.FileHistoryPage{Commits: []domain.Commit{{Hash: "late", Subject: "late"}}}, nil)
	if request.Err() == nil || a.fileHistory != nil {
		t.Fatal("closed history received late page")
	}
}
