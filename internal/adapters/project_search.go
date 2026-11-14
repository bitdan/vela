package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"vela/internal/application"
)

type ProjectFiles struct{}

// SearchProject reads only supplied Project paths, without walking the tree or
// invoking Git. os.Root prevents a linked path from escaping the selected root.
func (ProjectFiles) SearchProject(ctx context.Context, input application.ProjectSearchInput) (application.ProjectSearchResult, error) {
	if err := ctx.Err(); err != nil {
		return application.ProjectSearchResult{}, err
	}
	if !filepath.IsAbs(input.Root) {
		return application.ProjectSearchResult{}, errors.New("project search requires an absolute root")
	}
	root, err := os.OpenRoot(input.Root)
	if err != nil {
		return application.ProjectSearchResult{}, fmt.Errorf("open project search root: %w", err)
	}
	defer root.Close()
	return application.SearchProject(ctx, input, func(ctx context.Context, name string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			info, err := root.Lstat(parent)
			if err != nil {
				return "", err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return "", application.ErrProjectSearchSkipped
			}
		}
		info, err := root.Lstat(name)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() || info.Size() > application.MaxProjectSearchFileBytes {
			return "", application.ErrProjectSearchSkipped
		}
		file, err := root.Open(name)
		if err != nil {
			return "", err
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil {
			return "", err
		}
		if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
			return "", application.ErrProjectSearchSkipped
		}
		data, err := io.ReadAll(io.LimitReader(file, application.MaxProjectSearchFileBytes+1))
		if err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return string(data), nil
	})
}
