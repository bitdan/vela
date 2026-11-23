package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

func TestEnhancedComparisonLayoutAndStates(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		a, r := historicalWorkspace(t)
		a.Layout.Dark = theme == "dark"
		a.Layout.Appearance.FollowSystem = false
		left, parent1, parent2 := strings.Repeat("c", 40), strings.Repeat("d", 40), strings.Repeat("e", 40)
		r.Refs = map[string]string{"refs/heads/topic": left}
		a.Model.Active().Repository = r
		a.ReadRepositoryComparison = func(_ context.Context, _ domain.Repository, _, _ string, done func(domain.RevisionComparison, error)) {
			done(domain.RevisionComparison{Left: left, Right: r.Head}, nil)
		}
		a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, c domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
			done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: c.Left, Subject: "Merge topic changes", Author: "Developer", ParentHashes: []string{parent1, parent2}}, {Hash: parent1, Subject: "Preserve independent comparison state", Author: "Developer"}}}, nil)
		}
		scene := "parents"
		a.ReadRepositoryComparisonChanges = func(_ context.Context, _ string, c domain.RevisionComparison, ids []string, done func([]domain.ComparisonChangeGroup, error)) {
			if scene == "loading" {
				return
			}
			if scene == "error" {
				done(nil, errors.New("Commit is unavailable. Reload to compare current branch tips."))
				return
			}
			if scene == "empty" {
				done(nil, nil)
				return
			}
			if len(ids) > 1 {
				done([]domain.ComparisonChangeGroup{{Title: "Net changes · 2 commits", Source: (domain.RevisionComparison{Left: parent1, Right: c.Left}).Source(), Files: []domain.File{{Path: "internal/ui/comparison_session.go", Status: "M"}, {Path: "new 文件.go", Status: "A"}}}}, nil)
				return
			}
			var groups []domain.ComparisonChangeGroup
			for i, parent := range []string{parent1, parent2} {
				groups = append(groups, domain.ComparisonChangeGroup{Title: []string{"Parent 1 · dddddddd", "Parent 2 · eeeeeeee"}[i], Source: (domain.RevisionComparison{Left: parent, Right: c.Left}).Source(), Files: []domain.File{{Path: "internal/ui/branch_comparison_log.go", Status: "M"}}})
			}
			done(groups, nil)
		}
		a.run(compareCurrentBranch, commandTarget{source: "topic"})
		s := a.repo().tabs[a.repo().active].comparison
		tt := native.NewTester(a.View, 1440, 900)
		capture := func(name string) {
			t.Helper()
			tt.Frame()
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				writeScreenshot(t, tt, filepath.Join(dir, theme+"-comparison-"+name+".png"))
			}
		}
		capture(scene)
		if box, ok := tt.Find(comparisonTabTitle(s)); !ok || box.H < 20 {
			t.Fatalf("comparison tab must remain visible: %+v %t", box, ok)
		}
		if _, ok := tt.Find("Comparison group topic " + (domain.RevisionComparison{Left: parent1, Right: left}).Source()); !ok {
			t.Fatal("first merge parent group missing")
		}
		if err := tt.Click("Comparison file topic internal/ui/branch_comparison_log.go"); err != nil {
			t.Fatal(err)
		}
		tt.Key(0, native.KeyEnd)
		if s.sides[0].file != 1 || s.sides[0].fileSources[1] != (domain.RevisionComparison{Left: parent2, Right: left}).Source() {
			t.Fatal("keyboard could not reach second parent file")
		}
		capture("parent2-keyboard")
		for _, next := range []string{"aggregate", "loading", "empty", "error"} {
			scene = next
			s.sides[0].chosen.Add(parent1)
			a.loadComparisonDetails(s, s.sides[0], 0)
			capture(scene)
		}
		scene = "aggregate"
		a.loadComparisonDetails(s, s.sides[0], 0)
		s.sides[0].draft.Hash = "abcd1234"
		s.sides[0].draft.Regex = true
		s.sides[0].draft.Message = "^fix (one|two)$"
		capture("filters")
		a.Layout.HideCommit, a.Layout.HideGit = true, true
		tt.SetSize(1280, 800)
		capture("resized-hidden")
		a.run(restoreLayout, commandTarget{})
		tt.SetSize(1440, 900)
		capture("restored-layout")
		if a.repo().tabs[a.repo().active].comparison != s || s.sides[0].chosen.Len() != 2 || s.sides[0].draft.Message != "^fix (one|two)$" {
			t.Fatal("layout restoration lost comparison state")
		}
	}
}
