package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vela/internal/domain"
)

func TestExternalMultiCommitSequenceProtectsRelocatedIgnoredFile(t *testing.T) {
	f := newWorkflow(t)
	ctx := context.Background()
	originalBranch := f.git("branch", "--show-current")
	f.write("conflict.txt", "base\n")
	if err := os.Mkdir(filepath.Join(f.root, "old"), 0700); err != nil {
		t.Fatal(err)
	}
	f.write("old/tracked.txt", "same\n")
	f.write(".gitignore", "new/added.txt\n")
	f.git("add", "--", "conflict.txt", "old/tracked.txt", ".gitignore")
	f.git("commit", "-m", "base")
	f.git("checkout", "-b", "upstream")
	f.write("conflict.txt", "incoming\n")
	f.git("commit", "-am", "conflict")
	first := f.git("rev-parse", "HEAD")
	f.write("old/added.txt", "incoming contents\n")
	f.git("add", "--", "old/added.txt")
	f.git("commit", "-m", "add to old directory")
	second := f.git("rev-parse", "HEAD")
	f.git("checkout", originalBranch)
	f.git("mv", "--", "old", "new")
	f.write("conflict.txt", "local\n")
	f.git("commit", "-am", "rename and diverge")
	f.git("config", "merge.directoryRenames", "true")
	f.write("new/added.txt", "must preserve\n")
	if _, err := f.g.run(ctx, f.root, "cherry-pick", first, second); err == nil {
		t.Fatal("expected a paused external sequence")
	}
	r := f.read()
	err := f.g.ExecuteRepositoryOperation(ctx, r, domain.RepositoryOperation{Kind: domain.SkipSequence})
	if err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("unsafe recovery was executed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(f.root, "new", "added.txt"))
	if err != nil || string(data) != "must preserve\n" || f.git("rev-parse", "HEAD") != r.Head {
		t.Fatalf("recovery changed an ignored file or HEAD: %q %v", data, err)
	}
}

func TestExternalMultiCommitSequenceRecovery(t *testing.T) {
	for _, kind := range []string{"cherry-pick", "revert"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := newWorkflow(t)
			ctx := context.Background()
			branch := f.git("branch", "--show-current")
			path, laterPath := "冲突 file.txt", "后续 file.txt"
			f.write(path, "base\n")
			f.git("add", "--", path)
			f.git("commit", "-m", "base")
			base := f.git("rev-parse", "HEAD")
			f.git("checkout", "-b", "incoming")
			f.write(path, "incoming\n")
			f.git("commit", "-am", "first")
			first := f.git("rev-parse", "HEAD")
			f.write(laterPath, "later\n")
			f.git("add", "--", laterPath)
			f.git("commit", "-m", "second")
			second := f.git("rev-parse", "HEAD")
			replacement := ""
			if kind == "cherry-pick" {
				f.git("checkout", "-b", "substitute", first)
				f.write("replacement.txt", "replacement\n")
				f.git("add", "--", "replacement.txt")
				f.git("commit", "-m", "replacement")
				replacement = f.git("rev-parse", "HEAD")
			}
			start := base
			if kind == "revert" {
				start = second
			}
			f.git("checkout", branch)
			f.git("reset", "--hard", start)
			f.write(path, "local\n")
			f.git("commit", "-am", "local")
			original := f.git("rev-parse", "HEAD")
			pause := func() domain.Repository {
				t.Helper()
				if _, err := f.g.run(ctx, f.root, kind, "--no-edit", first, second); err == nil {
					t.Fatal("expected external conflict")
				}
				r := f.read()
				if r.Sequencer.Kind != kind || r.Sequencer.Remaining != 2 || r.Sequencer.Commit != first || len(r.Conflicts) != 1 {
					t.Fatalf("wrong external sequence: %+v", r.Sequencer)
				}
				return r
			}
			run := func(r domain.Repository, action domain.RepositoryOperationKind) error {
				return f.g.ExecuteRepositoryOperation(ctx, r, domain.RepositoryOperation{Kind: action})
			}
			r := pause()
			todo := filepath.Join(r.GitDir, "sequencer", "todo")
			raw, err := os.ReadFile(todo)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(todo, append(append([]byte{}, raw...), '\n'), 0600); err != nil {
				t.Fatal(err)
			}
			if err := run(r, domain.AbortSequence); !errors.Is(err, ErrStaleIndex) {
				t.Fatalf("stale sequence not rejected: %v", err)
			}
			if err := run(f.read(), domain.AbortSequence); err != nil || f.git("rev-parse", "HEAD") != original {
				t.Fatalf("abort did not restore original: %v", err)
			}
			r = pause()
			if kind == "cherry-pick" {
				f.write(laterPath, "untracked\n")
				if err := run(f.read(), domain.SkipSequence); err == nil || f.git("rev-parse", "HEAD") != original {
					t.Fatal("skip accepted a later collision")
				}
				if err := os.Remove(filepath.Join(r.Root, laterPath)); err != nil {
					t.Fatal(err)
				}
				f.git("replace", second, replacement)
				if err := run(f.read(), domain.SkipSequence); err != nil {
					t.Fatal(err)
				}
				if f.git("show", "HEAD:"+path) != "local" || f.git("show", "HEAD:replacement.txt") != "replacement" || f.git("ls-tree", "HEAD", "--", laterPath) != "" {
					t.Fatal("skip did not continue the same sequence Git started")
				}
			} else {
				f.write(path, "resolved\n")
				f.git("add", "--", path)
				if err := run(f.read(), domain.ContinueSequence); err != nil {
					t.Fatal(err)
				}
				if f.git("show", "HEAD:"+path) != "resolved" || f.git("ls-tree", "HEAD", "--", laterPath) != "" {
					t.Fatal("continue did not apply remaining revert")
				}
			}
			if after := f.read(); after.Sequencer.Active() || len(after.Working) != 0 || len(after.Staged) != 0 {
				t.Fatal("sequence ended with stale recovery or dirty state")
			}
		})
	}
}
