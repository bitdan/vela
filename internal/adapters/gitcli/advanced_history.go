package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) historicalOperation(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	ctx = canonicalHistory(ctx)
	if repo.Head == "" {
		return errors.New("create a commit before changing historical files")
	}
	if err := g.fixedCommit(ctx, repo.Root, op.Source); err != nil {
		return err
	}
	if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	if err := g.rejectHiddenIndexFlags(ctx, repo.Root); err != nil {
		return err
	}
	if err := rejectSubmoduleIndex(index); err != nil {
		return err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	if err := g.requireCleanTracked(ctx, repo.Root); err != nil {
		return err
	}
	if op.Kind == domain.CheckoutRevision {
		if err := g.validateHistoricalCheckoutPaths(ctx, repo.Root, op.Source); err != nil {
			return err
		}
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, op.Source); err != nil {
			return err
		}
		if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
			return err
		}
		if err := g.verifyWorktree(ctx, repo); err != nil {
			return err
		}
		return g.runOperation(ctx, repo.Root, "checkout", "--detach", "--no-guess", "--no-overwrite-ignore", "--no-recurse-submodules", op.Source, "--")
	}
	paths, err := historyPaths(op.Paths)
	if err != nil {
		return err
	}
	if op.Kind == domain.CherryPickSelected || op.Kind == domain.RevertSelected {
		return g.sequenceSelected(ctx, repo, index, op, paths)
	}
	if op.Kind != domain.RestoreRevisionFiles {
		return errors.New("unknown historical operation")
	}
	files := historyFiles(paths)
	if err := rejectHistoryIndexAliases(paths, index); err != nil {
		return err
	}
	if err := g.validateShelfPaths(ctx, repo, files, index); err != nil {
		return fmt.Errorf("historical path preflight: %w", err)
	}
	for _, path := range paths {
		out, err := g.read(ctx, repo.Root, "--literal-pathspecs", "ls-tree", "-z", op.Source, "--", path)
		if err != nil {
			return err
		}
		records := nulFields(out)
		if len(records) != 1 {
			return errors.New("select an existing regular file at this revision")
		}
		header, name, _ := strings.Cut(records[0], "\t")
		if name != path || (!strings.HasPrefix(header, "100644 blob ") && !strings.HasPrefix(header, "100755 blob ")) {
			return errors.New("restore supports existing regular historical files only")
		}
	}
	if err := g.protectSelectedPaths(ctx, repo.Root, paths); err != nil {
		return err
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	input := strings.Join(paths, "\x00") + "\x00"
	if _, err := g.runInput(ctx, repo.Root, input, "--literal-pathspecs", "-c", "submodule.recurse=false", "restore", "--source="+op.Source, "--worktree", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		return fmt.Errorf("historical restore did not complete; inspect working files before retrying: %w", err)
	}
	return nil
}

func historyPaths(paths []string) ([]string, error) {
	if len(paths) == 0 || len(paths) > 100 {
		return nil, errors.New("select 1 to 100 whole-file changes")
	}
	var result []string
	bytes := 0
	keys := map[string]string{}
	for _, path := range paths {
		if !validGitPath(path) || strings.ContainsRune(path, 0) {
			return nil, errors.New("invalid historical path")
		}
		if err := historyPlatformPath(path); err != nil {
			return nil, err
		}
		key := rebasePathKey(path)
		if old, ok := keys[key]; ok && old != path {
			return nil, errors.New("historical selection contains platform path aliases")
		}
		keys[key] = path
		if !slices.Contains(result, path) {
			result = append(result, path)
			bytes += len(path) + 1
		}
	}
	if bytes > 16384 {
		return nil, errors.New("historical paths exceed 16 KiB")
	}
	slices.Sort(result)
	return result, nil
}

func historyFiles(paths []string) []domain.File {
	files := make([]domain.File, len(paths))
	for i, path := range paths {
		files[i].Path = path
	}
	return files
}

func (g *Executor) protectSelectedPaths(ctx context.Context, root string, paths []string) error {
	out, err := g.read(ctx, root, "ls-files", "--others", "-z")
	if err != nil {
		return err
	}
	for _, local := range nulFields(out) {
		local = rebasePathKey(strings.TrimSuffix(local, "/"))
		for _, path := range paths {
			path = rebasePathKey(path)
			if local == path || strings.HasPrefix(local, path+"/") || strings.HasPrefix(path, local+"/") {
				return errors.New("move untracked or ignored files before restoring this path")
			}
		}
	}
	return nil
}

