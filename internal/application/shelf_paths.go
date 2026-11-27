package application

import (
	"slices"

	"vela/internal/domain"
)

const ShelfPathLimit = 100

// ShelfSavePaths includes both endpoints from both status panels. Git can
// report a rename in the Index and a separate Working modification at its target.
func ShelfSavePaths(repo domain.Repository, selected []string) []string {
	paths := slices.Clone(selected)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	links := make(map[string][]string)
	for _, files := range [][]domain.File{repo.Working, repo.Staged} {
		for _, f := range files {
			if f.OldPath != "" {
				links[f.Path] = append(links[f.Path], f.OldPath)
				links[f.OldPath] = append(links[f.OldPath], f.Path)
			}
		}
	}
	for i := 0; i < len(paths) && len(paths) <= ShelfPathLimit; i++ {
		for _, path := range links[paths[i]] {
			if !slices.Contains(paths, path) {
				paths = append(paths, path)
				if len(paths) > ShelfPathLimit {
					break
				}
			}
		}
	}
	slices.Sort(paths)
	return paths
}
