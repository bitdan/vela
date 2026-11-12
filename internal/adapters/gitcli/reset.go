package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"vela/internal/domain"
)

// worktreeVersion detects content edits even when status letters do not change.
// Hash raw tracked content as well: Git diff can hide assume-unchanged files
// and clean filters can hide raw edits. Include untracked files for Stash.
func (g *Executor) worktreeVersion(ctx context.Context, root string) (string, error) {
	diff, err := g.read(ctx, root, "-c", "core.fsmonitor=false", "diff", "--no-ext-diff", "--no-textconv", "--binary", "--ignore-submodules=none", "--")
	if err != nil {
		return "", err
	}
	paths, err := g.read(ctx, root, "ls-files", "--cached", "--others", "--exclude-standard", "--deduplicate", "-z")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	dir, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	io.WriteString(h, diff)
	io.WriteString(h, "\x00"+paths)
	for _, path := range nulFields(paths) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		name := filepath.FromSlash(path)
		info, err := dir.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			io.WriteString(h, "\x00missing\x00"+path)
			continue
		}
		if err != nil {
			return "", err
		}
		io.WriteString(h, fmt.Sprintf("\x00%d:%s\x00%s\x00%d\x00", len(path), path, info.Mode(), info.Size()))
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filepath.Join(root, name))
			if err != nil {
				return "", err
			}
			io.WriteString(h, fmt.Sprintf("%d:%s", len(target), target))
		} else if info.Mode().IsRegular() {
			f, err := dir.Open(name)
			if err != nil {
				return "", err
			}
			_, copyErr := io.Copy(h, contextFileReader{ctx, f})
			closeErr := f.Close()
			if err := errors.Join(copyErr, closeErr); err != nil {
				return "", err
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

type contextFileReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextFileReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (g *Executor) verifyWorktree(ctx context.Context, repo domain.Repository) error {
	if repo.WorktreeVersion == "" {
		return errors.New("refresh the repository before changing working files")
	}
	version, err := g.worktreeVersion(ctx, repo.Root)
	if err != nil {
		return err
	}
	if version != repo.WorktreeVersion {
		return ErrStaleIndex
	}
	return nil
}

func (g *Executor) reset(ctx context.Context, repo domain.Repository, index string, op domain.RepositoryOperation) error {
	if repo.Head == "" || repo.CurrentBranch == "Detached HEAD" {
		return errors.New("check out a local branch with a commit before resetting")
	}
	if err := g.checkOperationState(ctx, repo.Root, "", false); err != nil {
		return err
	}
	if op.Mode == "Soft" && len(conflictsFromIndex(index)) > 0 {
		return errors.New("resolve index conflicts before Soft reset; Mixed or Hard can clear unmerged entries")
	}
	if err := g.verifyWorktree(ctx, repo); err != nil {
		return err
	}
	switch op.Mode {
	case "Soft", "Mixed", "Hard":
	default:
		return errors.New("choose Soft, Mixed or Hard reset")
	}
	target, err := g.resolveSource(ctx, repo, op.Source)
	if err != nil {
		return err
	}
	if op.Mode == "Hard" {
		if err := g.protectWorktreePaths(ctx, repo.Root, repo.Head, target); err != nil {
			return err
		}
		if err := rejectSubmoduleIndex(index); err != nil {
			return err
		}
	}
	return g.runOperation(ctx, repo.Root, "-c", "submodule.recurse=false", "reset", "--"+strings.ToLower(op.Mode), target, "--")
}

func rejectSubmoduleIndex(index string) error {
	for _, entry := range nulFields(index) {
		if strings.HasPrefix(entry, "160000 ") {
			return errors.New("use Git to perform this operation in a repository containing submodules")
		}
	}
	return nil
}

func (g *Executor) rejectHiddenIndexFlags(ctx context.Context, root string) error {
	flags, err := g.read(ctx, root, "ls-files", "-v", "-z")
	if err != nil {
		return err
	}
	for _, row := range nulFields(flags) {
		if len(row) > 0 && (row[0] == 'S' || row[0] >= 'a' && row[0] <= 'z') {
			return errors.New("restore normal index flags before this operation; hidden tracked edits may exist")
		}
	}
	return nil
}
