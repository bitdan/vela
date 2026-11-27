package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"vela/internal/domain"
)

func sequencerAt(gitDir string) (domain.SequencerState, error) {
	return sequencerSnapshotAt(gitDir, nil)
}

func sequencerSnapshotAt(gitDir string, pending *[]string) (domain.SequencerState, error) {
	var state domain.SequencerState
	hash := sha256.New()
	for _, marker := range []struct{ name, kind string }{
		{"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"},
	} {
		data, err := os.ReadFile(filepath.Join(gitDir, marker.name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return state, err
		}
		commit := strings.TrimSpace(string(data))
		if state.Active() || !validObjectID(commit) {
			return state, errors.New("invalid cherry-pick or revert metadata")
		}
		state.Kind, state.Commit = marker.kind, commit
		fmt.Fprintf(hash, "%s\x00%s\x00", marker.name, data)
	}
	for _, name := range []string{"head", "abort-safety", "todo", "opts"} {
		data, err := os.ReadFile(filepath.Join(gitDir, "sequencer", name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return state, err
		}
		fmt.Fprintf(hash, "%s\x00%s\x00", name, data)
		if name == "head" {
			state.Original = strings.TrimSpace(string(data))
			if !validObjectID(state.Original) {
				return state, errors.New("invalid sequencer original HEAD")
			}
		}
		if name == "todo" {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
					continue
				}
				if len(fields) < 2 || (fields[0] != "pick" && fields[0] != "revert") {
					return state, errors.New("unsupported Git sequencer command")
				}
				kind := "cherry-pick"
				if fields[0] == "revert" {
					kind = "revert"
				}
				if state.Active() && state.Kind != kind {
					return state, errors.New("mixed Git sequencer commands are unsupported")
				}
				state.Remaining++
				if pending != nil && len(*pending) < domain.MaxSequenceRecovery {
					*pending = append(*pending, fields[1])
				}
				if !state.Active() {
					state.Kind, state.Commit = kind, fields[1]
				}
			}
		}
	}
	if state.Active() {
		state.Version = fmt.Sprintf("%x", hash.Sum(nil))
	}
	return state, nil
}

func (g *Executor) verifySequencer(ctx context.Context, repo domain.Repository) error {
	_, err := g.verifySequenceTodo(ctx, repo)
	return err
}

func (g *Executor) verifySequenceTodo(ctx context.Context, repo domain.Repository) ([]string, error) {
	dir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return nil, err
	}
	if dir != repo.GitDir {
		return nil, ErrStaleIndex
	}
	var pending []string
	state, err := sequencerSnapshotAt(dir, &pending)
	if err != nil {
		return nil, err
	}
	if !state.Active() || state != repo.Sequencer {
		return nil, ErrStaleIndex
	}
	return pending, nil
}

