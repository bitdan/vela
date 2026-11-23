package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/adapters"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestComparisonToolSnapshotRestoresFrozenRange(t *testing.T) {
	a, r := historicalWorkspace(t)
	left := strings.Repeat("c", 40)
	r.Refs = map[string]string{"refs/heads/topic": left}
	a.Model.Active().Repository = r
	a.ReadRepositoryComparison = func(_ context.Context, _ domain.Repository, _, _ string, done func(domain.RevisionComparison, error)) {
		done(domain.RevisionComparison{Left: left, Right: r.Head}, nil)
	}
	a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, c domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
		done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: c.Left, Subject: "saved"}}}, nil)
	}
	a.run(compareCurrentBranch, commandTarget{source: "topic"})
	s := a.repo().tabs[a.repo().active].comparison
	s.split, s.sides[0].split = 42, 61
	s.sides[0].draft = domain.ComparisonLogFilter{Message: "saved"}
	a.run(applyComparisonFilters, commandTarget{tabID: s.tabID})
	a.repo().tabs[a.repo().active].pinned = true
	saved := a.SessionSnapshot()
	path := filepath.Join(t.TempDir(), "session.json")
	if err := adapters.SaveSession(path, saved); err != nil {
		t.Fatal(err)
	}
	var err error
	saved, err = adapters.LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, repo := range saved.Repositories {
		if repo.Root == r.Root {
			count = len(repo.Tabs)
		}
	}
	if count != 1 {
		t.Fatalf("comparison tool identity not saved: %d tabs", count)
	}
	b := New(application.NewWorkspace(), application.DefaultLayout())
	b.RestoreSession(saved)
	// Simulate branches advancing: restore must use saved objects, not recapture refs.
	r.Head = strings.Repeat("d", 40)
	b.ReadRepository = func(_ context.Context, _ string, done func(domain.Repository, error)) { done(r, nil) }
	var comparisons []domain.RevisionComparison
	b.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, c domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
		comparisons = append(comparisons, c)
		done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: c.Left, Subject: "saved"}}}, nil)
	}
	b.StartSessionRestore()
	if len(b.repo().tabs) != 1 || b.repo().tabs[0].comparison == nil {
		t.Fatal("comparison tool not restored")
	}
	restored := b.repo().tabs[0].comparison
	if len(comparisons) != 2 || restored.comparison.Left != left || restored.comparison.Right != s.comparison.Right || restored.split != 42 || restored.sides[0].split != 61 || restored.sides[0].comparison.Filter.Message != "saved" || !b.repo().tabs[0].pinned {
		t.Fatalf("lost fixed identity, filter or layout: %+v", restored)
	}
	if b.repo().tabs[b.repo().active].id != saved.Repositories[len(saved.Repositories)-1].ActiveTab {
		t.Fatal("active tool tab lost")
	}
}

func TestComparisonRestoreMorePagesAndNavigationWins(t *testing.T) {
	a, r := historicalWorkspace(t)
	left, right, older := strings.Repeat("c", 40), r.Head, strings.Repeat("e", 40)
	saved := application.ComparisonSnapshot{Ref: "refs/heads/topic", Label: "topic", RightLabel: "main", Left: left, Right: right, Split: 45}
	saved.Sides[0] = application.ComparisonSideSnapshot{Selected: []string{older}, ActiveCommit: older, Loaded: 2, Split: 60, DetailSplit: 48}
	entry := application.TabSnapshot{Source: "branch-compare:restored", Comparison: &saved}
	var requests []context.Context
	var callbacks []func(domain.CommitLogPage, error)
	a.ReadRepositoryComparisonCommits = func(ctx context.Context, _ string, _ domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
		requests = append(requests, ctx)
		callbacks = append(callbacks, done)
	}
	if !a.restoreComparisonTab(entry) {
		t.Fatal("could not create restored comparison")
	}
	s := a.repo().tabs[len(a.repo().tabs)-1].comparison
	callbacks[0](domain.CommitLogPage{Commits: []domain.Commit{{Hash: left}}, HasMore: true}, nil)
	if len(callbacks) != 3 {
		t.Fatal("restore did not request saved loaded pages")
	}
	callbacks[2](domain.CommitLogPage{Commits: []domain.Commit{{Hash: older}}}, nil)
	if s.sides[0].selected != 1 || !s.sides[0].chosen.Has(older) || s.sides[0].chosen.Has(left) {
		t.Fatal("restore lost selection outside first page")
	}
	a.openCode("project", domain.File{Path: "user.go", New: "user navigation"})
	active := a.repo().tabs[a.repo().active].id
	callbacks[1](domain.CommitLogPage{Commits: []domain.Commit{{Hash: right}}}, nil)
	if a.repo().tabs[a.repo().active].id != active {
		t.Fatal("restore callback stole active tab")
	}
	a.run(closeTab, commandTarget{tabID: s.tabID})
	if a.repo().tabIndex(s.tabID) >= 0 {
		t.Fatal("restored comparison did not close")
	}
	callbacks[0](domain.CommitLogPage{Commits: []domain.Commit{{Hash: left}}}, nil)
	if a.repo().tabs[a.repo().active].id != active {
		t.Fatal("closed comparison callback changed navigation")
	}
}

