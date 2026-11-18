package ui

import (
	"context"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestComparisonFiltersKeepFrozenRangeAndCancelOnlyTheirSide(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.Refs = map[string]string{"refs/heads/topic": strings.Repeat("c", 40)}
	a.Model.Active().Repository = r
	a.ReadRepositoryComparison = func(_ context.Context, _ domain.Repository, _, _ string, done func(domain.RevisionComparison, error)) {
		done(domain.RevisionComparison{Left: r.Refs["refs/heads/topic"], Right: r.Head}, nil)
	}
	type query struct {
		ctx        context.Context
		comparison domain.RevisionComparison
		done       func(domain.CommitLogPage, error)
	}
	var queries []query
	a.ReadRepositoryComparisonCommits = func(ctx context.Context, _ string, comparison domain.RevisionComparison, offset, limit int, done func(domain.CommitLogPage, error)) {
		if offset != 0 || limit != 100 {
			t.Fatal("filtered query did not start with a fresh page")
		}
		queries = append(queries, query{ctx, comparison, done})
	}
	a.run(compareCurrentBranch, commandTarget{source: "topic"})
	s := a.repo().tabs[a.repo().active].comparison
	top, bottom := s.sides[0], s.sides[1]
	top.draft = domain.ComparisonLogFilter{Message: "literal [message]", Path: "src/file.go"}
	target := commandTarget{tabID: s.tabID, source: "0"}
	a.run(applyComparisonFilters, target)
	if len(queries) != 3 || queries[0].ctx.Err() == nil || queries[1].ctx.Err() != nil {
		t.Fatal("filter cancellation crossed comparison sides")
	}
	if queries[2].comparison.Left != r.Refs["refs/heads/topic"] || queries[2].comparison.Right != r.Head || queries[2].comparison.Filter != top.draft {
		t.Fatal("filter changed frozen range or lost input")
	}
	queries[0].done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: strings.Repeat("d", 40)}}}, nil)
	if len(top.commits) != 0 || bottom.comparison.Filter != (domain.ComparisonLogFilter{}) {
		t.Fatal("stale filter callback or other side was changed")
	}
	top.draft.Since = "invalid"
	a.run(applyComparisonFilters, target)
	if len(queries) != 3 || queries[2].ctx.Err() != nil || top.filterMessage == "" {
		t.Fatal("invalid filter replaced running query")
	}
	a.run(clearComparisonFilters, target)
	if len(queries) != 4 || queries[3].comparison.Filter != (domain.ComparisonLogFilter{}) || queries[2].ctx.Err() == nil {
		t.Fatal("clear did not restart original fixed range")
	}
	a.run(closeTab, commandTarget{tabID: s.tabID})
	if queries[1].ctx.Err() == nil || queries[3].ctx.Err() == nil {
		t.Fatal("closing tab did not cancel both sides")
	}
}
