package adapters

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"

	"vela/internal/application"
)

// Discovery is explicit and bounded. It never follows links or descends into
// Git metadata/nested repositories, even when ignored files are requested.
func discoverProjectSearchPaths(ctx context.Context, root *os.Root, supplied []string, filterText string) ([]string, bool, error) {
	filter, err := application.CompileProjectPathFilter(filterText)
	if err != nil {
		return nil, false, err
	}
	paths := make([]string, 0, min(len(supplied), application.MaxProjectSearchFiles))
	seen := map[string]bool{}
	for _, name := range supplied {
		if !application.ValidProjectSearchPath(name) {
			return nil, false, fmt.Errorf("invalid project search path %q", name)
		}
		if !seen[name] && filter(name) {
			paths = append(paths, name)
			seen[name] = true
		}
	}
	visited := 0
	nested := map[string]bool{}
	limit := errors.New("discovery limit")
	err = fs.WalkDir(root.FS(), ".", func(name string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return fmt.Errorf("discover %s: %w", name, walkErr)
		}
		visited++
		if visited > application.MaxProjectSearchFiles*2 {
			return limit
		}
		if name == "." {
			return nil
		}
		if !application.ValidProjectSearchPath(name) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			isRepository, err := nestedProjectRepository(root, name)
			if err != nil {
				return err
			}
			if isRepository {
				nested[name] = true
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || seen[name] || !filter(name) {
			return nil
		}
		if len(paths) >= application.MaxProjectSearchFiles {
			return limit
		}
		paths = append(paths, name)
		seen[name] = true
		return nil
	})
	kept := paths[:0]
	for _, name := range paths {
		excluded := false
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if nested[parent] {
				excluded = true
				break
			}
		}
		if !excluded {
			kept = append(kept, name)
		}
	}
	paths = kept
	if errors.Is(err, limit) {
		return paths, true, nil
	}
	return paths, false, err
}

func nestedProjectRepository(root *os.Root, name string) (bool, error) {
	if _, err := root.Lstat(path.Join(name, ".git")); err == nil {
		return true, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("inspect repository %s: %w", name, err)
	}
	for _, entry := range []struct {
		name      string
		directory bool
	}{{"HEAD", false}, {"objects", true}, {"refs", true}} {
		info, err := root.Lstat(path.Join(name, entry.name))
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("inspect bare repository %s: %w", name, err)
		}
		if info.IsDir() != entry.directory || info.Mode()&os.ModeSymlink != 0 {
			return false, nil
		}
	}
	return true, nil
}
