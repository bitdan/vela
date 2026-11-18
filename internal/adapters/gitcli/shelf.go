package gitcli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"vela/internal/domain"
)

func (g *Executor) shelfCommit(ctx context.Context, root, tree, message string, parents ...string) (string, error) {
	args := []string{"-c", "commit.gpgsign=false", "commit-tree", tree}
	for _, parent := range parents {
		args = append(args, "-p", parent)
	}
	out, err := g.runInputWithEnv(ctx, root, message+"\n", []string{"GIT_AUTHOR_NAME=Vela Shelf", "GIT_AUTHOR_EMAIL=shelf@vela.local", "GIT_COMMITTER_NAME=Vela Shelf", "GIT_COMMITTER_EMAIL=shelf@vela.local"}, args...)
	return strings.TrimSpace(out.stdout), err
}

func (g *Executor) shelfOperation(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	ctx = canonicalHistory(ctx)
	if err := g.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	shelves, err := g.readShelves(ctx, repo)
	if err != nil {
		return err
	}
	if op.Kind == domain.SaveShelf {
		return g.saveShelf(ctx, repo, index, op, shelves)
	}
	i := slices.IndexFunc(shelves, func(e domain.Shelf) bool { return e.ID == op.Target && e.OID == op.Source })
	if i < 0 {
		return errors.New("shelf changed; refresh before continuing")
	}
	entry := shelves[i]
	ref := shelfPrefix(repo.GitDir, entry.Deleted) + entry.ID
	switch op.Kind {
	case domain.RenameShelf:
		if entry.Deleted || !validShelfName(strings.TrimSpace(op.Message)) {
			return errors.New("use a single-line active shelf name up to 200 bytes")
		}
		tree, err := g.read(ctx, repo.Root, "rev-parse", entry.OID+"^{tree}")
		if err != nil {
			return err
		}
		next, err := g.shelfCommit(ctx, repo.Root, strings.TrimSpace(tree), shelfSubject+strings.TrimSpace(op.Message), entry.Base, entry.Index)
		if err != nil {
			return err
		}
		return g.runOperation(ctx, repo.Root, "update-ref", ref, next, entry.OID)
	case domain.DeleteShelf, domain.RestoreShelf:
		if entry.Deleted != (op.Kind == domain.RestoreShelf) {
			return errors.New("shelf deletion state changed")
		}
		return g.moveShelfRef(ctx, repo, entry, !entry.Deleted)
	case domain.PurgeShelf:
		if !entry.Deleted {
			return errors.New("move the shelf to Recently Deleted first")
		}
		return g.runOperation(ctx, repo.Root, "update-ref", "-d", ref, entry.OID)
	case domain.ApplyShelf:
		if entry.Deleted {
			return errors.New("restore the deleted shelf before applying it")
		}
		return g.applyShelf(ctx, repo, index, entry, op)
	}
	return errors.New("unknown shelf operation")
}

func (g *Executor) moveShelfRef(ctx context.Context, repo domain.Repository, entry domain.Shelf, deleted bool) error {
	source, target := shelfPrefix(repo.GitDir, entry.Deleted)+entry.ID, shelfPrefix(repo.GitDir, deleted)+entry.ID
	transaction := fmt.Sprintf("start\ncreate %s %s\ndelete %s %s\nprepare\ncommit\n", target, entry.OID, source, entry.OID)
	_, err := g.runInput(ctx, repo.Root, transaction, "update-ref", "--stdin")
	return err
}

