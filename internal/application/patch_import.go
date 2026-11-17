package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

const MaxImportedPatchBytes = 2 << 20

type RepositoryPatchImporter interface {
	PreviewImportedPatch(context.Context, domain.Repository, string, string) (domain.ImportedPatch, error)
	ApplyImportedPatch(context.Context, domain.Repository, domain.ImportedPatch) error
}

// ParseImportedPatch accepts Git unified diffs of existing regular text files.
// Every header must name the same path; no email envelope or extended operation
// headers are accepted. ParseTextPatch checks hunk counts and newline markers.
func ParseImportedPatch(text string) (domain.ImportedPatch, error) {
	var result domain.ImportedPatch
	if len(text) == 0 || len(text) > MaxImportedPatchBytes || !utf8.ValidString(text) || strings.ContainsAny(text, "\x00\r") || !strings.HasPrefix(text, "diff --git ") {
		return result, errors.New("choose a UTF-8 Git text patch of at most 2 MiB, without an email envelope")
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
		lines := strings.SplitN(part, "\n", 5)
		if len(lines) < 5 || !strings.HasPrefix(lines[2], "--- ") || !strings.HasPrefix(lines[3], "+++ ") {
			return result, errors.New("only ordinary text modifications with Git index headers are supported")
		}
		oldToken := strings.TrimSuffix(strings.TrimPrefix(lines[2], "--- "), "\t")
		newToken := strings.TrimSuffix(strings.TrimPrefix(lines[3], "+++ "), "\t")
		decode := func(token string) (string, error) {
			if strings.HasPrefix(token, "\"") {
				return strconv.Unquote(token)
			}
			return token, nil
		}
		oldPath, err := decode(oldToken)
		if err != nil {
			return result, fmt.Errorf("invalid patch source path: %w", err)
		}
		newPath, err := decode(newToken)
		if err != nil || !strings.HasPrefix(oldPath, "a/") || !strings.HasPrefix(newPath, "b/") || oldPath[2:] != newPath[2:] || lines[0] != "diff --git "+oldToken+" "+newToken {
			return result, errors.New("patch headers must identify one unchanged repository-relative path")
		}
		path := oldPath[2:]
		if seen[path] {
			return result, errors.New("duplicate patch file sections are unsupported")
		}
		seen[path] = true
		doc, err := ParseTextPatch(part, path)
		if err != nil {
			return result, err
		}
		result.Paths = append(result.Paths, path)
		result.Modes[path] = strings.Fields(lines[1])[2]
		result.Hunks += len(doc.Hunks)
	}
	result.Text = text
	return result, nil
}
