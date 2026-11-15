package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

func historicalWorkspace(t *testing.T) (*Workspace, domain.Repository) {
	t.Helper()
	a := New(application.New(), application.DefaultLayout())
	r := sessionRepository(t.TempDir())
	r.IndexVersion = "index"
	r.Head = strings.Repeat("a", 40)
	r.CurrentBranch = "main"
	r.Working, r.Staged = nil, nil
	r.History = []domain.Commit{{Hash: strings.Repeat("b", 40), Subject: "Historical work", Files: []domain.File{{Path: "a.go", Status: "M"}, {Path: "b.go", Status: "A"}, {Path: "c.go", Status: "M"}}}}
	a.Model.Switch(a.installRepository(r))
	return a, r
}

func TestHistoricalWriteRefreshesTargetRenameAndRetainsLateEdits(t *testing.T) {
	for _, dirty := range []bool{false, true} {
		t.Run(fmt.Sprint(dirty), func(t *testing.T) {
			a, r := historicalWorkspace(t)
			a.openCode("project", domain.File{Path: "destination.go", New: "before", DiskVersion: "disk"})
			var complete func(domain.Repository, error, error)
			a.OperateRepository = func(_ context.Context, _ domain.Repository, _ domain.RepositoryOperation, done func(domain.Repository, error, error)) {
				complete = done
			}
			a.ReadRepositoryFile = func(_ context.Context, _ string, _ string, file domain.File, done func(domain.File, error)) {
				file.New = "after"
				file.DiskVersion = "new-disk"
				done(file, nil)
			}
			a.run(cherryPickChanges, commandTarget{source: r.History[0].Hash, file: r.History[0].Files[0]})
			a.applyRepositoryOperation(a.repositoryOperation)
			if complete == nil {
				t.Fatal("write did not dispatch")
			}
			if dirty {
				a.activeDocument().code.Buffer.Set("late unsaved edit")
			}
			preview := diffTab("working", domain.File{Path: "preview.go", Old: "old", New: "stale"})
			a.repo().preview = &preview
			complete(r, nil, nil)
			want := "after"
			if dirty {
				want = "late unsaved edit"
			}
			if a.activeDocument().code.Buffer.String() != want {
				t.Fatal("renamed destination stale or late edit overwritten")
			}
			if a.repo().preview != nil {
				t.Fatal("stale mutable preview retained across refresh")
			}
		})
	}
}

func TestAdvancedHistoryScreenshots(t *testing.T) {
	dir := os.Getenv("VELA_SCREENSHOTS")
	if dir == "" {
		t.Skip("set VELA_SCREENSHOTS")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, theme := range []string{"dark", "light"} {
		a, r := historicalWorkspace(t)
		a.Layout.Dark = theme == "dark"
		a.Layout.Appearance.FollowSystem = false
		a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
		}
		a.ReadRepositoryRevisionTree = func(_ context.Context, _ string, oid, directory string, done func(domain.RevisionTree, error)) {
			done(domain.RevisionTree{Revision: oid, Directory: directory, Entries: []domain.RevisionTreeEntry{{Path: "internal", Kind: "tree", Mode: "040000"}, {Path: "README.md", Kind: "blob", Mode: "100644", Size: 521}, {Path: "assets.bin", Kind: "blob", Mode: "100644", Size: 4096}, {Path: "link", Kind: "blob", Mode: "120000", Size: 9}, {Path: "module", Kind: "commit", Mode: "160000"}}}, nil)
		}
		a.run(showRepositoryRevision, commandTarget{source: r.History[0].Hash})
		tt := native.NewTester(a.View, 1440, 900)
		writeScreenshot(t, tt, filepath.Join(dir, theme+"-repository-tree.png"))
		a.closeRevisionBrowser()
		a.run(checkoutRevision, commandTarget{source: r.History[0].Hash})
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, theme+"-checkout-revision.png"))
		a.repositoryOperation = nil
		a.run(getRevision, commandTarget{source: r.History[0].Hash, file: r.History[0].Files[0]})
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, theme+"-restore-file.png"))
		a.repositoryOperation = nil
		r.History[0].Merge = true
		r.History[0].ParentHashes = []string{strings.Repeat("c", 40), strings.Repeat("d", 40)}
		a.Model.Active().Repository = r
		a.ReadRepositoryRevisionChanges = func(_ context.Context, _ string, _ string, _ int, done func([]domain.File, error)) {
			done([]domain.File{{Path: "mainline-two-only.go", Status: "A"}, {Path: "internal/feature.go", Status: "M"}}, nil)
		}
		a.run(cherryPickChanges, commandTarget{source: r.History[0].Hash, file: r.History[0].Files[0]})
		s := a.repositoryOperation
		s.mainline = s.mainlines[2]
		a.loadHistoryComparison(s)
		s.historyChanges.chosen[0] = true
		updateHistoryComparisonPaths(s)
		tt.Frame()
		writeScreenshot(t, tt, filepath.Join(dir, theme+"-selected-mainline.png"))
	}
}