func (g *Executor) saveShelf(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation, shelves []domain.Shelf) error {
	op.Message = strings.TrimSpace(op.Message)
	if !validShelfName(op.Message) || len(shelves) >= 200 || repo.Head == "" {
		return errors.New("shelving requires HEAD, a single-line name up to 200 bytes, and fewer than 200 stored entries")
	}
	paths := slices.Clone(op.Paths)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	if len(paths) == 0 || len(paths) > 100 {
		return errors.New("select 1 to 100 tracked changed paths")
	}
	changed := slices.Concat(repo.Working, repo.Staged)
	for i := 0; i < len(paths); i++ {
		found := false
		for _, f := range changed {
			if f.Path != paths[i] && f.OldPath != paths[i] {
				continue
			}
			if f.Status == "?" || f.Status == "U" {
				return errors.New("stage untracked files and resolve conflicts before shelving")
			}
			found = true
			if f.OldPath != "" && !slices.Contains(paths, f.OldPath) {
				paths = append(paths, f.OldPath)
			}
			if !slices.Contains(paths, f.Path) {
				paths = append(paths, f.Path)
			}
		}
		if !found {
			return ErrStaleIndex
		}
	}
	if len(paths) > 100 {
		return errors.New("rename-expanded shelf selection exceeds 100 paths")
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
	files := make([]domain.File, 0, len(paths))
	for _, path := range paths {
		files = append(files, domain.File{Path: path})
	}
	if err := g.validateShelfPaths(ctx, repo, files, index); err != nil {
		return err
	}
	if err := g.validateShelfTreeModes(ctx, repo.Root, repo.Head, files); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp("", "vela-shelf-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(temporary, "index")}
	run := func(input string, args ...string) (string, error) {
		out, err := g.runInputWithEnv(ctx, repo.Root, input, env, args...)
		return strings.TrimSpace(out.stdout), err
	}
	if _, err := run("", "read-tree", repo.Head); err != nil {
		return err
	}
	var updates strings.Builder
	for _, path := range paths {
		fmt.Fprintf(&updates, "0 %s\t%s\x00", strings.Repeat("0", len(repo.Head)), path)
	}
	for _, record := range nulFields(index) {
		_, path, _ := strings.Cut(record, "\t")
		if slices.Contains(paths, path) {
			updates.WriteString(record)
			updates.WriteByte(0)
		}
	}
	if _, err := run(updates.String(), "update-index", "-z", "--index-info"); err != nil {
		return err
	}
	indexTree, err := run("", "write-tree")
	if err != nil {
		return err
	}
	// Preserve staged modes and force-added ignored paths when taking the
	// working snapshot; add only updates already selected Index membership.
	if _, err := run("", "read-tree", indexTree); err != nil {
		return err
	}
	var workPaths []string
	indexed := map[string]bool{}
	for _, record := range nulFields(index) {
		_, path, _ := strings.Cut(record, "\t")
		indexed[path] = true
	}
	for _, path := range paths {
		_, statErr := os.Lstat(filepath.Join(repo.Root, filepath.FromSlash(path)))
		if statErr == nil || indexed[path] {
			workPaths = append(workPaths, path)
		}
	}
	if len(workPaths) > 0 {
		if _, err := run(strings.Join(workPaths, "\x00")+"\x00", "--literal-pathspecs", "-c", "submodule.recurse=false", "add", "-A", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
			return err
		}
	}
	workTree, err := run("", "write-tree")
	if err != nil {
		return err
	}
	baseTree, err := g.read(ctx, repo.Root, "rev-parse", repo.Head+"^{tree}")
	if err != nil {
		return err
	}
	if workTree == strings.TrimSpace(baseTree) && indexTree == strings.TrimSpace(baseTree) {
		return errors.New("selected paths have no shelvable changes")
	}
	indexCommit, err := g.shelfCommit(ctx, repo.Root, indexTree, "vela-shelf-index-v1", repo.Head)
	if err != nil {
		return err
	}
	commit, err := g.shelfCommit(ctx, repo.Root, workTree, shelfSubject+op.Message, repo.Head, indexCommit)
	if err != nil {
		return err
	}
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return err
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	ref := shelfPrefix(repo.GitDir, false) + hex.EncodeToString(random[:])
	if err := g.runOperation(ctx, repo.Root, "update-ref", ref, commit, strings.Repeat("0", len(commit))); err != nil {
		return err
	}
	// From this point every selected version is durable. Revalidate once more;
	// external edits or cancellation leave the shelf copy and skip cleanup.
	if _, _, err := g.verifyCheckout(ctx, repo); err != nil {
		return fmt.Errorf("shelf saved; cleanup skipped: %w", err)
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return fmt.Errorf("shelf saved; cleanup skipped: %w", err)
	}
	cleanup := strings.Join(paths, "\x00") + "\x00"
	if _, err := g.runInput(ctx, repo.Root, cleanup, "--literal-pathspecs", "-c", "submodule.recurse=false", "restore", "--source="+repo.Head, "--staged", "--worktree", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		return fmt.Errorf("shelf saved; cleanup may be incomplete: %w", err)
	}
	return nil
}

func (g *Executor) validateShelfTreeModes(ctx context.Context, root, tip string, files []domain.File) error {
	args := []string{"--literal-pathspecs", "ls-tree", "-r", "-z", tip, "--"}
	pathBytes := 0
	for _, file := range files {
		pathBytes += len(file.Path)
		args = append(args, file.Path)
	}
	if pathBytes > 16384 {
		return errors.New("shelf paths exceed 16 KiB; select fewer files")
	}
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 2<<20, args...)
	if err != nil {
		return err
	}
	for _, record := range nulFields(out.stdout) {
		if !strings.HasPrefix(record, "100644 blob ") && !strings.HasPrefix(record, "100755 blob ") {
			return errors.New("shelf does not support linked files or submodules in saved/base trees")
		}
	}
	return nil
}

func (g *Executor) validateShelfPaths(ctx context.Context, repo domain.Repository, files []domain.File, index string) error {
	root, err := os.OpenRoot(repo.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	total := int64(0)
	for _, file := range files {
		if !validGitPath(file.Path) {
			return errors.New("invalid shelf path")
		}
		for _, part := range strings.Split(file.Path, "/") {
			if strings.EqualFold(strings.TrimRight(part, " ."), ".git") {
				return errors.New("shelf cannot access Git metadata")
			}
		}
		for parent := filepath.Dir(file.Path); parent != "."; parent = filepath.Dir(parent) {
			info, err := root.Lstat(parent)
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if err != nil {
				return err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return errors.New("shelf paths cannot traverse links")
			}
		}
		info, err := root.Lstat(file.Path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if info != nil {
			if !info.Mode().IsRegular() {
				return errors.New("shelf supports regular files only")
			}
			total += info.Size()
			if info.Size() > 16<<20 || total > 64<<20 {
				return errors.New("shelf selection exceeds 16 MiB/file or 64 MiB total")
			}
		}
		for _, record := range nulFields(index) {
			fields, path, _ := strings.Cut(record, "\t")
			if path == file.Path && !strings.HasPrefix(fields, "100644 ") && !strings.HasPrefix(fields, "100755 ") {
				return errors.New("shelf does not support linked files or submodules")
			}
		}
		attrs, err := g.read(ctx, repo.Root, "--literal-pathspecs", "check-attr", "-z", "filter", "working-tree-encoding", "--", file.Path)
		if err != nil {
			return err
		}
		values := nulFields(attrs)
		if len(values)%3 != 0 {
			return errors.New("invalid shelf attributes")
		}
		for i := 2; i < len(values); i += 3 {
			if values[i] != "unspecified" && values[i] != "unset" {
				return errors.New("shelf does not support custom clean filters or working-tree encoding")
			}
		}
	}
	return nil
}
