package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestRevisionTreeAndDetachedCheckout(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	if err := os.MkdirAll(filepath.Join(f.root, "dir"), 0700); err != nil {
		t.Fatal(err)
	}
	f.write("dir/中文 space.txt", "historical\n")
	f.write("binary.dat", "binary\x00data")
	f.git("add", ".")
	f.git("commit", "-m", "historical tree")
	oid := f.git("rev-parse", "HEAD")
	f.write("dir/中文 space.txt", "current\n")
	f.git("commit", "-am", "current")
	before := f.read()
	ctx := context.Background()
	page, err := f.g.ReadRevisionTree(ctx, f.root, oid, "")
	if err != nil || page.Revision != oid || !slices.ContainsFunc(page.Entries, func(e domain.RevisionTreeEntry) bool { return e.Path == "dir" && e.Kind == "tree" }) {
		t.Fatalf("root tree: %+v %v", page, err)
	}
	page, err = f.g.ReadRevisionTree(ctx, f.root, oid, "dir")
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Path != "dir/中文 space.txt" {
		t.Fatalf("directory: %+v %v", page, err)
	}
	file, err := f.g.ReadFile(ctx, f.root, "revision:"+oid, domain.File{Path: page.Entries[0].Path})
	if err != nil || file.New != "historical\n" {
		t.Fatalf("immutable read: %+v %v", file, err)
	}
	if f.read().Head != before.Head || f.git("status", "--porcelain") != "" {
		t.Fatal("browse changed checkout")
	}
	for _, path := range []string{"../escape", ".git", "dir/../dir", "dir/中文 space.txt"} {
		if _, err := f.g.ReadRevisionTree(ctx, f.root, oid, path); err == nil {
			t.Fatalf("accepted invalid tree directory %q", path)
		}
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.CheckoutRevision, Source: oid}); err != nil {
		t.Fatal(err)
	}
	after := f.read()
	if after.Head != oid || after.CurrentBranch != "Detached HEAD" || f.git("rev-parse", "main") != before.Head {
		t.Fatal("detached checkout moved branch")
	}
	f.git("checkout", "main")
	f.write("文件 name.txt", "dirty\n")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CheckoutRevision, Source: oid}); err == nil {
		t.Fatal("checked out over dirty tracked file")
	}
}

func TestRestoreRevisionKeepsHeadAndIndex(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	if err := os.MkdirAll(filepath.Join(f.root, "deleted"), 0700); err != nil {
		t.Fatal(err)
	}
	f.write("binary.dat", "old\x00binary\xff")
	f.write("deleted/中文.txt", "restored\n")
	f.git("add", ".")
	f.git("commit", "-m", "historical")
	oid := f.git("rev-parse", "HEAD")
	f.write("binary.dat", "current\x00binary")
	f.git("rm", "deleted/中文.txt")
	f.git("commit", "-am", "current")
	before := f.read()
	op := domain.RepositoryOperation{Kind: domain.RestoreRevisionFiles, Source: oid, Paths: []string{"binary.dat", "deleted/中文.txt"}}
	if err := f.run(op); err != nil {
		t.Fatal(err)
	}
	after := f.read()
	if after.Head != before.Head || after.IndexVersion != before.IndexVersion || len(after.Staged) != 0 {
		t.Fatal("restore changed HEAD or Index")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "binary.dat"))
	if string(data) != "old\x00binary\xff" || f.git("show", "HEAD:binary.dat") != "current\x00binary" {
		t.Fatal("binary snapshot lost")
	}
	data, _ = os.ReadFile(filepath.Join(f.root, "deleted/中文.txt"))
	if string(data) != "restored\n" {
		t.Fatal("deleted current path not restored")
	}
	if err := f.run(op); err == nil {
		t.Fatal("restored over dirty tracked files")
	}
}