func TestComparisonSecondExitKeepsPendingFileSelection(t *testing.T) {
	a, r := historicalWorkspace(t)
	left, parent := strings.Repeat("c", 40), strings.Repeat("e", 40)
	source := (domain.RevisionComparison{Left: parent, Right: left}).Source()
	saved := application.ComparisonSnapshot{Ref: "refs/heads/topic", Label: "topic", RightLabel: "main", Left: left, Right: r.Head}
	saved.Sides[0] = application.ComparisonSideSnapshot{Selected: []string{left}, ActiveCommit: left, Loaded: 1, File: "saved.go", FileSource: source, ClosedGroups: []string{source}}
	a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, c domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
		done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: c.Left}}}, nil)
	}
	var complete func([]domain.ComparisonChangeGroup, error)
	a.ReadRepositoryComparisonChanges = func(_ context.Context, _ string, c domain.RevisionComparison, _ []string, done func([]domain.ComparisonChangeGroup, error)) {
		if c.Left == left {
			complete = done
		}
	}
	if !a.restoreComparisonTab(application.TabSnapshot{Source: "branch-compare:pending-file", Comparison: &saved}) {
		t.Fatal("restore failed")
	}
	s := a.repo().tabs[len(a.repo().tabs)-1].comparison
	if s.sides[0].restore != nil || complete == nil {
		t.Fatal("fixture did not reach asynchronous file restoration")
	}
	got := comparisonSnapshot(s).Sides[0]
	if got.File != "saved.go" || got.FileSource != source {
		t.Fatalf("pending file identity lost on second exit: %+v", got)
	}
	complete([]domain.ComparisonChangeGroup{{Title: "parent", Source: source, Files: []domain.File{{Path: "saved.go", Status: "M"}}}}, nil)
	if s.sides[0].file != 0 {
		t.Fatal("pending file selection not installed")
	}
	s.sides[0].groupOutline.Open.Add(-1)
	a.repo().active = len(a.repo().tabs) - 1
	native.NewTester(a.View, 1440, 900).Frame()
	if s.sides[0].groupRow != 1 {
		t.Fatal("restored file selection is not reflected in the outline")
	}
}

func TestComparisonPartialRestoreKeepsLiveDraftAndSplits(t *testing.T) {
	a, r := historicalWorkspace(t)
	left := strings.Repeat("c", 40)
	saved := application.ComparisonSnapshot{Label: "topic", RightLabel: "main", Left: left, Right: r.Head}
	saved.Sides[0] = application.ComparisonSideSnapshot{Selected: []string{left}, ActiveCommit: left, Loaded: 200, Split: 64, DetailSplit: 55}
	a.ReadRepositoryComparisonCommits = func(context.Context, string, domain.RevisionComparison, int, int, func(domain.CommitLogPage, error)) {
	}
	if !a.restoreComparisonTab(application.TabSnapshot{Source: "branch-compare:pending-pages", Comparison: &saved}) {
		t.Fatal("restore failed")
	}
	s := a.repo().tabs[len(a.repo().tabs)-1].comparison
	s.sides[0].draft.Message = "new draft"
	s.sides[0].split, s.sides[0].detailSplit = 71, 63
	got := comparisonSnapshot(s).Sides[0]
	if got.Draft.Message != "new draft" || got.Split != 71 || got.DetailSplit != 63 || got.Loaded != 200 || len(got.Selected) != 1 || got.Selected[0] != left {
		t.Fatalf("partial restore overwrote live view or pending identities: %+v", got)
	}
}

func TestComparisonRestorePreservesExplicitEmptySelection(t *testing.T) {
	a, r := historicalWorkspace(t)
	left := strings.Repeat("c", 40)
	saved := application.ComparisonSnapshot{Label: "topic", RightLabel: "main", Left: left, Right: r.Head}
	// Ctrl-clicking the last chosen row leaves a focused row but no selection.
	saved.Sides[0] = application.ComparisonSideSnapshot{ActiveCommit: left, Loaded: 1}
	a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, c domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
		done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: c.Left}}}, nil)
	}
	a.ReadRepositoryComparisonChanges = func(context.Context, string, domain.RevisionComparison, []string, func([]domain.ComparisonChangeGroup, error)) {
	}
	if !a.restoreComparisonTab(application.TabSnapshot{Source: "branch-compare:no-selection", Comparison: &saved}) {
		t.Fatal("restore failed")
	}
	s := a.repo().tabs[len(a.repo().tabs)-1].comparison
	if s.sides[0].chosen.Len() != 0 || s.sides[0].revision != "" {
		t.Fatal("restore invented a commit selection")
	}
}
