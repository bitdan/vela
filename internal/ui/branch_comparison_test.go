package ui

import (
	"context"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestRealBranchComparisonNativeAndLateCallback(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.Refs = map[string]string{"refs/heads/topic": strings.Repeat("c", 40)}
	a.Model.Active().Repository = r
	var done func(domain.RevisionComparison, error)
	var readContext context.Context
	a.ReadRepositoryComparison = func(ctx context.Context, _ domain.Repository, _ string, _ string, callback func(domain.RevisionComparison, error)) {
		readContext = ctx
		done = callback
	}
	a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, comparison domain.RevisionComparison, _, _ int, callback func(domain.CommitLogPage, error)) {
		callback(domain.CommitLogPage{Commits: []domain.Commit{{Hash: comparison.Left, Subject: "Only in this branch", ParentHashes: []string{strings.Repeat("e", 40)}}}}, nil)
	}
	a.ReadRepositoryRevisionChanges = func(_ context.Context, _ string, revision string, parent int, callback func([]domain.File, error)) {
		if revision != r.Head && revision != r.Refs["refs/heads/topic"] || parent != 0 {
			t.Fatal("wrong comparison commit or parent")
		}
		callback([]domain.File{{Path: "feature.go", Status: "M"}}, nil)
	}
	a.ReadRepositoryFile = func(_ context.Context, _ string, source string, file domain.File, callback func(domain.File, error)) {
		left, right, ok := domain.ParseComparisonSource(source)
		if !ok || left != strings.Repeat("e", 40) || right != r.Refs["refs/heads/topic"] {
			t.Fatal("comparison did not use the selected commit and its parent")
		}
		file.Old = "old"
		file.New = "new"
		callback(file, nil)
	}
	a.run(compareCurrentBranch, commandTarget{source: "topic"})
	if a.branchComparison != nil || done == nil || len(a.repo().tabs) == 0 || a.repo().tabs[a.repo().active].comparison == nil {
		t.Fatal("real branch comparison unavailable")
	}
	comparison := a.repo().tabs[a.repo().active].comparison
	done(domain.RevisionComparison{Left: r.Refs["refs/heads/topic"], Right: r.Head}, nil)
	if comparison.sides[0].commits[0].Hash != r.Refs["refs/heads/topic"] || comparison.sides[1].commits[0].Hash != r.Head {
		t.Fatal("comparison directions were not reversed")
	}
	comparison.sides[0].file = 0
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Open Branch Comparison Diff topic"); err != nil {
		t.Fatal(err)
	}
	if len(a.repo().tabs) == 0 || a.activeDocument().diff == nil || a.activeDocument().diff.New.String() != "new" {
		t.Fatal("did not open actual comparison diff")
	}
	if a.repo().tabIndex(comparison.tabID) < 0 || a.backgroundBlocked() {
		t.Fatal("opening Diff must retain comparison tab without blocking background refresh")
	}
	if err := tt.Click(comparisonTabTitle(comparison)); err != nil {
		t.Fatal(err)
	}
	if a.repo().tabs[a.repo().active].comparison != comparison || comparison.sides[0].file != 0 || comparison.sides[1].file != -1 {
		t.Fatal("returning to comparison lost independent selection")
	}
	_ = a.SessionSnapshot() // Comparison is a transient tool tab, not a file Diff.
	a.run(closeTab, commandTarget{tabID: comparison.tabID})
	if a.repo().tabIndex(comparison.tabID) >= 0 {
		t.Fatal("comparison tab did not close")
	}
	a.run(reopenTab, commandTarget{})
	if a.repo().tabs[a.repo().active].comparison != comparison {
		t.Fatal("comparison tab did not reopen")
	}
	a.run(branchDiff, commandTarget{source: "topic"})
	late := done
	a.run(selectRepo, commandTarget{index: 0})
	if readContext.Err() == nil || a.branchComparison != nil {
		t.Fatal("navigation did not cancel comparison")
	}
	late(domain.RevisionComparison{Left: r.Head}, nil)
	if a.Model.Index != 0 || a.branchComparison != nil {
		t.Fatal("late comparison changed repository")
	}
}

func TestComparisonTabsRestoreRenameIdentity(t *testing.T) {
	a, r := historicalWorkspace(t)
	source := domain.RevisionComparison{Left: strings.Repeat("c", 40), Right: r.Head}.Source()
	a.openDiff(source, domain.File{Path: "new.go", OldPath: "old.go", Status: "R", Old: "before", New: "after"})
	saved := a.SessionSnapshot()
	b := New(application.NewWorkspace(), application.DefaultLayout())
	b.RestoreSession(saved)
	b.ReadRepository = func(_ context.Context, _ string, done func(domain.Repository, error)) { done(r, nil) }
	var identity domain.File
	b.ReadRepositoryFile = func(_ context.Context, _ string, gotSource string, file domain.File, done func(domain.File, error)) {
		if gotSource != source {
			t.Fatal("comparison source lost")
		}
		identity = file
		file.Old = "before"
		file.New = "after"
		done(file, nil)
	}
	b.StartSessionRestore()
	if identity.Status != "R" || identity.OldPath != "old.go" || len(b.repo().tabs) != 1 || b.repo().tabs[0].source != source {
		t.Fatal("comparison tab metadata not restored")
	}
}