func TestHistoricalWriteSafety(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.write("ignored.txt", "historical\n")
	f.git("add", ".")
	f.git("commit", "-m", "target")
	oid := f.git("rev-parse", "HEAD")
	f.git("rm", "ignored.txt")
	f.write(".gitignore", "ignored.txt\n")
	f.git("add", ".")
	f.git("commit", "-m", "ignore local")
	f.write("ignored.txt", "local ignored\n")
	before := f.read()
	for _, kind := range []domain.RepositoryOperationKind{domain.CheckoutRevision, domain.RestoreRevisionFiles} {
		if err := f.run(domain.RepositoryOperation{Kind: kind, Source: oid, Paths: []string{"ignored.txt"}}); err == nil {
			t.Fatal("overwrote ignored file")
		}
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "ignored.txt"))
	if string(data) != "local ignored\n" || f.read().Head != before.Head {
		t.Fatal("refusal mutated checkout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := f.g.ExecuteRepositoryOperation(ctx, before, domain.RepositoryOperation{Kind: domain.CheckoutRevision, Source: oid}); err == nil {
		t.Fatal("cancelled checkout succeeded")
	}
	f.git("commit", "--allow-empty", "-m", "external")
	if err := f.g.ExecuteRepositoryOperation(context.Background(), before, domain.RepositoryOperation{Kind: domain.RestoreRevisionFiles, Source: oid, Paths: []string{"文件 name.txt"}}); err == nil {
		t.Fatal("stale HEAD accepted")
	}
	for _, path := range []string{"../escape", ".git/config", "missing.txt"} {
		if err := f.run(domain.RepositoryOperation{Kind: domain.RestoreRevisionFiles, Source: oid, Paths: []string{path}}); err == nil {
			t.Fatalf("unsafe restore %q", path)
		}
	}
}

func TestSelectedHistoricalChangesAndRevert(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	base := f.git("rev-parse", "HEAD")
	f.git("checkout", "-b", "topic")
	f.write("文件 name.txt", "selected\n")
	f.write("unselected.txt", "unselected\n")
	f.write("binary.dat", "new\x00binary")
	f.git("add", ".")
	f.git("commit", "--author", "Historical Author <history@example.test>", "-m", "source changes")
	oid := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	op := domain.RepositoryOperation{Kind: domain.CherryPickSelected, Source: oid, Paths: []string{"文件 name.txt", "binary.dat"}}
	if err := f.run(op); err != nil {
		t.Fatal(err)
	}
	if f.git("rev-parse", "HEAD^") != base || f.git("show", "HEAD:文件 name.txt") != "selected" || f.git("rev-parse", "topic") != oid {
		t.Fatal("wrong selected commit")
	}
	if _, err := os.Stat(filepath.Join(f.root, "unselected.txt")); !os.IsNotExist(err) {
		t.Fatal("unselected path included")
	}
	if !strings.Contains(f.git("log", "-1", "--format=%B"), oid) {
		t.Fatal("original provenance missing")
	}
	if f.git("log", "-1", "--format=%an <%ae>") != "Historical Author <history@example.test>" {
		t.Fatal("original author lost")
	}
	op.Kind = domain.RevertSelected
	if err := f.run(op); err != nil {
		t.Fatal(err)
	}
	if f.git("rev-parse", "HEAD^{tree}") != f.git("rev-parse", base+"^{tree}") {
		t.Fatal("selected revert restored wrong tree")
	}
	if !strings.Contains(f.git("log", "-1", "--format=%B"), oid) {
		t.Fatal("selected revert lost original commit provenance")
	}
}

func TestSelectedHistoricalRootAndMergeMainline(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	root := f.git("rev-parse", "HEAD")
	rootFiles, err := f.g.ReadRevisionChanges(context.Background(), f.root, root, 0)
	if err != nil || len(rootFiles) != 1 || rootFiles[0].Status != "A" {
		t.Fatalf("root delta: %+v %v", rootFiles, err)
	}
	f.git("checkout", "--orphan", "receiver")
	f.git("rm", "-rf", ".")
	f.git("commit", "--allow-empty", "-m", "empty receiver")
	empty := f.git("rev-parse", "HEAD^{tree}")
	op := domain.RepositoryOperation{Kind: domain.CherryPickSelected, Source: root, Paths: []string{"文件 name.txt"}}
	if err := f.run(op); err != nil {
		t.Fatal(err)
	}
	op.Kind = domain.RevertSelected
	if err := f.run(op); err != nil {
		t.Fatal(err)
	}
	if f.git("rev-parse", "HEAD^{tree}") != empty {
		t.Fatal("root subset roundtrip failed")
	}

	m := newWorkflow(t)
	base := m.git("rev-parse", "HEAD")
	m.git("checkout", "-b", "topic")
	m.write("topic.txt", "topic\n")
	m.git("add", ".")
	m.git("commit", "-m", "topic")
	m.git("checkout", "main")
	m.write("main.txt", "main\n")
	m.git("add", ".")
	m.git("commit", "-m", "main")
	m.git("merge", "--no-ff", "--no-edit", "topic")
	merge := m.git("rev-parse", "HEAD")
	mergeFiles, err := m.g.ReadRevisionChanges(context.Background(), m.root, merge, 2)
	if err != nil || len(mergeFiles) != 1 || mergeFiles[0].Path != "main.txt" {
		t.Fatalf("parent2 changes: %+v %v", mergeFiles, err)
	}
	m.git("checkout", "-b", "receiver", base)
	op = domain.RepositoryOperation{Kind: domain.CherryPickSelected, Source: merge, Paths: []string{"main.txt"}}
	if err := m.run(op); err == nil {
		t.Fatal("implicit merge parent accepted")
	}
	op.Mainline = 2
	if err := m.run(op); err != nil {
		t.Fatal(err)
	}
	if m.git("show", "HEAD:main.txt") != "main" {
		t.Fatal("mainline delta lost")
	}
	if _, err := os.Stat(filepath.Join(m.root, "topic.txt")); !os.IsNotExist(err) {
		t.Fatal("included other mainline changes")
	}
}