// sequenceSelected materializes only the chosen delta in an isolated index.
// Git's normal sequencer then owns commit creation and conflict recovery.
func (g *Executor) sequenceSelected(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation, paths []string) error {
	if repo.CurrentBranch == "Detached HEAD" {
		return errors.New("check out a local branch before applying selected changes")
	}
	line, err := g.read(ctx, repo.Root, "rev-list", "--parents", "--max-count=1", op.Source, "--")
	if err != nil {
		return err
	}
	parents := strings.Fields(line)
	if len(parents) == 0 {
		return errors.New("selected commit is unavailable")
	}
	base := ""
	if len(parents) > 2 {
		if op.Mainline < 1 || op.Mainline >= len(parents) {
			return errors.New("choose the merge mainline parent")
		}
		base = parents[op.Mainline]
	} else {
		if op.Mainline != 0 {
			return errors.New("mainline is only supported for merge commits")
		}
		if len(parents) == 2 {
			base = parents[1]
		}
	}
	if base == "" {
		out, err := g.runInput(ctx, repo.Root, "", "hash-object", "-w", "-t", "tree", "--stdin")
		if err != nil {
			return err
		}
		base = strings.TrimSpace(out.stdout)
	}
	// Read rename identities before validating membership; selecting either side
	// applies the complete rename instead of leaving a duplicate source behind.
	diffResult, err := g.runInputWithEnvLimit(ctx, repo.Root, "", nil, 2<<20, "diff-tree", "--no-commit-id", "--name-status", "-r", "-z", "--find-renames=50%", "--no-ext-diff", "--no-textconv", base, op.Source, "--")
	if err != nil {
		return err
	}
	changed := map[string]bool{}
	fields := nulFields(diffResult.stdout)
	for i := 0; i < len(fields); {
		status := fields[i]
		i++
		if i >= len(fields) || status == "" {
			return errors.New("invalid historical diff record")
		}
		path := fields[i]
		i++
		changed[path] = true
		if status[0] == 'R' || status[0] == 'C' {
			if i >= len(fields) {
				return errors.New("invalid historical rename")
			}
			next := fields[i]
			i++
			changed[next] = true
			if status[0] == 'R' && (slices.Contains(paths, path) || slices.Contains(paths, next)) {
				if !slices.Contains(paths, path) {
					paths = append(paths, path)
				}
				if !slices.Contains(paths, next) {
					paths = append(paths, next)
				}
			}
		}
	}
	paths, err = historyPaths(paths)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if !changed[path] {
			return errors.New("selected path is not a change in this commit/mainline")
		}
	}
	files := historyFiles(paths)
	if err := rejectHistoryIndexAliases(paths, index); err != nil {
		return err
	}
	if err := g.validateShelfPaths(ctx, repo, files, index); err != nil {
		return err
	}
	for _, tip := range []string{base, op.Source, repo.Head} {
		if err := g.validateShelfTreeModes(ctx, repo.Root, tip, files); err != nil {
			return err
		}
	}
	if err := g.protectSelectedPaths(ctx, repo.Root, paths); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "vela-history-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(temporary, "index")}
	run := func(input string, args ...string) (string, error) {
		out, err := g.runInputWithEnv(ctx, repo.Root, input, env, args...)
		return strings.TrimSpace(out.stdout), operationError(ctx, out, err)
	}
	if _, err := run("", "read-tree", base); err != nil {
		return err
	}
	var updates strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&updates, "0 %s\t%s\x00", strings.Repeat("0", len(op.Source)), path)
	}
	args := append([]string{"--literal-pathspecs", "ls-tree", "-r", "-z", op.Source, "--"}, paths...)
	target, err := g.read(ctx, repo.Root, args...)
	if err != nil {
		return err
	}
	for _, record := range nulFields(target) {
		header, path, _ := strings.Cut(record, "\t")
		parts := strings.Fields(header)
		if len(parts) != 3 || !slices.Contains(paths, path) {
			return errors.New("historical selection contains a directory")
		}
		fmt.Fprintf(&updates, "%s %s\t%s\x00", parts[0], parts[2], path)
	}
	if _, err := run(updates.String(), "update-index", "-z", "--index-info"); err != nil {
		return err
	}
	tree, err := run("", "write-tree")
	if err != nil {
		return err
	}
	info, err := g.read(ctx, repo.Root, "show", "-s", "--format=%an%x00%ae%x00%aI%x00%B", op.Source, "--")
	if err != nil {
		return err
	}
	metadata := strings.SplitN(info, "\x00", 4)
	if len(metadata) != 4 {
		return errors.New("invalid selected commit metadata")
	}
	originalMessage := strings.TrimSpace(metadata[3])
	if op.Kind == domain.RevertSelected {
		subject, body, _ := strings.Cut(originalMessage, "\n")
		originalMessage = subject + " (selected from " + op.Source + ")\n" + body
	}
	message := originalMessage + "\n\nSelected changes from " + op.Source + "\n"
	protected, err := domain.ProtectedBranch(op.Safety.ProtectedBranches, repo.CurrentBranch)
	if err != nil {
		return err
	}
	for _, path := range paths {
		message += strconv.Quote(path) + "\n"
	}
	if op.Kind == domain.CherryPickSelected && op.Safety.CherryPickSuffix && protected {
		message += "\n(cherry picked from commit " + op.Source + ")\n"
	}
	commitArgs := []string{"-c", "commit.gpgsign=false", "commit-tree", tree}
	if len(parents) > 1 {
		commitArgs = append(commitArgs, "-p", base)
	}
	author := []string{"GIT_AUTHOR_NAME=" + metadata[0], "GIT_AUTHOR_EMAIL=" + metadata[1], "GIT_AUTHOR_DATE=" + metadata[2]}
	out, err := g.runInputWithEnv(ctx, repo.Root, message, author, commitArgs...)
	if err != nil {
		return operationError(ctx, out, err)
	}
	synthetic := strings.TrimSpace(out.stdout)
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	repo.History = append(slices.Clone(repo.History), domain.Commit{Hash: synthetic})
	kind := domain.StartCherryPick
	if op.Kind == domain.RevertSelected {
		kind = domain.StartRevert
	}
	return g.sequence(ctx, repo, index, domain.RepositoryOperation{Kind: kind, Source: synthetic})
}