func TestComparisonTabIdentityAfterReloadAndClose(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.Refs = map[string]string{"refs/heads/topic": strings.Repeat("c", 40)}
	a.Model.Active().Repository = r
	var callbacks []func(domain.RevisionComparison, error)
	var contexts []context.Context
	a.ReadRepositoryComparison = func(ctx context.Context, _ domain.Repository, _, _ string, done func(domain.RevisionComparison, error)) {
		contexts = append(contexts, ctx)
		callbacks = append(callbacks, done)
	}
	a.ReadRepositoryComparisonCommits = func(context.Context, string, domain.RevisionComparison, int, int, func(domain.CommitLogPage, error)) {
	}
	a.run(compareCurrentBranch, commandTarget{source: "topic"})
	first := a.repo().tabs[a.repo().active].comparison
	a.run(compareCurrentBranch, commandTarget{source: "topic"})
	if len(callbacks) != 1 || len(a.repo().tabs) != 1 {
		t.Fatal("same comparison did not reuse its tab")
	}
	a.Model.Active().Repository.CurrentBranch = "dev"
	a.run(reloadBranchComparison, commandTarget{tabID: first.tabID})
	if first.rightLabel != "dev" || contexts[0].Err() == nil {
		t.Fatal("reload did not replace branch snapshot or cancel old request")
	}
	a.Model.Active().Repository.CurrentBranch = "main"
	a.run(compareCurrentBranch, commandTarget{source: "topic"})
	second := a.repo().tabs[a.repo().active].comparison
	if second == first || len(a.repo().tabs) != 2 || second.rightLabel != "main" {
		t.Fatal("reload left a stale comparison identity")
	}
	if a.disabledFor(splitRight, commandTarget{tabID: second.tabID}) == "" {
		t.Fatal("tool tab must not enter file split path")
	}
	if saved := a.SessionSnapshot(); len(saved.Repositories[a.Model.Index].Tabs) != 0 {
		t.Fatal("comparison tool tabs must not be serialized as file diffs")
	}
	a.run(closeTab, commandTarget{tabID: first.tabID})
	if contexts[1].Err() == nil {
		t.Fatal("closing comparison did not cancel its read")
	}
	callbacks[1](domain.RevisionComparison{Left: strings.Repeat("c", 40), Right: r.Head}, nil)
	if first.ready || a.repo().tabs[a.repo().active].comparison != second {
		t.Fatal("closed comparison callback affected live tabs")
	}
}

func TestComparisonHistoryUsesRightRevision(t *testing.T) {
	a, r := historicalWorkspace(t)
	left, right := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, test := range []struct {
		comparison domain.RevisionComparison
		want       string
	}{{domain.RevisionComparison{Left: left, Right: right}, right}, {domain.RevisionComparison{Left: left, WorktreeVersion: strings.Repeat("c", 64)}, r.Head}} {
		if got := a.blameRevision(commandTarget{source: test.comparison.Source()}); got != test.want {
			t.Fatalf("comparison history starts at %q, want %q", got, test.want)
		}
	}
}

func TestMissingBranchCommitsNativePagingKeepsGlobalLog(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.Refs = map[string]string{"refs/heads/topic": strings.Repeat("c", 40)}
	a.Model.Active().Repository = r
	a.Model.Active().Message = "draft"
	a.repo().logQuery = "keep filter"
	a.ReadRepositoryComparison = func(_ context.Context, _ domain.Repository, _ string, mode string, done func(domain.RevisionComparison, error)) {
		if mode != "missing" {
			t.Fatal("missing query unnecessarily loads tree diff")
		}
		done(domain.RevisionComparison{Left: r.Refs["refs/heads/topic"], Right: r.Head}, nil)
	}
	var late func(domain.CommitLogPage, error)
	var requestContext context.Context
	a.ReadRepositoryComparisonCommits = func(ctx context.Context, _ string, _ domain.RevisionComparison, offset, limit int, done func(domain.CommitLogPage, error)) {
		if limit != 100 {
			t.Fatal("wrong query page limit")
		}
		requestContext = ctx
		if offset == 0 {
			done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: strings.Repeat("d", 40), Subject: "first"}}, HasMore: true}, nil)
		} else {
			late = done
		}
	}
	a.run(missingBranchCommits, commandTarget{source: "topic"})
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Load More Missing Commits"); err != nil {
		t.Fatal(err)
	}
	if late == nil {
		t.Fatal("load more did not dispatch")
	}
	a.run(closeBranchComparison, commandTarget{})
	if requestContext.Err() == nil {
		t.Fatal("closing did not cancel page")
	}
	late(domain.CommitLogPage{Commits: []domain.Commit{{Hash: strings.Repeat("e", 40)}}}, nil)
	if a.branchComparison != nil || len(a.Model.Active().Repository.History) != 1 || a.Model.Active().Message != "draft" || a.repo().logQuery != "keep filter" {
		t.Fatal("query polluted global log or draft")
	}
}