func (g *Executor) sequence(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if op.Kind == domain.StartCherryPick || op.Kind == domain.StartRevert {
		if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" {
			return errors.New("check out a local branch with a commit first")
		}
		if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
			return err
		}
		if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
			return err
		}
		commit, err := g.resolveSource(ctx, repo, op.Source)
		if err != nil {
			return err
		}
		line, err := g.read(ctx, repo.Root, "rev-list", "--parents", "--max-count=1", commit, "--")
		if err != nil {
			return err
		}
		parents := strings.Fields(line)
		if len(parents) == 0 {
			return errors.New("the selected commit is unavailable")
		}
		args := []string{"--no-edit", "--no-rerere-autoupdate"}
		protected, err := domain.ProtectedBranch(op.Safety.ProtectedBranches, repo.CurrentBranch)
		if err != nil {
			return err
		}
		if op.Kind == domain.StartCherryPick && op.Safety.CherryPickSuffix && protected {
			args = append(args, "-x")
		}
		if len(parents) > 2 {
			if op.Mainline < 1 || op.Mainline >= len(parents) {
				return errors.New("choose the mainline parent for this merge commit")
			}
			args = append(args, "--mainline", strconv.Itoa(op.Mainline))
		} else if op.Mainline != 0 {
			return errors.New("mainline is only supported for merge commits")
		}
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, commit); err != nil {
			return err
		}
		kind := "cherry-pick"
		if op.Kind == domain.StartRevert {
			kind = "revert"
			// Revert can restore paths present only in the selected parent.
			if len(parents) > 1 {
				parent := 1
				if op.Mainline > 0 {
					parent = op.Mainline
				}
				if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, parents[parent]); err != nil {
					return err
				}
			}
		}
		out, runErr := g.sequenceCommand(ctx, repo.Root, kind, append(args, commit)...)
		err = operationError(ctx, out, runErr)
		if err != nil && out.exitCode == 1 && ctx.Err() == nil && op.Kind == domain.StartRevert && g.revertUnchanged(ctx, repo) {
			return fmt.Errorf("%w: %w", domain.ErrRevertUnchanged, err)
		}
		return err
	}
	pending, err := g.verifySequenceTodo(ctx, repo)
	if err != nil {
		return err
	}
	if op.Kind == domain.ContinueSequence && len(conflictsFromIndex(index)) != 0 {
		return errors.New("resolve all conflicted files before continuing")
	}
	multiReplay := op.Kind != domain.AbortSequence && len(pending) > 1
	var replacements string
	if multiReplay {
		replacements, err = g.read(ctx, repo.Root, "for-each-ref", "--format=%(refname)%00%(objectname)", "refs/replace/")
		if err != nil {
			return err
		}
	}
	current, err := g.resolveSequenceCommits(ctx, repo.Root, []string{repo.Sequencer.Commit})
	if err != nil {
		return err
	}
	if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, current[0]); err != nil {
		return err
	}
	if repo.Sequencer.Original != "" {
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, repo.Sequencer.Original); err != nil {
			return err
		}
	}
	if op.Kind != domain.AbortSequence && len(pending) > 0 {
		if repo.Sequencer.Remaining > domain.MaxSequenceRecovery {
			return errors.New("continue or skip sequences longer than 100 commits using Git")
		}
		if err := g.protectSequencePaths(ctx, repo.Root, pending); err != nil {
			return err
		}
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	if err := g.verifySequencer(ctx, repo); err != nil {
		return err
	}
	if multiReplay {
		if err := g.verifyUpdateMetadata(ctx, repo); err != nil {
			return err
		}
		refs, err := g.read(ctx, repo.Root, "for-each-ref", "--format=%(refname)%00%(objectname)", "refs/replace/")
		if err != nil {
			return err
		}
		if refs != replacements {
			return ErrStaleIndex
		}
		boundary, err := commitLogBoundary(repo.CommonDir)
		if err != nil {
			return err
		}
		if boundary != repo.LogSnapshot.Boundary {
			return errors.New("repository ancestry changed; refresh before continuing the sequence")
		}
		// Native merge can synthesize a new path for a directory rename or
		// file/directory conflict and overwrite ignored files at that path.
		ignored, err := g.read(ctx, repo.Root, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
		if err != nil {
			return err
		}
		if ignored != "" {
			return errors.New("move ignored files before continuing: directory or file conflicts in later commits can overwrite them")
		}
	}
	switch op.Kind {
	case domain.ContinueSequence:
		return g.runSequence(ctx, repo.Root, repo.Sequencer.Kind, "--continue")
	case domain.SkipSequence:
		return g.runSequence(ctx, repo.Root, repo.Sequencer.Kind, "--skip")
	case domain.AbortSequence:
		return g.runSequence(ctx, repo.Root, repo.Sequencer.Kind, "--abort")
	}
	return errors.New("unknown sequencer operation")
}

func (g *Executor) runSequence(ctx context.Context, root, kind string, args ...string) error {
	out, err := g.sequenceCommand(ctx, root, kind, args...)
	return operationError(ctx, out, err)
}

func (g *Executor) sequenceCommand(ctx context.Context, root, kind string, args ...string) (result, error) {
	if kind != "cherry-pick" && kind != "revert" {
		return result{}, errors.New("unknown sequencer kind")
	}
	return g.runInputWithEnv(ctx, root, "", []string{"GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:"},
		append([]string{"-c", "submodule.recurse=false", kind}, args...)...)
}

// An empty revert may leave no sequencer files. Classify its observable result
// without matching Git's translated diagnostics, retaining those diagnostics.
func (g *Executor) revertUnchanged(ctx context.Context, repo domain.Repository) bool {
	_, index, err := g.verifyCheckout(ctx, repo)
	if err != nil || g.requireCleanTracked(ctx, repo.Root) != nil {
		return false
	}
	return g.checkCommitState(ctx, repo.Root, index) == nil
}
