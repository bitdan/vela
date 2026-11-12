package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) readBisect(ctx context.Context, root, dir string) (domain.BisectState, error) {
	var state domain.BisectState
	start, err := os.ReadFile(filepath.Join(dir, "BISECT_START"))
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.Original = strings.TrimSuffix(string(start), "\n")
	// The same range can be ended and restarted with identical content. The
	// start marker's write time distinguishes those search sessions as well.
	startInfo, err := os.Stat(filepath.Join(dir, "BISECT_START"))
	if err != nil {
		return state, err
	}
	h := sha256.New()
	h.Write(start)
	fmt.Fprintf(h, "\x00start-mtime:%d", startInfo.ModTime().UnixNano())
	var terms, names string
	for _, name := range []string{"BISECT_LOG", "BISECT_NAMES", "BISECT_TERMS", "BISECT_HEAD", "BISECT_EXPECTED_REV", "BISECT_ANCESTORS_OK"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return state, err
		}
		fmt.Fprintf(h, "\x00%s\x00%s", name, data)
		if name == "BISECT_TERMS" {
			terms = string(data)
		}
		if name == "BISECT_NAMES" {
			names = strings.TrimSpace(string(data))
		}
		if name == "BISECT_HEAD" && err == nil {
			state.NoCheckout = true
			state.Candidate = strings.TrimSpace(string(data))
		}
	}
	refs, err := g.read(ctx, root, "for-each-ref", "--sort=refname", "--format=%(refname)%00%(objectname)", "refs/bisect/")
	if err != nil {
		return state, err
	}
	h.Write([]byte(refs))
	var skipped []string
	for _, row := range strings.Split(strings.TrimSuffix(refs, "\n"), "\n") {
		ref, oid, ok := strings.Cut(row, "\x00")
		if !ok {
			continue
		}
		switch {
		case ref == "refs/bisect/bad":
			state.Bad = oid
		case strings.HasPrefix(ref, "refs/bisect/good-"):
			state.Good = append(state.Good, oid)
		case strings.HasPrefix(ref, "refs/bisect/skip-"):
			skipped = append(skipped, oid)
		}
	}
	state.Supported = state.NoCheckout && (terms == "" || terms == "bad\ngood\n") && (names == "" || names == "--" || names == "'--'")
	if !state.NoCheckout {
		candidate, err := g.read(ctx, root, "rev-parse", "--verify", "HEAD^{commit}")
		if err != nil {
			return state, err
		}
		state.Candidate = strings.TrimSpace(candidate)
	}
	original, err := g.read(ctx, root, "rev-parse", "--verify", "--end-of-options", state.Original+"^{commit}")
	if err != nil {
		return state, err
	}
	state.OriginalHead = strings.TrimSpace(original)
	h.Write([]byte(original))
	state.Version = fmt.Sprintf("%x", h.Sum(nil))
	if state.Supported && state.Bad != "" && len(state.Good) > 0 {
		args := []string{"rev-list", state.Bad, "--not"}
		args = append(args, state.Good...)
		args = append(args, "--")
		remaining, err := g.read(ctx, root, args...)
		if err != nil {
			return state, err
		}
		commits := strings.Fields(remaining)
		if len(commits) == 1 && !slices.Contains(skipped, commits[0]) {
			state.FirstBad = commits[0]
			state.Candidate = commits[0]
		}
		state.Ambiguous = len(commits) > 0 && slices.ContainsFunc(commits, func(oid string) bool { return slices.Contains(skipped, oid) }) &&
			!slices.ContainsFunc(commits, func(oid string) bool { return oid != state.Bad && !slices.Contains(skipped, oid) })
	}
	return state, nil
}

func (g *Executor) bisect(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	dir, err := g.gitDirectory(ctx, repo.Root)
	if err != nil {
		return err
	}
	state, err := g.readBisect(ctx, repo.Root, dir)
	if err != nil {
		return err
	}
	if state.Version != repo.Bisect.Version {
		return ErrStaleIndex
	}
	if err := g.checkOperationState(ctx, repo.Root, index, op.Kind != domain.StartBisect); err != nil {
		return err
	}
	if op.Kind == domain.StartBisect {
		if state.Active() {
			return errors.New("a bisect session is already active")
		}
		bad, err := g.resolveSource(ctx, repo, op.Source)
		if err != nil {
			return err
		}
		good, err := g.resolveSource(ctx, repo, op.Target)
		if err != nil {
			return err
		}
		ancestor, err := g.isAncestor(ctx, repo.Root, good, bad)
		if err != nil {
			return err
		}
		if good == bad || !ancestor {
			return errors.New("good must be a distinct ancestor of bad")
		}
		return g.runOperation(ctx, repo.Root, "bisect", "start", "--no-checkout", bad, good, "--")
	}
	if !state.Active() {
		return errors.New("no bisect session is active")
	}
	if op.Kind == domain.ResetBisect {
		if !state.NoCheckout {
			if err := g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
				return err
			}
			if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
				return err
			}
			if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, state.OriginalHead); err != nil {
				return err
			}
		}
		return g.runOperation(ctx, repo.Root, "-c", "submodule.recurse=false", "bisect", "reset")
	}
	if !state.Supported {
		return errors.New("classify this external checkout or custom-term bisect session using Git")
	}
	if state.FirstBad != "" || state.Ambiguous {
		return errors.New("the search has finished; reset Bisect before starting another search")
	}
	if state.Candidate == "" || op.Source != state.Candidate {
		return ErrStaleIndex
	}
	action := "good"
	switch op.Kind {
	case domain.BisectBad:
		action = "bad"
	case domain.BisectSkip:
		action = "skip"
	}
	return g.runOperation(ctx, repo.Root, "bisect", action, state.Candidate)
}
