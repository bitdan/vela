package gitcli

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestCherryPickAndRevertRecovery(t *testing.T) {
	t.Parallel()
	ctx, dir, g := context.Background(), t.TempDir(), New()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	path := "中文 file.txt"
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(value+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("config", "user.name", "Vela Test")
	git("config", "user.email", "vela@example.test")
	git("config", "commit.gpgsign", "false")
	write("base")
	git("add", "--", path)
	git("commit", "-m", "base")
	git("checkout", "-b", "topic")
	write("topic")
	git("commit", "-am", "topic change")
	topic := git("rev-parse", "HEAD")
	git("checkout", "main")
	write("main")
	git("commit", "-am", "main change")
	original := git("rev-parse", "HEAD")
	read := func() domain.Repository {
		t.Helper()
		repo, err := g.ReadRepository(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	run := func(kind domain.RepositoryOperationKind, source string) error {
		return g.ExecuteRepositoryOperation(ctx, read(), domain.RepositoryOperation{Kind: kind, Source: source})
	}
	start := func() {
		t.Helper()
		if err := run(domain.StartCherryPick, topic); err == nil {
			t.Fatal("expected a cherry-pick conflict")
		}
		if state := read().Sequencer; state.Kind != "cherry-pick" || state.Commit != topic {
			t.Fatalf("wrong cherry-pick state: %+v", state)
		}
	}
	start()
	stale := read()
	marker := filepath.Join(dir, ".git", "CHERRY_PICK_HEAD")
	if err := os.WriteFile(marker, []byte(original+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := g.ExecuteRepositoryOperation(ctx, stale, domain.RepositoryOperation{Kind: domain.AbortSequence}); err != ErrStaleIndex {
		t.Fatalf("stale sequencer state accepted: %v", err)
	}
	if err := os.WriteFile(marker, []byte(topic+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(domain.ContinueSequence, ""); err == nil {
		t.Fatal("continued before resolving the conflict")
	}
	if err := run(domain.AbortSequence, ""); err != nil || git("rev-parse", "HEAD") != original {
		t.Fatalf("abort failed to restore HEAD: %v", err)
	}
	start()
	if err := run(domain.SkipSequence, ""); err != nil || git("rev-parse", "HEAD") != original {
		t.Fatalf("skip failed: %v", err)
	}
	start()
	doc, err := g.ReadMergeDocument(ctx, read(), path)
	if err != nil || doc.Current != "main\n" || doc.Incoming != "topic\n" {
		t.Fatalf("wrong cherry-pick sides: %+v, %v", doc, err)
	}
	if err := run(domain.AcceptIncoming, path); err != nil {
		t.Fatal(err)
	}
	if err := run(domain.ContinueSequence, ""); err != nil || git("rev-parse", "HEAD^") != original {
		t.Fatalf("continue failed: %v", err)
	}
	picked := git("rev-parse", "HEAD")
	write("later edit")
	git("commit", "-am", "later")
	later := git("rev-parse", "HEAD")
	if err := run(domain.StartRevert, picked); err == nil || read().Sequencer.Kind != "revert" {
		t.Fatalf("expected a revert conflict: %v", err)
	}
	if err := run(domain.AbortSequence, ""); err != nil || git("rev-parse", "HEAD") != later {
		t.Fatalf("revert abort failed: %v", err)
	}
	if err := run(domain.StartRevert, picked); err == nil {
		t.Fatal("expected another revert conflict")
	}
	if err := run(domain.AcceptIncoming, path); err != nil {
		t.Fatal(err)
	}
	if err := run(domain.ContinueSequence, ""); err != nil || git("rev-parse", "HEAD^") != later || git("show", "HEAD:"+path) != "main" {
		t.Fatalf("revert resolution failed: %v", err)
	}
	if read().Sequencer.Active() {
		t.Fatal("sequencer state remained after success")
	}
}

func TestSequencerSafetyAndCleanOperations(t *testing.T) {
	t.Parallel()
	ctx, dir, g := context.Background(), t.TempDir(), New()
	if _, err := g.Initialize(ctx, dir, "main"); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(path, value string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, path)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, path), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	read := func() domain.Repository {
		t.Helper()
		repo, err := g.ReadRepository(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		return repo
	}
	run := func(kind domain.RepositoryOperationKind, source string, mainline int) error {
		return g.ExecuteRepositoryOperation(ctx, read(), domain.RepositoryOperation{Kind: kind, Source: source, Mainline: mainline})
	}
	git("config", "user.name", "Vela Test")
	git("config", "user.email", "vela@example.test")
	git("config", "commit.gpgsign", "false")
	write("base.txt", "base\n")
	git("add", ".")
	git("commit", "-m", "base")
	base := git("rev-parse", "HEAD")
	git("branch", "target")
	git("checkout", "-b", "topic")
	write("new 文件.txt", "new\n")
	git("add", ".")
	git("commit", "-m", "add file")
	topic := git("rev-parse", "HEAD")
	git("checkout", "main")
	write("base.txt", "dirty\n")
	before := read()
	if err := run(domain.StartCherryPick, topic, 0); err == nil {
		t.Fatal("started with dirty tracked files")
	}
	if git("rev-parse", "HEAD") != base || read().IndexVersion != before.IndexVersion {
		t.Fatal("dirty refusal changed HEAD or index")
	}
	write("base.txt", "base\n")
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := g.ExecuteRepositoryOperation(cancelled, read(), domain.RepositoryOperation{Kind: domain.StartCherryPick, Source: topic}); err == nil {
		t.Fatal("cancelled operation succeeded")
	}
	if git("rev-parse", "HEAD") != base {
		t.Fatal("cancelled operation changed HEAD")
	}
	if err := run(domain.StartCherryPick, topic, 0); err != nil {
		t.Fatal(err)
	}
	picked := git("rev-parse", "HEAD")
	if git("rev-parse", "HEAD^") != base || git("show", "HEAD:new 文件.txt") != "new" {
		t.Fatal("wrong cherry-picked tree or parent")
	}
	if err := run(domain.StartRevert, picked, 0); err != nil || git("rev-parse", "HEAD^{tree}") != git("rev-parse", base+"^{tree}") {
		t.Fatalf("clean revert failed: %v", err)
	}
	reverted := git("rev-parse", "HEAD")
	if err := run(domain.StartRevert, picked, 0); !errors.Is(err, domain.ErrRevertUnchanged) ||
		git("rev-parse", "HEAD") != reverted || read().Sequencer.Active() {
		t.Fatalf("empty revert was not reported as unchanged: %v", err)
	}
	if err := run(domain.StartCherryPick, topic, 0); err != nil {
		t.Fatal(err)
	}
	if err := run(domain.StartCherryPick, topic, 0); err == nil || !read().Sequencer.Active() || len(read().Conflicts) != 0 {
		t.Fatalf("empty cherry-pick did not stop for a decision: %v", err)
	}
	if err := run(domain.SkipSequence, "", 0); err != nil || read().Sequencer.Active() {
		t.Fatalf("could not skip empty commit: %v", err)
	}
	git("checkout", "-b", "feature")
	write("second.txt", "feature\n")
	git("add", ".")
	git("commit", "-m", "feature")
	git("checkout", "main")
	git("merge", "--no-ff", "--no-edit", "feature")
	merge := git("rev-parse", "HEAD")
	git("checkout", "target")
	if err := run(domain.StartCherryPick, merge, 0); err == nil || git("rev-parse", "HEAD") != base {
		t.Fatal("merge commit did not require a mainline")
	}
	if err := run(domain.StartCherryPick, merge, 1); err != nil || git("show", "HEAD:second.txt") != "feature" {
		t.Fatalf("mainline cherry-pick failed: %v", err)
	}
	if err := run(domain.StartRevert, merge, 1); err != nil || git("rev-parse", "HEAD^{tree}") != git("rev-parse", base+"^{tree}") {
		t.Fatalf("mainline revert failed: %v", err)
	}
	git("checkout", "-b", "generated-source")
	write("ignored.txt", "tracked upstream\n")
	git("add", ".")
	git("commit", "-m", "generated file")
	generated := git("rev-parse", "HEAD")
	git("checkout", "target")
	write(".git/info/exclude", "ignored.txt\n")
	write("ignored.txt", "local content\n")
	if err := run(domain.StartCherryPick, generated, 0); err == nil || !strings.Contains(err.Error(), "ignored") {
		t.Fatalf("ignored file was not protected: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "ignored.txt"))
	if err != nil || string(data) != "local content\n" {
		t.Fatal("ignored file was overwritten")
	}
	if err := run(domain.StartRevert, base, 0); err != nil {
		t.Fatalf("root commit revert failed: %v", err)
	}
	rootReverted := git("rev-parse", "HEAD")
	if err := run(domain.StartRevert, base, 0); !errors.Is(err, domain.ErrRevertUnchanged) ||
		git("rev-parse", "HEAD") != rootReverted || read().Sequencer.Active() {
		t.Fatalf("empty root revert was not reported as unchanged: %v", err)
	}
}
