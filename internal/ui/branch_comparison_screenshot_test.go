package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func TestProductionComparisonScreenshots(t *testing.T) {
	dir := os.Getenv("VELA_SCREENSHOTS")
	if dir == "" {
		t.Skip("set VELA_SCREENSHOTS to export PNGs")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"dark", "light"} {
		a := New(application.NewWorkspace(), application.DefaultLayout())
		a.Layout.Dark = theme == "dark"
		a.Layout.Appearance.FollowSystem = false
		a.ReadRepository = func(context.Context, string, func(domain.Repository, error)) {}
		a.PickRepositoryDirectory = func(func(string, error)) {}
		a.CloneRepository = func(context.Context, string, string, func(string, error)) {}
		a.InitializeRepository = func(context.Context, string, string, func(string, error)) {}
		capture := func(name string) {
			t.Helper()
			writeScreenshot(t, native.NewTester(a.View, 1440, 900), filepath.Join(dir, theme+"-"+name+".png"))
		}
		capture("welcome")
		a.run(openDemo, commandTarget{})
		capture("explicit-demo")
		a, r := historicalWorkspace(t)
		a.Layout.Dark = theme == "dark"
		a.Layout.Appearance.FollowSystem = false
		r.Refs = map[string]string{"refs/heads/topic": strings.Repeat("c", 40)}
		a.Model.Active().Repository = r
		a.ReadRepositoryFile = func(context.Context, string, string, domain.File, func(domain.File, error)) {}
		a.ReadRepositoryRevisionTree = func(context.Context, string, string, string, func(domain.RevisionTree, error)) {}
		for _, scene := range []string{"committed", "working", "missing", "loading", "empty", "error"} {
			a.ReadRepositoryComparisonCommits = func(_ context.Context, _ string, comparison domain.RevisionComparison, _, _ int, done func(domain.CommitLogPage, error)) {
				if scene == "empty" {
					done(domain.CommitLogPage{}, nil)
					return
				}
				done(domain.CommitLogPage{Commits: []domain.Commit{{Hash: comparison.Left, Subject: "Fix remote tracking for renamed branch", Author: "Developer"}, {Hash: strings.Repeat("e", 40), Subject: "保留分支比较与主 Log 的独立状态", Author: "开发者"}}, HasMore: true}, nil)
			}
			a.ReadRepositoryRevisionChanges = func(_ context.Context, _, _ string, _ int, done func([]domain.File, error)) {
				done([]domain.File{{Path: "internal/ui/branch_comparison.go", Status: "M"}, {Path: "new file.go", Status: "A"}}, nil)
			}
			a.ReadRepositoryComparison = func(_ context.Context, _ domain.Repository, _ string, mode string, done func(domain.RevisionComparison, error)) {
				if scene == "loading" {
					return
				}
				if scene == "error" {
					done(domain.RevisionComparison{}, errors.New("Repository changed. Close this window, refresh the repository and compare again."))
					return
				}
				result := domain.RevisionComparison{Left: r.Refs["refs/heads/topic"], Right: r.Head}
				if mode == "working" {
					result.Right = ""
					result.WorktreeVersion = strings.Repeat("f", 64)
				}
				if scene != "empty" {
					result.Files = []domain.File{{Path: "internal/ui/branch_comparison.go", OldPath: "internal/ui/branch_diff.go", Status: "R"}, {Path: "README.md", Status: "M"}, {Path: "旧文件.txt", Status: "D"}, {Path: "new file.go", Status: "A"}}
				}
				done(result, nil)
			}
			id := compareCurrentBranch
			if scene == "working" {
				id = branchDiff
			}
			if scene == "missing" {
				id = missingBranchCommits
			}
			a.run(id, commandTarget{source: "topic"})
			if a.branchComparison != nil {
				a.branchComparison.selected = 0
			}
			capture(scene)
			a.closeBranchComparisonDialog()
			if s := a.comparisonTarget(commandTarget{}); s != nil && s.tabID != "" {
				a.closeTabs(closeTab, commandTarget{tabID: s.tabID})
			}
		}
	}
}
