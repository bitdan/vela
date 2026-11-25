package ui

import (
	"context"
	"errors"
	native "github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"strings"
	"testing"
	"time"
	"vela/internal/application"
	"vela/internal/domain"
)

func inlineFixture(t *testing.T) *Workspace {
	a := New(application.New(), application.DefaultLayout())
	r := sessionRepository(t.TempDir())
	r.Head = strings.Repeat("a", 40)
	a.Model.Switch(a.installRepository(r))
	a.SaveRepositoryFile = func(context.Context, domain.Repository, domain.File, string, func(domain.Repository, domain.File, error, error)) {
	}
	a.openCode("project", domain.File{Path: "中文 file.txt", New: "first\nsecond\n", DiskVersion: "v1"})
	a.WatchEditorActivity = func(context.Context, func()) {}
	a.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) { done(f, nil) }
	return a
}
func inlineDoc(a *Workspace) domain.BlameDocument {
	return domain.BlameDocument{Path: "中文 file.txt", Revision: a.Model.Active().Repository.Head, Text: "first\nsecond\n", Lines: []domain.BlameLine{{Commit: strings.Repeat("b", 40), Author: "Alice", Summary: "first commit", Line: 1, AuthoredAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)}, {Commit: strings.Repeat("c", 40), Author: "Bob", Summary: "second commit", Line: 2, AuthoredAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)}}}
}
func TestInlineBlameCurrentLineAndDetails(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			a := inlineFixture(t)
			a.Layout.Dark = theme == "dark"
			reads := 0
			a.ReadRepositoryBlame = func(_ context.Context, _, _, _ string, done func(domain.BlameDocument, error)) {
				reads++
				done(inlineDoc(a), nil)
			}
			a.ReadRepositoryLog = func(_ context.Context, _ string, s domain.CommitLogSnapshot, _, _ int, done func(domain.CommitLogPage, error)) {
				done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: s.Tips[0], Author: "Bob", Body: "full message", Subject: "second commit"}}}, nil)
			}
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Inline Blame"); err != nil {
				t.Fatal(err)
			}
			if a.inlineBlame.line.Author != "Alice" {
				t.Fatalf("missing first attribution: %+v", a.inlineBlame)
			}
			code := a.activeDocument().code
			if err := tt.Click("Source editor"); err != nil {
				t.Fatal(err)
			}
			tt.Key(native.Ctrl, native.KeyHome)
			tt.Key(0, native.KeyDown)
			a.tickInlineBlame()
			tt.Frame()
			if a.inlineBlame.line.Author != "Bob" || reads != 1 {
				t.Fatalf("cursor move: pos=%+v line=%+v reads=%d", code.CursorPos(), a.inlineBlame.line, reads)
			}
			exportSessionScreenshot(t, tt, "inline-blame-"+theme)
			if err := tt.Click("Inline Blame Commit Details"); err != nil {
				t.Fatal(err)
			}
			tt.Frame()
			if a.inlineBlame.details == nil || a.inlineBlame.details.Body != "full message" {
				t.Fatal("missing fixed commit details")
			}
			exportSessionScreenshot(t, tt, "inline-blame-details-"+theme)
			a.run(closeInlineDetails, commandTarget{})
			a.run(toggleInlineBlame, commandTarget{})
			if a.inlineBlame.enabled {
				t.Fatal("toggle off failed")
			}
			_ = code
		})
	}
}
func TestInlineBlameStaleAndDirtySuppression(t *testing.T) {
	a := inlineFixture(t)
	var done func(domain.BlameDocument, error)
	calls := 0
	a.ReadRepositoryBlame = func(_ context.Context, _, _, _ string, f func(domain.BlameDocument, error)) { calls++; done = f }
	a.run(toggleInlineBlame, commandTarget{})
	old := done
	b := a.activeDocument().code.Buffer
	b.Set("edit" + b.String())
	a.tickInlineBlame()
	old(inlineDoc(a), nil)
	if a.inlineBlame.line.Commit != "" {
		t.Fatal("late result attributed dirty text")
	}
	b.Undo()
	a.tickInlineBlame()
	done(domain.BlameDocument{}, errors.New("unavailable"))
	a.tickInlineBlame()
	if calls != 2 || a.inlineBlame.line.Commit != "" {
		t.Fatal("error retried or stale attribution retained")
	}
	a.run(toggleInlineBlame, commandTarget{})
	a.run(toggleInlineBlame, commandTarget{})
	old = done
	a.activeDocument().code.SetSelections(editor.Selection{Head: 6, Anchor: 6})
	a.selectRepository(0)
	old(inlineDoc(a), nil)
	if a.inlineBlame.line.Commit != "" {
		t.Fatal("navigation accepted late result")
	}
}
func TestInlineBlameRequiresExactDiskAndHeadText(t *testing.T) {
	for _, disk := range []string{"changed\n", "first\nsecond", ""} {
		t.Run(disk, func(t *testing.T) {
			a := inlineFixture(t)
			a.ReadRepositoryBlame = func(_ context.Context, _, _, _ string, done func(domain.BlameDocument, error)) {
				done(inlineDoc(a), nil)
			}
			a.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) {
				f.New = disk
				done(f, nil)
			}
			a.run(toggleInlineBlame, commandTarget{})
			if a.inlineBlame.line.Commit != "" {
				t.Fatal("mismatched disk attributed")
			}
		})
	}
}

func TestInlineBlameEmptyErrorSplitAndLateUndo(t *testing.T) {
	a := inlineFixture(t)
	var finish func(domain.BlameDocument, error)
	a.ReadRepositoryBlame = func(_ context.Context, _, _, _ string, done func(domain.BlameDocument, error)) { finish = done }
	a.run(toggleInlineBlame, commandTarget{})
	old := finish
	b := a.activeDocument().code.Buffer
	b.Set("other")
	b.Undo()
	old(inlineDoc(a), nil)
	if a.inlineBlame.doc != nil {
		t.Fatal("edit and undo accepted old buffer version")
	}
	a.tickInlineBlame()
	finish(inlineDoc(a), nil)
	code := a.activeDocument().code
	code.GoTo(editor.Pos{Line: 2})
	a.tickInlineBlame()
	if a.inlineBlame.line.Commit != "" {
		t.Fatal("virtual EOF line attributed")
	}
	a.openCode("project", domain.File{Path: "split.txt", New: "split", DiskVersion: "split"})
	split := *a.activeDocument()
	a.repo().split = &split
	a.focusedEditor = split.code
	a.tickInlineBlame()
	finish(domain.BlameDocument{}, errors.New("missing"))
	if a.inlineBlame.line.Commit != "" || !strings.Contains(a.inlineBlame.status, "missing") {
		t.Fatal("split error retained previous attribution")
	}
	a.StopEditorActivity()
	if a.inlineBlame.identity.code != nil {
		t.Fatal("close retained editor")
	}
}