func TestSelectedHistoricalTargetSideRename(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("checkout", "-b", "topic")
	f.write("文件 name.txt", "incoming\n")
	f.git("commit", "-am", "incoming")
	oid := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	f.git("mv", "文件 name.txt", "destination.txt")
	f.git("commit", "-m", "rename on target")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CherryPickSelected, Source: oid, Paths: []string{"文件 name.txt"}}); err != nil {
		t.Fatal(err)
	}
	if f.git("show", "HEAD:destination.txt") != "incoming" {
		t.Fatal("target rename not followed")
	}
}

func TestSelectedHistoricalConflictContinueAndSkip(t *testing.T) {
	t.Parallel()
	for _, continueIt := range []bool{false, true} {
		t.Run(map[bool]string{false: "skip", true: "continue"}[continueIt], func(t *testing.T) {
			f := newWorkflow(t)
			f.git("checkout", "-b", "topic")
			f.write("文件 name.txt", "incoming\n")
			f.git("commit", "-am", "incoming")
			oid := f.git("rev-parse", "HEAD")
			f.git("checkout", "main")
			f.write("文件 name.txt", "current\n")
			f.git("commit", "-am", "current")
			before := f.read()
			if err := f.run(domain.RepositoryOperation{Kind: domain.CherryPickSelected, Source: oid, Paths: []string{"文件 name.txt"}}); err == nil {
				t.Fatal("expected conflict")
			}
			kind := domain.SkipSequence
			if continueIt {
				kind = domain.ContinueSequence
				f.write("文件 name.txt", "resolved\n")
				f.git("add", "--", "文件 name.txt")
			}
			if err := f.run(domain.RepositoryOperation{Kind: kind}); err != nil {
				t.Fatal(err)
			}
			if f.read().Sequencer.Active() {
				t.Fatal("sequencer still active")
			}
			if continueIt {
				if f.git("show", "HEAD:文件 name.txt") != "resolved" || f.git("rev-parse", "HEAD^") != before.Head {
					t.Fatal("continuation lost resolution")
				}
			} else if f.read().Head != before.Head {
				t.Fatal("skip moved original head")
			}
		})
	}
}

