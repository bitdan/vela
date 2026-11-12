package gitcli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vela/internal/domain"
)

func (g *Executor) ReadMergeDocument(ctx context.Context, repo domain.Repository, path string) (domain.MergeDocument, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var doc domain.MergeDocument
	if repo.Root == "" || !validGitPath(path) || strings.ContainsRune(path, 0) {
		return doc, errors.New("select a valid conflicted file")
	}
	_, index, err := g.verifyCheckout(ctx, repo)
	if err != nil {
		return doc, err
	}
	if err := g.verifyConflictOperation(ctx, repo); err != nil {
		return doc, err
	}
	for _, conflict := range conflictsFromIndex(index) {
		if conflict.Path == path {
			doc.Conflict = conflict
			break
		}
	}
	if doc.Conflict.Path == "" || doc.Conflict.Submodule {
		return doc, errors.New("select an unresolved text file outside submodules")
	}
	dir, err := os.OpenRoot(repo.Root)
	if err != nil {
		return doc, err
	}
	defer dir.Close()
	doc.Result, _, err = mergeWorkingText(dir, path)
	if err != nil {
		return doc, err
	}
	crlf := strings.Count(doc.Result, "\r\n")
	if crlf > 0 && crlf != strings.Count(doc.Result, "\n") {
		return doc, errors.New("this file has mixed line endings. Resolve it in an external editor")
	}
	if doc.Conflict.Current != "" {
		if doc.Current, err = g.readBlob(ctx, repo.Root, doc.Conflict.Current); err != nil {
			return doc, err
		}
	}
	if doc.Conflict.Incoming != "" {
		if doc.Incoming, err = g.readBlob(ctx, repo.Root, doc.Conflict.Incoming); err != nil {
			return doc, err
		}
	}
	doc.MarkerSize, err = g.conflictMarkerSize(ctx, repo.Root, path)
	doc.WorktreeVersion = mergeTextVersion(doc.Result)
	doc.Head, doc.MergeHeads, doc.Rebase, doc.Sequencer = repo.Head, repo.MergeHeads, repo.Rebase, repo.Sequencer
	return doc, err
}

func (g *Executor) conflictMarkerSize(ctx context.Context, root, path string) (int, error) {
	value, err := g.read(ctx, root, "check-attr", "-z", "conflict-marker-size", "--", path)
	if err != nil {
		return 0, err
	}
	fields := nulFields(value)
	if len(fields) == 3 {
		if n, err := strconv.Atoi(fields[2]); err == nil && n > 0 {
			if n > 1024 {
				return 0, errors.New("the conflict marker size exceeds the merge editor limit")
			}
			return n, nil
		}
	}
	return 7, nil
}

func mergeTextVersion(text string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
}

func mergeWorkingText(dir *os.Root, path string) (string, os.FileMode, error) {
	for parent := filepath.Dir(path); parent != "."; parent = filepath.Dir(parent) {
		info, err := dir.Lstat(parent)
		if err != nil {
			return "", 0, err
		}
		if !info.IsDir() {
			return "", 0, errors.New("the conflicted file is under a symbolic link or non-directory")
		}
	}
	info, err := dir.Lstat(path)
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, errors.New("only regular working files can be edited here")
	}
	file, err := dir.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !os.SameFile(info, opened) {
		return "", 0, errors.New("the working file was replaced")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPreviewBytes+1))
	if err != nil {
		return "", 0, err
	}
	text, err := previewText(string(data))
	return text, info.Mode().Perm(), err
}

func (g *Executor) saveMergeResult(ctx context.Context, root string, op domain.RepositoryOperation) error {
	if _, err := previewText(op.Result); err != nil {
		return err
	}
	size, err := g.conflictMarkerSize(ctx, root, op.Source)
	if err != nil {
		return err
	}
	if blocks, incomplete := domain.ConflictBlocks(op.Result, size); len(blocks) != 0 || incomplete {
		return errors.New("resolve the remaining conflict markers before staging")
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	original, mode, err := mergeWorkingText(dir, op.Source)
	if err != nil {
		return err
	}
	if op.WorktreeVersion != mergeTextVersion(original) {
		return errors.New("the working file changed on disk. Preserve your draft and reopen the file before applying")
	}
	if op.Result != original {
		if err := replaceMergeText(ctx, dir, op, mode); err != nil {
			return err
		}
	}
	if err := g.runOperation(ctx, root, "-c", "core.fsmonitor=false", "--literal-pathspecs", "add", "--", op.Source); err != nil {
		return fmt.Errorf("The result is on disk but could not be staged: %w", err)
	}
	return nil
}

func replaceMergeText(ctx context.Context, dir *os.Root, op domain.RepositoryOperation, mode os.FileMode) error {
	temp := filepath.Join(filepath.Dir(op.Source), ".vela-merge-"+rand.Text())
	file, err := dir.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer dir.Remove(temp)
	_, writeErr := io.WriteString(file, op.Result)
	if err := errors.Join(writeErr, file.Chmod(mode), file.Close()); err != nil {
		return err
	}
	current, currentMode, err := mergeWorkingText(dir, op.Source)
	if err != nil {
		return err
	}
	if mergeTextVersion(current) != op.WorktreeVersion || currentMode != mode {
		return errors.New("the working file changed while applying the result")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return dir.Rename(temp, op.Source)
}
