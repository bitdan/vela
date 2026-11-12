package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vela/internal/domain"
)

var ErrPublishedAmend = errors.New("the last commit is reachable from a remote-tracking branch")

// Commit writes the current index using the repository's normal hooks and signing configuration.
func (g *Executor) Commit(ctx context.Context, snapshot domain.Repository, message string, amend bool) error {
	if strings.TrimSpace(message) == "" || strings.ContainsRune(message, 0) {
		return errors.New("enter a valid commit message")
	}
	if snapshot.Root == "" || snapshot.IndexVersion == "" {
		return errors.New("refresh the repository before committing")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	current, index, err := g.verifyCheckout(ctx, snapshot)
	if err != nil {
		return err
	}
	if current.CurrentBranch == "Detached HEAD" {
		return errors.New("check out a local branch before committing")
	}
	if err := g.checkCommitState(ctx, current.Root, index); err != nil {
		return err
	}
	if amend {
		if current.Head == "" {
			return errors.New("there is no commit to amend")
		}
		refs, err := g.read(ctx, current.Root, "for-each-ref", "--count=1", "--format=%(refname)",
			"--contains="+current.Head, "refs/remotes/")
		if err != nil {
			return err
		}
		if refs != "" {
			return ErrPublishedAmend
		}
	} else {
		diff, err := g.run(ctx, current.Root, "diff", "--cached", "--quiet", "--no-ext-diff", "--ignore-submodules=none",
			"--ita-invisible-in-index", "--")
		if err == nil {
			return errors.New("there are no staged changes to commit")
		}
		if diff.exitCode != 1 || ctx.Err() != nil {
			return err
		}
	}
	args := []string{"-c", "core.fsmonitor=false", "commit", "--file=-", "--cleanup=verbatim"}
	if amend {
		args = append(args, "--amend")
	}
	message = strings.ReplaceAll(message, "\r\n", "\n")
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	out, err := g.runInput(ctx, current.Root, message, args...)
	if err != nil && ctx.Err() == nil && strings.TrimSpace(out.stderr) != "" {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out.stderr))
	}
	return err
}

func (g *Executor) checkCommitState(ctx context.Context, root, index string) error {
	return g.checkOperationState(ctx, root, index, false)
}

func (g *Executor) checkOperationState(ctx context.Context, root, index string, allowBisect bool) error {
	for _, row := range nulFields(index) {
		fields, _, _ := strings.Cut(row, "\t")
		if !strings.HasSuffix(fields, " 0") {
			return errors.New("resolve the index conflicts before committing")
		}
	}
	dir, err := g.gitDirectory(ctx, root)
	if err != nil {
		return err
	}
	markers := []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-apply", "rebase-merge", "sequencer"}
	if !allowBisect {
		markers = append(markers, "BISECT_START")
	}
	for _, marker := range markers {
		if _, err := os.Lstat(filepath.Join(dir, marker)); err == nil {
			return errors.New("finish the in-progress Git operation before committing here")
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}
