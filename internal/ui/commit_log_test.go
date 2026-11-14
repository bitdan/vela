package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func logFixture(t *testing.T, dark bool) (*Workspace, *native.Tester) {
	t.Helper()
	layout := application.DefaultLayout()
	layout.Dark = dark
	a := New(application.New(), layout)
	hash := strings.Repeat("a", 40)
	repo := domain.Repository{Root: t.TempDir(), Name: "paged", CurrentBranch: "main", Head: hash,
		History:     []domain.Commit{{Hash: hash, Subject: "Newest", Refs: []string{"main"}, ParentHashes: []string{strings.Repeat("b", 40)}}},
		LogSnapshot: domain.CommitLogSnapshot{Tips: []string{hash}}, LogHasMore: true}
	a.Model.Switch(a.installRepository(repo))
	a.openCode("scratch", domain.File{Path: "scratch.txt"})
	a.repo().tabs[a.repo().active].code.Buffer.Set("unsaved draft")
	a.Model.Active().Working, a.Model.Active().Staged = "working", "staged"
	a.Model.Active().Historical, a.Model.Active().Message = "history", "commit draft"
	return a, native.NewTester(a.View, 1440, 900)
}

func TestGlobalLogLoadRetryAndPreserveState(t *testing.T) {
	if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, dark := range []bool{false, true} {
		t.Run(map[bool]string{false: "light", true: "dark"}[dark], func(t *testing.T) {
			a, tt := logFixture(t, dark)
			code := a.repo().tabs[a.repo().active].code
			var finish func(domain.CommitLogPage, error)
			calls := 0
			a.ReadRepositoryLog = func(ctx context.Context, root string, snapshot domain.CommitLogSnapshot, offset, limit int, done func(domain.CommitLogPage, error)) {
				calls++
				if root != a.Model.Active().Repository.Root || offset != 1 || limit != 500 || len(snapshot.Tips) != 1 {
					t.Fatal("wrong page identity")
				}
				finish = done
			}
			tt.Frame()
			if err := tt.Click("Load More Commits"); err != nil {
				t.Fatal(err)
			}
			a.run(loadMoreCommits, commandTarget{})
			if calls != 1 {
				t.Fatal("duplicate request while loading")
			}
			finish(domain.CommitLogPage{}, errors.New("fixture failure"))
			tt.Frame()
			if !tt.HasText("Could not load commits · fixture failure") || len(a.Model.Active().Repository.History) != 1 {
				t.Fatal("failed page lost history or error")
			}
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				writeScreenshot(t, tt, filepath.Join(dir, map[bool]string{false: "light", true: "dark"}[dark]+"-log-retry.png"))
			}
			if err := tt.Click("Load More Commits"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("Loading commits…") {
				t.Fatal("missing loading state")
			}
			a.repo().logQuery = "older"
			tt.Frame()
			finish(domain.CommitLogPage{Commits: []domain.Commit{{Hash: strings.Repeat("b", 40), Subject: "older"}}}, nil)
			tt.Frame()
			s := a.Model.Active()
			if len(s.Repository.History) != 2 || s.Repository.LogHasMore || len(s.Repository.History[0].Parents) != 1 || len(a.repo().filteredLog) != 1 {
				t.Fatalf("append did not update graph/filter: %+v", s.Repository.History)
			}
			if s.Commit != 0 || s.Working != "working" || s.Staged != "staged" || s.Historical != "history" || s.Message != "commit draft" || code.Buffer.String() != "unsaved draft" || a.repo().tabs[a.repo().active].code != code {
				t.Fatal("pagination reset independent state")
			}
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				writeScreenshot(t, tt, filepath.Join(dir, map[bool]string{false: "light", true: "dark"}[dark]+"-log-loaded.png"))
			}
		})
	}
}

func TestGlobalLogLatePageIsIgnored(t *testing.T) {
	for _, action := range []string{"cancel", "switch", "refresh", "write", "close"} {
		t.Run(action, func(t *testing.T) {
			a, _ := logFixture(t, true)
			var finish func(domain.CommitLogPage, error)
			var requestCtx context.Context
			a.ReadRepositoryLog = func(ctx context.Context, root string, snapshot domain.CommitLogSnapshot, offset, limit int, done func(domain.CommitLogPage, error)) {
				requestCtx, finish = ctx, done
			}
			index := a.Model.Index
			a.run(loadMoreCommits, commandTarget{})
			switch action {
			case "cancel":
				a.run(cancelLogLoading, commandTarget{})
			case "switch":
				a.run(selectRepo, commandTarget{index: 0})
			case "refresh":
				a.installRepository(a.Model.Active().Repository)
			case "write":
				a.beginRepositoryWrite(a.Model.Active().Repository.Root, "writing", func() {})
			case "close":
				a.RequestClose()
			}
			if requestCtx.Err() != context.Canceled {
				t.Fatal("request not canceled")
			}
			finish(domain.CommitLogPage{Commits: []domain.Commit{{Hash: "late", Subject: "late"}}}, nil)
			if len(a.Model.Sessions[index].Repository.History) != 1 {
				t.Fatal("late page mutated old session")
			}
		})
	}
}

func TestGlobalLogRejectsInvalidPages(t *testing.T) {
	for _, page := range []domain.CommitLogPage{
		{HasMore: true},
		{Commits: []domain.Commit{{Hash: strings.Repeat("a", 40)}}},
		{Commits: []domain.Commit{{Hash: strings.Repeat("b", 40)}, {Hash: strings.Repeat("b", 40)}}},
	} {
		a, _ := logFixture(t, true)
		a.ReadRepositoryLog = func(_ context.Context, _ string, _ domain.CommitLogSnapshot, _, _ int, done func(domain.CommitLogPage, error)) {
			done(page, nil)
		}
		a.run(loadMoreCommits, commandTarget{})
		if len(a.Model.Active().Repository.History) != 1 || a.repo().logError == "" {
			t.Fatal("invalid page accepted")
		}
	}
}

func TestGlobalLogAppendKeepsListFocusAndScroll(t *testing.T) {
	a, tt := logFixture(t, true)
	history := make([]domain.Commit, 400)
	for i := range history {
		history[i] = domain.Commit{Hash: fmt.Sprintf("%040x", i+1), Subject: fmt.Sprintf("row %d", i), ParentHashes: []string{fmt.Sprintf("%040x", i+2)}}
	}
	domain.ReconnectCommitLog(history)
	a.Model.Active().Repository.History = history
	a.repo().rebuildCommitLog(a.Model.Active().Repository)
	tt.Frame()
	a.repo().log.ScrollTo(200, native.Center)
	tt.Frame()
	if err := tt.Click("row 200"); err != nil {
		t.Fatal(err)
	}
	before, ok := tt.Find("row 200")
	if !ok {
		t.Fatal("selected row is not visible")
	}
	a.ReadRepositoryLog = func(_ context.Context, _ string, _ domain.CommitLogSnapshot, _, _ int, done func(domain.CommitLogPage, error)) {
		done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: fmt.Sprintf("%040x", 401), Subject: "last"}}}, nil)
	}
	a.run(loadMoreCommits, commandTarget{})
	tt.Frame()
	after, ok := tt.Find("row 200")
	if !ok || after.Y != before.Y || a.Model.Active().Commit != 200 {
		t.Fatal("append moved viewport/selection")
	}
	tt.Key(0, native.KeyDown)
	if a.Model.Active().Commit != 201 {
		t.Fatal("append lost list keyboard focus")
	}
}
