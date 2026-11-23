package ui

import (
	"context"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

func TestComparisonMultiSelectionNativeAndLateDetails(t *testing.T) {
	for _, dark := range []bool{true, false} {
		a, r := historicalWorkspace(t)
		a.Layout.Dark = dark
		left := strings.Repeat("c", 40)
		older := strings.Repeat("e", 40)
		r.Refs = map[string]string{"refs/heads/topic": left}
		a.Model.Active().Repository = r
		a.ReadRepositoryComparison = func(_ context.Context, _ domain.Repository, _, _ string, done func(domain.RevisionComparison, error)) {
			done(domain.RevisionComparison{Left: left, Right: r.Head}, nil)
		}
		a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, c domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
			done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: c.Left, Subject: "Newer"}, {Hash: older, Subject: "Older"}}}, nil)
		}
		type query struct {
			ctx  context.Context
			ids  []string
			done func([]domain.ComparisonChangeGroup, error)
		}
		var queries []query
		a.ReadRepositoryComparisonChanges = func(ctx context.Context, _ string, _ domain.RevisionComparison, ids []string, done func([]domain.ComparisonChangeGroup, error)) {
			queries = append(queries, query{ctx, ids, done})
		}
		a.ReadRepositoryFile = func(_ context.Context, _ string, source string, f domain.File, done func(domain.File, error)) {
			if source != (domain.RevisionComparison{Left: older, Right: left}).Source() {
				t.Fatalf("wrong grouped diff: %s", source)
			}
			f.Old, f.New = "before", "after"
			done(f, nil)
		}
		a.run(compareCurrentBranch, commandTarget{source: "topic"})
		s := a.repo().tabs[a.repo().active].comparison
		tt := native.NewTester(a.View, 1440, 900)
		if err := tt.ClickWith(native.Ctrl, "Comparison commit topic "+older); err != nil {
			t.Fatal(err)
		}
		if s.sides[0].chosen.Len() != 2 || s.sides[1].chosen.Len() != 1 || len(queries) != 3 || len(queries[2].ids) != 2 || queries[0].ctx.Err() == nil || queries[1].ctx.Err() != nil {
			t.Fatal("multi-select or per-side cancellation failed")
		}
		queries[0].done([]domain.ComparisonChangeGroup{{Title: "stale", Source: r.Head, Files: []domain.File{{Path: "stale.go"}}}}, nil)
		if len(s.sides[0].files) != 0 {
			t.Fatal("stale details installed")
		}
		source := (domain.RevisionComparison{Left: older, Right: left}).Source()
		queries[2].done([]domain.ComparisonChangeGroup{{Title: "Net changes", Source: source, Files: []domain.File{{Path: "result.go", Status: "M"}}}}, nil)
		tt.Frame()
		if err := tt.Click("Comparison file topic result.go"); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Open Branch Comparison Diff topic"); err != nil {
			t.Fatal(err)
		}
		if a.activeDocument().diff == nil || a.activeDocument().diff.New.String() != "after" {
			t.Fatal("group file did not open correct diff")
		}
		if err := tt.Click(comparisonTabTitle(s)); err != nil {
			t.Fatal(err)
		}
		if err := tt.Click("Comparison commit topic " + left); err != nil {
			t.Fatal(err)
		}
		tt.Key(native.Shift, native.KeyDown)
		if s.sides[0].chosen.Len() != 2 {
			t.Fatal("Shift+Down did not extend selection")
		}
		a.run(closeTab, commandTarget{tabID: s.tabID})
		if queries[len(queries)-1].ctx.Err() == nil || queries[1].ctx.Err() == nil {
			t.Fatal("close did not cancel comparison requests")
		}
	}
}

func TestComparisonFilterInvalidRegexRetainsResults(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.Refs = map[string]string{"refs/heads/topic": strings.Repeat("c", 40)}
	a.Model.Active().Repository = r
	a.ReadRepositoryComparison = func(_ context.Context, _ domain.Repository, _, _ string, done func(domain.RevisionComparison, error)) {
		done(domain.RevisionComparison{Left: r.Refs["refs/heads/topic"], Right: r.Head}, nil)
	}
	a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, c domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
		done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: c.Left}}}, nil)
	}
	a.run(compareCurrentBranch, commandTarget{source: "topic"})
	s := a.repo().tabs[a.repo().active].comparison
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Comparison regex topic"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Comparison message filter topic"); err != nil {
		t.Fatal(err)
	}
	tt.Type("[")
	if err := tt.Click("Apply Comparison Filters topic"); err != nil {
		t.Fatal(err)
	}
	if s.sides[0].filterMessage == "" || len(s.sides[0].commits) != 1 || s.filters[0].Regex {
		t.Fatal("invalid regex replaced applied results")
	}
	if err := tt.Click("Clear Comparison Filters topic"); err != nil {
		t.Fatal(err)
	}
	if s.sides[0].draft.Regex || s.sides[0].draft.Message != "" || s.sides[0].filterMessage != "" {
		t.Fatal("clear failed")
	}
}
