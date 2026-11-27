package application

import (
	"errors"
	"vela/internal/domain"
)

func ValidateShelfHunks(paths []string, selections []domain.ShelfHunkSelection) error {
	known := map[string]bool{}
	for _, p := range paths {
		known[p] = true
	}
	seen := map[string]bool{}
	total := 0
	for _, s := range selections {
		if !known[s.Path] || seen[s.Path] || s.Version == "" || len(s.Hunks) == 0 {
			return errors.New("select nonempty, unique Shelf hunk paths with preview versions")
		}
		seen[s.Path] = true
		total += len(s.Hunks)
		if total > MaxImportedPatchHunks {
			return errors.New("selected Shelf hunks exceed 10000")
		}
		ordinals := map[int]bool{}
		for _, n := range s.Hunks {
			if n < 0 || ordinals[n] {
				return errors.New("invalid or duplicate Shelf hunk ordinal")
			}
			ordinals[n] = true
		}
	}
	return nil
}
