package ui

import (
	"context"
	"io/fs"
	"testing"

	"vela/internal/application"
	"vela/internal/domain"
)

func TestDropRefreshMakesDeletedProjectFileReadOnly(t *testing.T) {
	for _, edited := range []bool{false, true} {
		a := New(application.New(), application.DefaultLayout())
		repo := sessionRepository(t.TempDir())
		a.Model.Switch(a.installRepository(repo))
		a.SaveRepositoryFile = func(context.Context, domain.Repository, domain.File, string, func(domain.Repository, domain.File, error, error)) {
		}
		file := domain.File{Path: "introduced.txt", New: "previous text", DiskVersion: "before"}
		a.openCode("project", file)
		entry := a.activeDocument()
		a.splitTab(entry.id, false, false)
		var finish func(domain.File, error)
		a.ReadRepositoryFile = func(_ context.Context, _, _ string, _ domain.File, done func(domain.File, error)) {
			finish = done
		}
		a.refreshHistoricalWriteViews(repo.Root, domain.RepositoryOperation{Kind: domain.DropCommit})
		wantText := file.New
		if edited {
			wantText = "new typing"
			entry.code.Buffer.Set(wantText)
		}
		finish(file, fs.ErrNotExist)
		for _, view := range []*tab{entry, a.repo().split} {
			if view.code.ReadOnly == edited || view.code.Buffer.String() != wantText || (view.file.DiskVersion == "") == edited {
				t.Fatalf("edited=%t: readOnly=%t, text=%q, disk version=%q", edited, view.code.ReadOnly, view.code.Buffer.String(), view.file.DiskVersion)
			}
		}
	}
}

func TestFileRefreshUpdatesCleanSharedViewsAndKeepsNewerTyping(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	a.Model.Switch(a.installRepository(repo))
	f := domain.File{Path: "file.txt", New: "discarded", Old: "base", Status: "M", DiskVersion: "old"}
	a.openCode("project", f)
	id := a.activeDocument().id
	a.splitTab(id, false, false)
	buffer := a.repo().split.code.Buffer
	a.openDiff("working", f)
	active := a.activeDocument().id
	diff := a.activeDocument().diff
	callbacks := map[string]func(domain.File, error){}
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, file domain.File, done func(domain.File, error)) {
		callbacks[source] = done
	}
	a.refreshOpenFileViews(repo.Root, f.Path)
	updated := f
	updated.New = "remaining"
	updated.DiskVersion = "fresh"
	callbacks["project"](updated, nil)
	callbacks["working"](updated, nil)
	if buffer.String() != "remaining" || a.repo().split.file.DiskVersion != "fresh" || diff.New.String() != "remaining" || a.activeDocument().id != active {
		t.Fatal("clean views stale or refresh stole focus")
	}
	a.refreshOpenFileViews(repo.Root, f.Path)
	buffer.Set("new typing")
	updated.New = "another disk change"
	callbacks["project"](updated, nil)
	if buffer.String() != "new typing" || !buffer.Modified() {
		t.Fatal("background reload overwrote newer typing")
	}
}

func TestFileRefreshIsCanceledByNextWrite(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	repo.IndexVersion = "index"
	a.Model.Switch(a.installRepository(repo))
	a.openCode("project", domain.File{Path: "file.txt", New: "before", DiskVersion: "old"})
	buffer := a.activeDocument().code.Buffer
	var finish func(domain.File, error)
	var request context.Context
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		request = ctx
		finish = done
	}
	a.refreshOpenFileViews(repo.Root, "file.txt")
	a.beginRepositoryWrite(repo.Root, "next", func() {})
	finish(domain.File{Path: "file.txt", New: "late", DiskVersion: "late"}, nil)
	if request.Err() == nil || buffer.String() != "before" {
		t.Fatal("older refresh survived next write")
	}
}

func TestFileRefreshUpdatesIndependentSplitDiffs(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	repo := sessionRepository(t.TempDir())
	a.Model.Switch(a.installRepository(repo))
	file := domain.File{Path: "file.txt", Status: "M", Old: "base", New: "before"}
	a.openDiff("working", file)
	main := a.activeDocument().diff
	a.splitTab(a.activeDocument().id, false, false)
	split := a.repo().split.diff
	a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		f.Old, f.New = "updated base", "after"
		done(f, nil)
	}
	a.refreshOpenFileViews(repo.Root, file.Path)
	if main.New.String() != "after" || split.New.String() != "after" || main.Old.String() != "updated base" || split.Old.String() != "updated base" {
		t.Fatal("same tab identity hid independent diff buffers")
	}
}

func TestFileRefreshUsesActualSourceMetadata(t *testing.T) {
	for _, item := range []struct{ source, status, old string }{{"working", "?", ""}, {"staged", "A", ""}, {"staged", "R", "old.txt"}} {
		t.Run(item.source+item.status, func(t *testing.T) {
			a := New(application.New(), application.DefaultLayout())
			repo := sessionRepository(t.TempDir())
			file := domain.File{Path: "new.txt", Status: item.status, OldPath: item.old}
			if item.source == "working" {
				repo.Working = []domain.File{file}
			} else {
				repo.Staged = []domain.File{file}
			}
			a.Model.Switch(a.installRepository(repo))
			a.openDiff(item.source, file)
			var received domain.File
			a.ReadRepositoryFile = func(ctx context.Context, root, source string, f domain.File, done func(domain.File, error)) {
				received = f
				done(f, nil)
			}
			a.refreshOpenFileViews(repo.Root, file.Path)
			if received.Status != file.Status || received.OldPath != file.OldPath {
				t.Fatalf("refresh changed Git identity %+v", received)
			}
		})
	}
}