func TestSelectedHistoryMergeMainlineComparison(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.History[0].Merge = true
	r.History[0].ParentHashes = []string{strings.Repeat("c", 40), strings.Repeat("d", 40)}
	a.Model.Active().Repository = r
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	var parent int
	a.ReadRepositoryRevisionChanges = func(_ context.Context, _ string, _ string, line int, done func([]domain.File, error)) {
		parent = line
		done([]domain.File{{Path: "parent-two-only.go", Status: "A"}}, nil)
	}
	a.run(cherryPickChanges, commandTarget{source: r.History[0].Hash, file: r.History[0].Files[0]})
	s := a.repositoryOperation
	if s == nil {
		t.Fatal("merge confirmation unavailable")
	}
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Choose mainline parent"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click(fmt.Sprintf("2 · %s", r.History[0].ParentHashes[1])); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if parent != 2 {
		t.Fatal("did not request second-parent comparison")
	}
	if err := tt.Click("Historical change parent-two-only.go"); err != nil {
		t.Fatal(err)
	}
	if len(s.op.Paths) != 1 || s.op.Paths[0] != "parent-two-only.go" {
		t.Fatal("second-parent-only change cannot be selected")
	}
}

func TestAdvancedHistoryConfirmSelectionAndDirtyGuard(t *testing.T) {
	a, r := historicalWorkspace(t)
	var got domain.RepositoryOperation
	a.OperateRepository = func(_ context.Context, _ domain.Repository, op domain.RepositoryOperation, _ func(domain.Repository, error, error)) {
		got = op
	}
	commit := r.History[0]
	a.repo().historicalSelectionRevision = commit.Hash
	a.repo().historicalChosen.Add("a.go")
	a.repo().historicalChosen.Add("b.go")
	target := commandTarget{source: commit.Hash, file: commit.Files[0]}
	a.run(cherryPickChanges, target)
	if a.repositoryOperation == nil || a.repositoryOperation.op.Kind != domain.CherryPickSelected || len(a.repositoryOperation.op.Paths) != 2 || got.Kind != 0 {
		t.Fatal("selection not captured before confirmation")
	}
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Run Cherry-Pick Selected Changes"); err != nil {
		t.Fatal(err)
	}
	if got.Kind != domain.CherryPickSelected || len(got.Paths) != 2 {
		t.Fatalf("wrong operation: %+v", got)
	}
	a.repositoryWrite = ""
	a.repositoryOperation = nil
	a.openCode("project", domain.File{Path: "a.go", New: "package main\n", DiskVersion: "disk"})
	a.repo().tabs[a.repo().active].code.Buffer.Set("// unsaved\npackage main\n")
	if a.disabledFor(getRevision, target) == "" || a.disabledFor(checkoutRevision, commandTarget{source: commit.Hash}) == "" {
		t.Fatal("unsaved source not protected")
	}
}

