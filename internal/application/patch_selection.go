package application

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"vela/internal/domain"
)

// SelectParsedImportedPatch assembles a candidate from immutable captured parts
// on UI interaction. It issues no preview version. Backend review/apply always
// reparse the resulting Text and do not trust these UI metadata fields.
func SelectParsedImportedPatch(input domain.ImportedPatch, selected map[string][]int) (domain.ImportedPatch, error) {
	if len(input.Files) == 0 || len(input.Files) > 100 {
		return domain.ImportedPatch{}, errors.New("capture a supported patch before selecting hunks")
	}
	for path := range selected {
		if !slices.ContainsFunc(input.Files, func(file domain.PatchDocument) bool { return file.Path == path }) {
			return domain.ImportedPatch{}, fmt.Errorf("unknown patch path %q", path)
		}
	}
	doc := domain.ImportedPatch{Modes: make(map[string]string), Strip: 1}
	var result strings.Builder
	for _, file := range input.Files {
		indices := slices.Clone(selected[file.Path])
		slices.Sort(indices)
		if len(indices) == 0 {
			continue
		}
		if _, exists := doc.Modes[file.Path]; exists {
			return domain.ImportedPatch{}, errors.New("duplicate captured patch file")
		}
		mode := input.Modes[file.Path]
		if mode != "100644" && mode != "100755" && !(file.Kind == "R" && mode == "") {
			return domain.ImportedPatch{}, errors.New("unsupported captured patch mode")
		}
		part := file
		part.Hunks = nil
		result.WriteString(file.Header)
		if file.Kind != "" {
			if len(indices) != 1 || indices[0] != 0 {
				return domain.ImportedPatch{}, errors.New("structural, binary and mode changes must be selected as whole files")
			}
			part.Hunks = slices.Clone(file.Hunks)
			for _, h := range file.Hunks {
				result.WriteString(h.Text)
			}
			result.WriteString(file.Payload)
		} else {
			for i, index := range indices {
				if index < 0 || index >= len(file.Hunks) || i > 0 && index == indices[i-1] {
					return domain.ImportedPatch{}, fmt.Errorf("invalid hunk selection for %q", file.Path)
				}
				part.Hunks = append(part.Hunks, file.Hunks[index])
				result.WriteString(file.Hunks[index].Text)
			}
		}
		doc.Files = append(doc.Files, part)
		doc.Paths = append(doc.Paths, file.Path)
		doc.Modes[file.Path] = mode
		if file.OldPath != "" {
			doc.Paths = append(doc.Paths, file.OldPath)
			doc.Modes[file.OldPath] = mode
		}
		doc.Hunks += len(part.Hunks)
		if doc.Hunks > MaxImportedPatchHunks || result.Len() > MaxImportedPatchBytes {
			return domain.ImportedPatch{}, errors.New("selected patch exceeds import limits")
		}
	}
	if result.Len() == 0 {
		return domain.ImportedPatch{}, errors.New("select at least one patch hunk")
	}
	doc.Text = result.String()
	return doc, nil
}
