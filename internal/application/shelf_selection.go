package application

import (
	"errors"
	"fmt"

	"vela/internal/domain"
)

// SelectShelfFiles resolves a captured path selection against the current shelf.
// Explicit empty selection is never interpreted as applying the entire shelf.
// Older callers may omit paths to request the full entry.
func SelectShelfFiles(entry domain.Shelf, paths []string, explicit bool) ([]domain.File, bool, error) {
	if len(entry.Files) == 0 || len(entry.Files) > 100 {
		return nil, false, errors.New("select a shelf with 1 to 100 saved paths")
	}
	known := make(map[string]bool, len(entry.Files))
	for _, file := range entry.Files {
		if known[file.Path] {
			return nil, false, errors.New("shelf contains duplicate paths")
		}
		known[file.Path] = true
	}
	if !explicit && len(paths) == 0 {
		paths = make([]string, 0, len(entry.Files))
		for _, file := range entry.Files {
			paths = append(paths, file.Path)
		}
	}
	if len(paths) == 0 || len(paths) > 100 {
		return nil, false, errors.New("select 1 to 100 shelved file paths")
	}
	chosen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if !known[path] || chosen[path] {
			return nil, false, fmt.Errorf("invalid or duplicate shelved path %q; reopen the shelf selection", path)
		}
		chosen[path] = true
	}
	partial := len(chosen) < len(entry.Files)
	if partial && !explicit {
		return nil, false, errors.New("partial unshelve requires an explicit file selection")
	}
	files := make([]domain.File, 0, len(chosen))
	for _, file := range entry.Files {
		if chosen[file.Path] {
			files = append(files, file)
		}
	}
	return files, partial, nil
}
