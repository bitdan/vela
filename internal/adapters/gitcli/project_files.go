package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

// ProjectFiles shares the Git writer lock. Trash is supplied by Bootstrap and
// must move to the OS recycle bin without falling back to permanent deletion.
type ProjectFiles struct {
	Git   *Executor
	Trash func(string) error
}

const projectTreeBytes = 256 << 20
const projectTreeEntries = 10000

func projectPath(name string, rootAllowed bool) error {
	if rootAllowed && name == "." {
		return nil
	}
	if name == "" || name == "." || !fs.ValidPath(name) || strings.ContainsAny(name, "\\:\x00\r\n") {
		return errors.New("use a relative project path without parent traversal")
	}
	for _, part := range strings.Split(name, "/") {
		if strings.EqualFold(part, ".git") || strings.HasPrefix(strings.ToLower(part), "git~") || strings.HasPrefix(strings.ToLower(part), ".git~") || strings.ContainsAny(part, "<>\"|?*") || strings.TrimRight(part, " .") != part {
			return errors.New("Git metadata and platform-incompatible names are not allowed")
		}
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
			return errors.New("reserved device names are not allowed")
		}
	}
	return nil
}

// Check each component as well as using os.Root: links inside the repository
// must not redirect a file action to a different user path.
func ordinaryProjectPath(root *os.Root, name string, missingLeaf bool) error {
	parts := strings.Split(name, "/")
	for i := range parts {
		info, err := root.Lstat(filepath.FromSlash(strings.Join(parts[:i+1], "/")))
		if errors.Is(err, fs.ErrNotExist) && missingLeaf && i == len(parts)-1 {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("unsupported linked or special path: %s", strings.Join(parts[:i+1], "/"))
		}
		if i < len(parts)-1 && !info.IsDir() {
			return errors.New("parent is not a directory")
		}
	}
	return nil
}

func inspectProjectPath(ctx context.Context, root *os.Root, name string) (domain.ProjectPathSnapshot, error) {
	snapshot := domain.ProjectPathSnapshot{Path: name}
	if err := projectPath(name, true); err != nil {
		return snapshot, err
	}
	if err := ordinaryProjectPath(root, name, false); err != nil {
		return snapshot, err
	}
	info, err := root.Lstat(filepath.FromSlash(name))
	if err != nil {
		return snapshot, err
	}
	snapshot.Directory = info.IsDir()
	h := sha256.New()
	var total int64
	entries := 0
	err = fs.WalkDir(root.FS(), name, func(item string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := projectPath(item, true); err != nil {
			return err
		}
		entries++
		if entries > projectTreeEntries {
			return errors.New("file action exceeds 10,000 entries")
		}
		stat, err := d.Info()
		if err != nil {
			return err
		}
		if stat.Mode()&os.ModeSymlink != 0 || (!stat.IsDir() && !stat.Mode().IsRegular()) {
			return fmt.Errorf("unsupported linked or special path: %s", item)
		}
		fmt.Fprintf(h, "%s\x00%s\x00%d\x00%d\x00", item, stat.Mode(), stat.Size(), stat.ModTime().UnixNano())
		if stat.IsDir() {
			return nil
		}
		if stat.Size() > projectTreeBytes-total {
			return errors.New("file action exceeds 256 MiB")
		}
		file, err := root.Open(filepath.FromSlash(item))
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(h, io.LimitReader(&projectContextReader{ctx: ctx, reader: file}, projectTreeBytes-total+1))
		total += n
		after, statErr := file.Stat()
		if statErr == nil && (after.Size() != stat.Size() || !after.ModTime().Equal(stat.ModTime()) || n != stat.Size() || total > projectTreeBytes) {
			statErr = errors.New("source changed while being inspected")
		}
		return errors.Join(copyErr, statErr, file.Close())
	})
	if err == nil {
		snapshot.Version = fmt.Sprintf("%x", h.Sum(nil))
	}
	return snapshot, err
}

type projectContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *projectContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (m ProjectFiles) InspectProjectPath(ctx context.Context, repo domain.Repository, name string) (domain.ProjectPathSnapshot, error) {
	root, err := os.OpenRoot(repo.Root)
	if err != nil {
		return domain.ProjectPathSnapshot{}, err
	}
	defer root.Close()
	return inspectProjectPath(ctx, root, name)
}

func (m ProjectFiles) ExecuteProjectFileOperation(ctx context.Context, repo domain.Repository, op domain.ProjectFileOperation) error {
	m.Git.write.Lock()
	defer m.Git.write.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, index, err := m.Git.verifyCheckout(ctx, repo); err != nil {
		return err
	} else if err := m.Git.checkCommitState(ctx, repo.Root, index); err != nil {
		return err
	}
	if repo.ResolvingConflicts() || repo.Bisect.Active() {
		return errors.New("finish the current Git operation first")
	}
	root, err := os.OpenRoot(repo.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	create := op.Action == domain.CreateProjectFile || op.Action == domain.CreateProjectDirectory
	if !create {
		if op.Source.Path == "." || op.Source.Version == "" {
			return errors.New("select and inspect a project item first")
		}
		current, err := inspectProjectPath(ctx, root, op.Source.Path)
		if err != nil {
			return err
		}
		if current != op.Source {
			return errors.New("source changed on disk; reopen the file action")
		}
	}
	if op.Action == domain.TrashProjectPath {
		if m.Trash == nil {
			return errors.New("recycle bin is unavailable")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return m.Trash(filepath.Join(repo.Root, filepath.FromSlash(op.Source.Path)))
	}
	if op.Action == domain.IgnoreProjectPath {
		return appendProjectIgnore(ctx, root, op.Source)
	}
	if err := projectPath(op.Destination, false); err != nil {
		return err
	}
	if !create && strings.EqualFold(op.Source.Path, op.Destination) {
		return errors.New("choose a different destination")
	}
	if op.Source.Directory && strings.HasPrefix(strings.ToLower(op.Destination), strings.ToLower(op.Source.Path)+"/") {
		return errors.New("a directory cannot be moved or copied into itself")
	}
	if err := ordinaryProjectPath(root, op.Destination, true); err != nil {
		return err
	}
	if _, err := root.Lstat(filepath.FromSlash(op.Destination)); !errors.Is(err, fs.ErrNotExist) {
		if err != nil {
			return err
		}
		return errors.New("destination already exists; files will not be overwritten")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	switch op.Action {
	case domain.CreateProjectFile:
		file, err := root.OpenFile(filepath.FromSlash(op.Destination), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		return file.Close()
	case domain.CreateProjectDirectory:
		return root.Mkdir(filepath.FromSlash(op.Destination), 0755)
	case domain.MoveProjectPath:
		// Copy exclusively first so even a concurrently created destination can
		// never be overwritten. Recycle the original only after a second check.
		if m.Trash == nil {
			return errors.New("recycle bin is unavailable for moving the source")
		}
		if err := copyProjectPath(ctx, root, op.Source, op.Destination); err != nil {
			return err
		}
		current, err := inspectProjectPath(ctx, root, op.Source.Path)
		if err != nil || current != op.Source {
			return fmt.Errorf("destination copied to %s but source changed; source kept: %w", op.Destination, errors.Join(err, errors.New("source identity changed")))
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("destination copied to %s; source kept: %w", op.Destination, err)
		}
		if err := m.Trash(filepath.Join(repo.Root, filepath.FromSlash(op.Source.Path))); err != nil {
			return fmt.Errorf("destination copied to %s but source could not be recycled; inspect both paths: %w", op.Destination, err)
		}
		return nil
	case domain.CopyProjectPath:
		return copyProjectPath(ctx, root, op.Source, op.Destination)
	default:
		return errors.New("unknown project file operation")
	}
}

func copyProjectPath(ctx context.Context, root *os.Root, source domain.ProjectPathSnapshot, destination string) error {
	// Each target is exclusively created. Failures keep partial output for
	// inspection; never recursively remove a tree that an external app may edit.
	var total int64
	entries := 0
	err := fs.WalkDir(root.FS(), source.Path, func(item string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := projectPath(item, false); err != nil {
			return err
		}
		if err := ordinaryProjectPath(root, item, false); err != nil {
			return err
		}
		entries++
		if entries > projectTreeEntries {
			return errors.New("copy exceeds 10,000 entries")
		}
		target := destination + strings.TrimPrefix(item, source.Path)
		stat, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := ordinaryProjectPath(root, target, true); err != nil {
				return err
			}
			return root.Mkdir(filepath.FromSlash(target), stat.Mode().Perm()|0700)
		}
		if err := ordinaryProjectPath(root, target, true); err != nil {
			return err
		}
		in, err := root.Open(filepath.FromSlash(item))
		if err != nil {
			return err
		}
		out, err := root.OpenFile(filepath.FromSlash(target), os.O_WRONLY|os.O_CREATE|os.O_EXCL, stat.Mode().Perm()|0600)
		if err != nil {
			return errors.Join(err, in.Close())
		}
		n, copyErr := io.Copy(out, io.LimitReader(&projectContextReader{ctx: ctx, reader: in}, projectTreeBytes-total+1))
		total += n
		if total > projectTreeBytes {
			copyErr = errors.Join(copyErr, errors.New("copy exceeds 256 MiB"))
		}
		if copyErr == nil {
			copyErr = out.Chmod(stat.Mode().Perm())
		}
		return errors.Join(copyErr, out.Close(), in.Close())
	})
	if err != nil {
		return fmt.Errorf("copy may be incomplete at %s; inspect the destination: %w", destination, err)
	}
	current, err := inspectProjectPath(ctx, root, source.Path)
	if err != nil || current != source {
		return fmt.Errorf("source changed while copying to %s; inspect the destination: %w", destination, errors.Join(err, errors.New("source identity changed")))
	}
	return nil
}

func appendProjectIgnore(ctx context.Context, root *os.Root, source domain.ProjectPathSnapshot) error {
	if source.Path == ".gitignore" {
		return errors.New("cannot add .gitignore to itself")
	}
	if err := ordinaryProjectPath(root, ".gitignore", true); err != nil {
		return err
	}
	var data []byte
	file, err := root.Open(".gitignore")
	if err == nil {
		data, err = io.ReadAll(io.LimitReader(file, 2<<20+1))
		err = errors.Join(err, file.Close())
	} else if errors.Is(err, fs.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return err
	}
	if len(data) > 2<<20 || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return errors.New(".gitignore must be UTF-8 text of at most 2 MiB")
	}
	pattern := "/" + strings.NewReplacer("\\", "\\\\", "*", "\\*", "?", "\\?", "[", "\\[", "]", "\\]", " ", "\\ ").Replace(source.Path)
	if source.Directory {
		pattern += "/"
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if line == pattern {
			return nil
		}
	}
	newline := "\n"
	if strings.Contains(string(data), "\r\n") {
		newline = "\r\n"
	}
	addition := pattern + newline
	if len(data) > 0 && data[len(data)-1] != '\n' {
		addition = newline + addition
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	out, err := root.OpenFile(".gitignore", os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	_, err = out.WriteString(addition)
	return errors.Join(err, out.Close())
}

// ResolveProjectFolder validates paths before passing them to the OS shell.
func ResolveProjectFolder(rootPath, name string) (string, error) {
	if err := projectPath(name, true); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := ordinaryProjectPath(root, name, false); err != nil {
		return "", err
	}
	info, err := root.Lstat(filepath.FromSlash(name))
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		name = path.Dir(name)
	}
	return filepath.Join(rootPath, filepath.FromSlash(name)), nil
}
