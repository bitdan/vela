package application

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

const MaxImportedPatchBytes = 2 << 20
const MaxImportedPatchHunks = 10000

type RepositoryPatchImporter interface {
	PreviewImportedPatch(context.Context, domain.Repository, string, string) (domain.ImportedPatch, error)
	ApplyImportedPatch(context.Context, domain.Repository, domain.ImportedPatch) error
}

// RepositoryPatchReviewer checks captured/selected text without rereading the
// external source. The adapter reparses it before issuing a preview version.
type RepositoryPatchReviewer interface {
	ReviewImportedPatch(context.Context, domain.Repository, domain.ImportedPatch) (domain.ImportedPatch, error)
}

// ParseImportedPatch accepts bounded Git diffs, including whole-file binary,
// regular mode, addition, deletion and rename changes. Endpoint headers must agree; unsupported
// metadata is rejected. ParseTextPatch checks hunk counts and newline markers.
func ParseImportedPatch(text string) (domain.ImportedPatch, error) {
	var result domain.ImportedPatch
	if len(text) == 0 || len(text) > MaxImportedPatchBytes || !utf8.ValidString(text) || strings.ContainsAny(text, "\x00\r") || !strings.HasPrefix(text, "diff --git ") || !strings.HasSuffix(text, "\n") {
		return result, errors.New("choose a UTF-8 Git patch of at most 2 MiB, without an email envelope")
	}
	parts := strings.Split(text, "\ndiff --git ")
	if len(parts) > 100 {
		return result, errors.New("patch import supports at most 100 files")
	}
	seen := map[string]bool{}
	result.Modes = map[string]string{}
	for i, part := range parts {
		if i > 0 {
			part = "diff --git " + part
		}
		if i < len(parts)-1 {
			part += "\n"
		}
		doc, mode, err := parseImportedFile(part)
		if err != nil {
			return result, err
		}
		paths := []string{doc.Path}
		if doc.OldPath != "" {
			paths = append(paths, doc.OldPath)
		}
		for _, path := range paths {
			if seen[path] {
				return result, errors.New("overlapping patch endpoints are unsupported")
			}
			seen[path] = true
			result.Paths = append(result.Paths, path)
			result.Modes[path] = mode
		}
		result.Hunks += len(doc.Hunks)
		if result.Hunks > MaxImportedPatchHunks {
			return domain.ImportedPatch{}, errors.New("patch import supports at most 10000 hunks")
		}
		result.Files = append(result.Files, doc)
	}
	result.Text = text
	result.Strip = 1
	return result, nil
}

// SelectImportedPatch keeps exact Git headers and hunk bodies in source order.
// Invalid identities and empty selections fail rather than widening the scope.
func SelectImportedPatch(text string, selected map[string][]int) (domain.ImportedPatch, error) {
	doc, err := ParseImportedPatch(text)
	if err != nil {
		return domain.ImportedPatch{}, err
	}
	doc, err = SelectParsedImportedPatch(doc, selected)
	if err != nil {
		return domain.ImportedPatch{}, err
	}
	return ParseImportedPatch(doc.Text)
}
