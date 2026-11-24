package ui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters"
	"vela/internal/adapters/gitcli"

	"vela/internal/application"
	"vela/internal/domain"
)

func TestProjectDraftSnapshotContainsUnsavedText(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Switch(a.installRepository(sessionRepository(t.TempDir())))
	a.openCode("project", domain.File{Path: "你好 file.go", New: "disk\r\n", DiskVersion: "v1"})
	a.activeDocument().code.Buffer.Set("unsaved 草稿\n")
	raw, err := json.Marshal(a.SessionSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "unsaved 草稿") || !strings.Contains(string(raw), "v1") {
		t.Fatalf("session lost draft: %s", raw)
	}
}

func TestProjectDraftRealDiskRoundtripAndStaleSave(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	g := gitcli.New()
	if _, err := g.Initialize(ctx, root, "main"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "草稿 file.txt")
	if err := os.WriteFile(path, []byte("original\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, err := g.ReadRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	file, err := g.ReadFile(ctx, root, "project", domain.File{Path: "草稿 file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	a := New(application.New(), application.DefaultLayout())
	a.Model.Switch(a.installRepository(repo))
	a.openCode("project", file)
	a.activeDocument().code.Buffer.Set("draft from previous start\n")
	sessionPath := filepath.Join(t.TempDir(), "workspace.json")
	if err := adapters.SaveSession(sessionPath, a.SessionSnapshot()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external change\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	saved, err := adapters.LoadSession(sessionPath)
	if err != nil {
		t.Fatal(err)
	}
	b := New(application.New(), application.DefaultLayout())
	b.RestoreSession(saved)
	b.ReadRepository = func(c context.Context, root string, done func(domain.Repository, error)) {
		r, e := g.ReadRepository(c, root)
		done(r, e)
	}
	b.ReadRepositoryFile = func(c context.Context, root, source string, f domain.File, done func(domain.File, error)) {
		f, e := g.ReadFile(c, root, source, f)
		done(f, e)
	}
	var saveErr error
	b.SaveRepositoryFile = func(c context.Context, r domain.Repository, f domain.File, text string, done func(domain.Repository, domain.File, error, error)) {
		f, saveErr = g.SaveFile(c, r, f, text)
		done(r, f, saveErr, nil)
	}
	b.StartSessionRestore()
	entry := b.activeDocument()
	if entry == nil || entry.file.New != "external change\r\n" || bufferDiskText(entry.code.Buffer) != "draft from previous start\r\n" {
		t.Fatal("draft and current disk were not kept independently")
	}
	b.run(saveSourceFile, commandTarget{})
	if saveErr == nil || !entry.modified() {
		t.Fatal("stale draft overwrote external content")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "external change\r\n" {
		t.Fatalf("disk changed: %q %v", raw, err)
	}
}

func TestProjectDraftLateRestoreKeepsPendingContent(t *testing.T) {
	root := t.TempDir()
	saved := application.WorkspaceSnapshot{ActiveRoot: root, Repositories: []application.RepositorySnapshot{{Root: root, Tabs: []application.TabSnapshot{{Source: "project", Path: "你好 file.go", Code: true, Draft: &application.EditorDraftSnapshot{Text: "draft", BaseText: "base", DiskVersion: "v1", LineEnding: "\n"}}}}}}
	a := New(application.New(), application.DefaultLayout())
	a.RestoreSession(saved)
	a.ReadRepository = func(_ context.Context, _ string, done func(domain.Repository, error)) {
		done(sessionRepository(root), nil)
	}
	var finish func(domain.File, error)
	a.ReadRepositoryFile = func(_ context.Context, _, _ string, _ domain.File, done func(domain.File, error)) { finish = done }
	a.StartSessionRestore()
	a.run(selectRepo, commandTarget{index: 1})
	finish(domain.File{Path: "你好 file.go", New: "late"}, nil)
	if a.Model.Active().Repository.Root == root || len(sessionDraftContents(a.SessionSnapshot())) != 1 {
		t.Fatal("late restore replaced navigation or erased pending draft")
	}
}

func TestProjectDraftExit(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		for _, outcome := range []string{"success", "failure", "cancel", "new-input", "discard"} {
			t.Run(theme+"/"+outcome, func(t *testing.T) {
				a := New(application.New(), application.DefaultLayout())
				a.Layout.Dark = theme == "dark"
				a.Model.Switch(a.installRepository(sessionRepository(t.TempDir())))
				a.openCode("project", domain.File{Path: "你好 file.go", New: "disk\n", DiskVersion: "v1"})
				a.activeDocument().code.Buffer.Set("kept draft\n")
				var finish func(error)
				var ctx context.Context
				var captured application.WorkspaceSnapshot
				a.SaveWorkspaceSession = func(c context.Context, s application.WorkspaceSnapshot, done func(error)) {
					ctx, captured, finish = c, s, done
				}
				closed := 0
				a.CloseWindow = func() { closed++ }
				if a.RequestClose() {
					t.Fatal("unsaved editor closed without choice")
				}
				tt := native.NewTester(a.View, 1440, 900)
				if outcome == "discard" {
					if err := tt.Click("Discard and Close"); err != nil {
						t.Fatal(err)
					}
					if closed != 1 || len(sessionDraftContents(a.SessionSnapshot())) != 0 || finish != nil {
						t.Fatal("discard persisted draft or failed to close")
					}
					return
				}
				if err := tt.Click("Keep Drafts and Close"); err != nil {
					t.Fatal(err)
				}
				if finish == nil || closed != 0 || a.RequestClose() {
					t.Fatal("closed before persistence callback")
				}
				if err := tt.Click("Discard and Close"); err == nil && closed != 0 {
					t.Fatal("discard raced save")
				}
				switch outcome {
				case "failure":
					finish(errors.New("disk full"))
				case "cancel":
					if err := tt.Click("Cancel"); err != nil {
						t.Fatal(err)
					}
					if ctx.Err() != context.Canceled {
						t.Fatal("cancel did not signal worker")
					}
					finish(nil)
				case "new-input":
					a.activeDocument().code.Buffer.Set("newer input\n")
					finish(nil)
				default:
					path := filepath.Join(t.TempDir(), "workspace.json")
					if err := adapters.SaveSession(path, captured); err != nil {
						t.Fatal(err)
					}
					loaded, err := adapters.LoadSession(path)
					if err != nil || len(sessionDraftContents(loaded)) != 1 {
						t.Fatalf("draft not durable: %v", err)
					}
					finish(nil)
				}
				if outcome == "success" {
					if closed != 1 || !a.RequestClose() {
						t.Fatal("successful save did not close")
					}
				} else {
					if closed != 0 || !a.activeDocument().modified() || a.sessionSaveError == "" {
						t.Fatal("failed/canceled/stale save lost edits or closed")
					}
					exportSessionScreenshot(t, tt, "draft-"+theme+"-"+outcome)
				}
			})
		}
	}
}

func TestProjectDraftEmptyAndStandaloneSplit(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Switch(a.installRepository(sessionRepository(t.TempDir())))
	a.openCode("project", domain.File{Path: "你好 file.go", New: "delete me\n", DiskVersion: "v1"})
	a.activeDocument().code.Buffer.Set("")
	a.splitTab(a.activeDocument().id, true, true)
	a.repo().tabs = nil
	if !a.hasUnsavedTabs() {
		t.Fatal("standalone split lost dirty state")
	}
	if !a.hasUnsavedRealFiles() {
		t.Fatal("standalone split bypassed repository edit protection")
	}
	s := a.SessionSnapshot()
	drafts := sessionDraftContents(s)
	if len(drafts) != 1 {
		t.Fatal("standalone split draft not captured")
	}
	for _, d := range drafts {
		if d.Text != "" {
			t.Fatal("empty draft changed")
		}
	}
	// A missing file with an empty draft still needs recovery and close protection.
	b := New(application.New(), application.DefaultLayout())
	b.RestoreSession(s)
	b.ReadRepository = func(_ context.Context, _ string, done func(domain.Repository, error)) {
		done(sessionRepository(s.ActiveRoot), nil)
	}
	b.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) {
		done(f, context.DeadlineExceeded)
	}
	b.StartSessionRestore()
	if b.activeDocument() == nil || !b.activeDocument().modified() || len(sessionDraftContents(b.SessionSnapshot())) != 1 {
		t.Fatal("missing file lost empty recovered draft")
	}
}

// The session text must survive an actual JSON roundtrip, then be restored
// independently of the current disk contents without issuing a file write.
func TestProjectDraftRestoresWithoutOverwritingDisk(t *testing.T) {
	for _, disk := range []string{"v1", "v2", "missing"} {
		t.Run(disk, func(t *testing.T) {
			root := t.TempDir()
			a := New(application.New(), application.DefaultLayout())
			a.Model.Switch(a.installRepository(sessionRepository(root)))
			a.openCode("project", domain.File{Path: "你好 file.go", New: "disk\r\n", DiskVersion: "v1"})
			a.activeDocument().code.Buffer.Set("unsaved 草稿\n")
			raw, _ := json.Marshal(a.SessionSnapshot())
			var saved application.WorkspaceSnapshot
			if err := json.Unmarshal(raw, &saved); err != nil {
				t.Fatal(err)
			}
			b := New(application.New(), application.DefaultLayout())
			b.RestoreSession(saved)
			b.ReadRepository = func(_ context.Context, _ string, done func(domain.Repository, error)) {
				done(sessionRepository(root), nil)
			}
			b.ReadRepositoryFile = func(_ context.Context, _, _ string, f domain.File, done func(domain.File, error)) {
				f.New = "external\n"
				f.DiskVersion = disk
				if disk == "missing" {
					done(f, context.DeadlineExceeded)
				} else {
					done(f, nil)
				}
			}
			var version, text string
			b.SaveRepositoryFile = func(_ context.Context, _ domain.Repository, f domain.File, s string, done func(domain.Repository, domain.File, error, error)) {
				version, text = f.DiskVersion, s
			}
			b.StartSessionRestore()
			entry := b.activeDocument()
			if entry == nil || entry.code == nil || entry.code.Buffer.String() != "unsaved 草稿\n" || !entry.modified() {
				t.Fatalf("draft not restored: %+v", entry)
			}
			if text != "" {
				t.Fatal("restore wrote to disk")
			}
			b.run(saveSourceFile, commandTarget{})
			if version != "v1" || text != "unsaved 草稿\r\n" {
				t.Fatalf("lost original version/EOL: %q %q", version, text)
			}
		})
	}
}
