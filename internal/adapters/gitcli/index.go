package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"vela/internal/domain"
)

var ErrStaleIndex = errors.New("repository changed since the last refresh")

func (g *Executor) readCheckout(ctx context.Context, repo *domain.Repository) (string, error) {
	branch, err := g.run(ctx, repo.Root, "symbolic-ref", "--quiet", "HEAD")
	if err != nil && branch.exitCode != 1 {
		return "", err
	}
	repo.CurrentBranch = strings.TrimPrefix(strings.TrimSuffix(branch.stdout, "\n"), "refs/heads/")
	if repo.CurrentBranch == "" {
		repo.CurrentBranch = "Detached HEAD"
	}
	head, err := g.run(ctx, repo.Root, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil && head.exitCode != 1 {
		return "", err
	}
	repo.Head = strings.TrimSpace(head.stdout)
	index, err := g.read(ctx, repo.Root, "ls-files", "--stage", "-z")
	if err != nil {
		return "", err
	}
	repo.IndexVersion = fmt.Sprintf("%x", sha256.Sum256([]byte(index)))
	return index, nil
}

// UpdateIndex applies an explicit set of complete files and leaves HEAD and the worktree alone.
func (g *Executor) UpdateIndex(ctx context.Context, snapshot domain.Repository, stage bool, selected []domain.File) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	g.write.Lock()
	defer g.write.Unlock()
	if snapshot.Root == "" || snapshot.IndexVersion == "" || len(selected) == 0 {
		return errors.New("select changes in a local repository")
	}
	current, index, err := g.verifyCheckout(ctx, snapshot)
	if err != nil {
		return err
	}
	current.Staged, current.Working, err = g.readStatus(ctx, snapshot.Root)
	if err != nil {
		return err
	}
	from := current.Staged
	if stage {
		from = current.Working
	}
	actual := filesByPath(from)
	beforeWorking, afterWorking := filesByPath(snapshot.Working), filesByPath(current.Working)
	paths := make(map[string]bool)
	for _, file := range selected {
		if change, ok := actual[file.Path]; !ok || change != file || file.Status == "U" {
			return ErrStaleIndex
		}
		oldPath := ""
		if file.Status == "R" {
			oldPath = file.OldPath
		}
		for _, name := range []string{file.Path, oldPath} {
			if name == "" {
				continue
			}
			if !validGitPath(name) || strings.ContainsRune(name, 0) {
				return fmt.Errorf("invalid repository path %q", name)
			}
			if beforeWorking[name] != afterWorking[name] || afterWorking[name].Status == "U" {
				return ErrStaleIndex
			}
			paths[name] = true
		}
	}
	if len(paths) == 0 {
		return errors.New("select at least one file")
	}
	if err := validateIndexPaths(snapshot.Root, paths, index, current.Staged); err != nil {
		return err
	}
	var input strings.Builder
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		input.WriteString(path)
		input.WriteByte(0)
	}
	args := []string{"-c", "core.fsmonitor=false", "--literal-pathspecs"}
	if stage {
		args = append(args, "add", "--all")
	} else {
		source := current.Head
		if source == "" {
			empty, err := g.read(ctx, snapshot.Root, "hash-object", "-t", "tree", "--stdin")
			if err != nil {
				return err
			}
			source = strings.TrimSpace(empty)
		}
		args = append(args, "reset", "--quiet", "--no-refresh", source)
	}
	out, err := g.runInput(ctx, snapshot.Root, input.String(), append(args, "--pathspec-from-file=-", "--pathspec-file-nul")...)
	if err != nil && ctx.Err() == nil && out.stderr != "" {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out.stderr))
	}
	return err
}

func (g *Executor) verifyCheckout(ctx context.Context, snapshot domain.Repository) (domain.Repository, string, error) {
	current := domain.Repository{Root: snapshot.Root}
	root, err := g.read(ctx, snapshot.Root, "rev-parse", "--show-toplevel")
	if err != nil {
		return current, "", err
	}
	if filepath.Clean(strings.TrimSuffix(root, "\n")) != snapshot.Root {
		return current, "", ErrStaleIndex
	}
	index, err := g.readCheckout(ctx, &current)
	if err != nil {
		return current, "", err
	}
	if current.Head != snapshot.Head || current.CurrentBranch != snapshot.CurrentBranch || current.IndexVersion != snapshot.IndexVersion {
		return current, "", ErrStaleIndex
	}
	return current, index, nil
}

func filesByPath(files []domain.File) map[string]domain.File {
	byPath := make(map[string]domain.File, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	return byPath
}

func validateIndexPaths(root string, paths map[string]bool, index string, staged []domain.File) error {
	for path := range paths {
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if info != nil && info.IsDir() {
			return fmt.Errorf("%q is now a directory", path)
		}
	}
	for _, entry := range nulFields(index) {
		fields, path, _ := strings.Cut(entry, "\t")
		if paths[path] && strings.HasPrefix(fields, "160000 ") {
			return fmt.Errorf("%q is a submodule", path)
		}
		if err := rejectDirectoryPath(paths, path); err != nil {
			return err
		}
	}
	for _, file := range staged {
		for _, path := range []string{file.Path, file.OldPath} {
			if err := rejectDirectoryPath(paths, path); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectDirectoryPath(paths map[string]bool, path string) error {
	for slash := strings.LastIndexByte(path, '/'); slash >= 0; slash = strings.LastIndexByte(path, '/') {
		path = path[:slash]
		if paths[path] {
			return fmt.Errorf("%q also contains Git paths; refresh and resolve the file/directory change in Git", path)
		}
	}
	return nil
}