func TestHistoricalBrowserLateCallbacksAndRestore(t *testing.T) {
	a, r := historicalWorkspace(t)
	var complete func(domain.RevisionTree, error)
	var readContext context.Context
	a.ReadRepositoryRevisionTree = func(ctx context.Context, _ string, _ string, _ string, done func(domain.RevisionTree, error)) {
		readContext = ctx
		complete = done
	}
	a.ReadRepositoryFile = func(_ context.Context, _ string, source string, file domain.File, done func(domain.File, error)) {
		if source != "revision:"+r.History[0].Hash {
			t.Fatal("mutable source")
		}
		file.New = "historical"
		done(file, nil)
	}
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	a.run(showRepositoryRevision, commandTarget{source: r.History[0].Hash})
	if a.revisionBrowser == nil || complete == nil {
		t.Fatal("did not load historical tree")
	}
	complete(domain.RevisionTree{Revision: r.History[0].Hash, Entries: []domain.RevisionTreeEntry{{Path: "dir", Kind: "tree"}, {Path: "a.go", Kind: "blob", Mode: "100644"}}}, nil)
	s := a.revisionBrowser
	a.repo().historicalSelectionRevision = r.History[0].Hash
	a.repo().historicalChosen.Add("a.go")
	a.repo().historicalChosen.Add("b.go")
	s.selected = 1
	tt := native.NewTester(a.View, 1440, 900)
	if err := tt.Click("Restore Historical File"); err != nil {
		t.Fatal(err)
	}
	if a.revisionBrowser != nil || a.repositoryOperation == nil || a.repositoryOperation.op.Kind != domain.RestoreRevisionFiles || len(a.repositoryOperation.op.Paths) != 1 || a.repositoryOperation.op.Paths[0] != "a.go" {
		t.Fatal("restore did not confirm fixed file")
	}
	a.repositoryOperation = nil
	a.run(showRepositoryRevision, commandTarget{source: r.History[0].Hash})
	late := complete
	a.run(selectRepo, commandTarget{index: 0})
	if readContext.Err() == nil || a.revisionBrowser != nil {
		t.Fatal("repository navigation did not cancel tree")
	}
	late(domain.RevisionTree{Revision: r.History[0].Hash}, nil)
	if a.Model.Index != 0 || a.revisionBrowser != nil {
		t.Fatal("late tree callback changed current workspace")
	}
}

func TestSelectedHistoryProtectsRenamedDestinationBuffer(t *testing.T) {
	a, r := historicalWorkspace(t)
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	a.openCode("project", domain.File{Path: "renamed-destination.go", New: "disk", DiskVersion: "disk"})
	a.activeDocument().code.Buffer.Set("unsaved")
	target := commandTarget{source: r.History[0].Hash, file: r.History[0].Files[0]}
	if a.disabledFor(cherryPickChanges, target) == "" || a.disabledFor(revertChanges, target) == "" {
		t.Fatal("selected sequence allowed dirty potential rename destination")
	}
}

func TestSelectedHistoryEmptyFirstParentMergeAndLateComparison(t *testing.T) {
	a, r := historicalWorkspace(t)
	r.History[0].Merge = true
	r.History[0].Files = nil
	r.History[0].ParentHashes = []string{strings.Repeat("c", 40), strings.Repeat("d", 40)}
	a.Model.Active().Repository = r
	a.OperateRepository = func(context.Context, domain.Repository, domain.RepositoryOperation, func(domain.Repository, error, error)) {
	}
	var callbacks []func([]domain.File, error)
	var contexts []context.Context
	a.ReadRepositoryRevisionChanges = func(ctx context.Context, _ string, _ string, _ int, done func([]domain.File, error)) {
		contexts = append(contexts, ctx)
		callbacks = append(callbacks, done)
	}
	a.run(cherryPickChanges, commandTarget{source: r.History[0].Hash})
	s := a.repositoryOperation
	if s == nil {
		t.Fatal("merge with empty first-parent diff cannot open comparison")
	}
	s.mainline = s.mainlines[1]
	a.loadHistoryComparison(s)
	s.mainline = s.mainlines[2]
	a.loadHistoryComparison(s)
	if contexts[0].Err() == nil {
		t.Fatal("old parent request was not cancelled")
	}
	callbacks[0]([]domain.File{{Path: "stale.go", Status: "A"}}, nil)
	if len(s.historyChanges.files) != 0 {
		t.Fatal("old parent result installed")
	}
	callbacks[1]([]domain.File{{Path: "parent2.go", Status: "A"}}, nil)
	if len(s.historyChanges.files) != 1 || s.historyChanges.files[0].Path != "parent2.go" || !historyComparisonDisabled(s) {
		t.Fatal("parent2 result missing or unselected Run enabled")
	}
	s.mainline = s.mainlines[1]
	a.loadHistoryComparison(s)
	a.run(selectRepo, commandTarget{index: 0})
	if contexts[2].Err() == nil {
		t.Fatal("navigation did not cancel comparison")
	}
	callbacks[2]([]domain.File{{Path: "late.go", Status: "A"}}, nil)
	if a.repositoryOperation != nil || a.Model.Index != 0 {
		t.Fatal("late comparison changed navigated workspace")
	}
}