func TestHistoricalReadCancellationAndStaleWorktree(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	before := f.read()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.g.ReadRevisionTree(ctx, f.root, before.Head, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("read cancellation: %v", err)
	}
	f.write("文件 name.txt", "external\n")
	if err := f.g.ExecuteRepositoryOperation(context.Background(), before, domain.RepositoryOperation{Kind: domain.RestoreRevisionFiles, Source: before.Head, Paths: []string{"文件 name.txt"}}); err == nil {
		t.Fatal("stale working content overwritten")
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "external\n" {
		t.Fatal("stale rejection lost external content")
	}
}

func TestHistoricalObjectsIgnoreReplacementRefs(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	original := f.git("rev-parse", "HEAD")
	f.write("文件 name.txt", "replacement\n")
	f.write("replaced-only.txt", "other\n")
	f.git("add", ".")
	f.git("commit", "-m", "replacement")
	replacement := f.git("rev-parse", "HEAD")
	f.git("replace", original, replacement)
	page, err := f.g.ReadRevisionTree(context.Background(), f.root, original, "")
	if err != nil || len(page.Entries) != 1 {
		t.Fatalf("replacement changed immutable tree: %+v %v", page, err)
	}
	file, err := f.g.ReadFile(context.Background(), f.root, "revision:"+original, domain.File{Path: "文件 name.txt"})
	if err != nil || file.New != "base\n" {
		t.Fatalf("replacement changed immutable blob: %q %v", file.New, err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.RestoreRevisionFiles, Source: original, Paths: []string{"文件 name.txt"}}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(f.root, "文件 name.txt"))
	if string(data) != "base\n" {
		t.Fatal("restored replacement object")
	}
}

func TestHistoricalWindowsPathAliases(t *testing.T) {
	t.Parallel()
	if filepath.Separator != '\\' {
		t.Skip("Windows case-folding policy")
	}
	f := newWorkflow(t)
	makeObject := func(input string, args ...string) string {
		out, err := f.g.runInput(context.Background(), f.root, input, args...)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(out.stdout)
	}
	upper := makeObject("upper\n", "hash-object", "-w", "--stdin")
	lower := makeObject("lower\n", "hash-object", "-w", "--stdin")
	tree := makeObject("100644 blob "+upper+"\tA.txt\x00100644 blob "+lower+"\ta.txt\x00", "mktree", "-z")
	oid := makeObject("aliases\n", "commit-tree", tree)
	before := f.read()
	for _, op := range []domain.RepositoryOperation{{Kind: domain.RestoreRevisionFiles, Source: oid, Paths: []string{"A.txt", "a.txt"}}, {Kind: domain.CheckoutRevision, Source: oid}} {
		if err := f.run(op); err == nil {
			t.Fatal("accepted colliding historical paths")
		}
		if after := f.read(); after.Head != before.Head || after.IndexVersion != before.IndexVersion || f.git("status", "--porcelain") != "" {
			t.Fatal("collision rejection mutated checkout")
		}
	}
	f.write("a.txt", "current\n")
	f.git("add", ".")
	f.git("commit", "-m", "tracked lowercase")
	if err := f.run(domain.RepositoryOperation{Kind: domain.RestoreRevisionFiles, Source: oid, Paths: []string{"A.txt"}}); err == nil {
		t.Fatal("historical path aliased distinct current tracked file")
	}
}

func TestSelectedHistoricalRenameConflictAndAbort(t *testing.T) {
	t.Parallel()
	f := newWorkflow(t)
	f.git("checkout", "-b", "topic")
	f.git("mv", "文件 name.txt", "renamed.txt")
	f.write("other.txt", "exclude\n")
	f.git("add", ".")
	f.git("commit", "-m", "rename")
	rename := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	if err := f.run(domain.RepositoryOperation{Kind: domain.CherryPickSelected, Source: rename, Paths: []string{"renamed.txt"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "文件 name.txt")); !os.IsNotExist(err) {
		t.Fatal("rename source retained")
	}
	if _, err := os.Stat(filepath.Join(f.root, "other.txt")); !os.IsNotExist(err) {
		t.Fatal("unselected addition included")
	}
	f.git("checkout", "topic")
	f.write("renamed.txt", "incoming\n")
	f.git("commit", "-am", "incoming")
	oid := f.git("rev-parse", "HEAD")
	f.git("checkout", "main")
	f.write("renamed.txt", "current\n")
	f.git("commit", "-am", "current")
	before := f.read()
	if err := f.run(domain.RepositoryOperation{Kind: domain.CherryPickSelected, Source: oid, Paths: []string{"renamed.txt"}}); err == nil {
		t.Fatal("expected conflict")
	}
	paused := f.read()
	if !paused.Sequencer.Active() || len(paused.Conflicts) != 1 {
		t.Fatal("no real sequencer recovery state")
	}
	if _, err := f.g.ReadMergeDocument(context.Background(), paused, "renamed.txt"); err != nil {
		t.Fatal(err)
	}
	if err := f.run(domain.RepositoryOperation{Kind: domain.AbortSequence}); err != nil {
		t.Fatal(err)
	}
	if f.read().Head != before.Head || f.git("status", "--porcelain") != "" {
		t.Fatal("abort failed to restore original state")
	}
}
