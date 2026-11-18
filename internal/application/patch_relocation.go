package application

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"vela/internal/domain"
)

// ResolveImportedPatchPath mirrors git apply: strip components first (the
// synthetic a/ or b/ counts as one), then prepend the repository-relative root.
// It never cleans away traversal or accepts platform-specific absolute paths.
func ResolveImportedPatchPath(source string, strip int, directory string) (string, error) {
	if strip == 0 {
		strip = 1
	} // Compatibility for callers predating relocation.
	if strip < 1 || strip > 16 {
		return "", errors.New("patch strip depth must be between 1 and 16")
	}
	valid := func(name string) bool {
		return name != "." && len(name) <= 4096 && fs.ValidPath(name) && !strings.ContainsAny(name, "\\:\x00\r\n")
	}
	if !valid(source) {
		return "", fmt.Errorf("invalid source patch path %q", source)
	}
	if directory != "" && !valid(directory) {
		return "", errors.New("patch destination must be a repository-relative directory without traversal, trailing slash, backslashes or drive letters")
	}
	parts := strings.Split(source, "/")
	if strip > len(parts) {
		return "", fmt.Errorf("strip depth %d removes the complete path %q", strip, source)
	}
	destination := strings.Join(parts[strip-1:], "/")
	if directory != "" {
		destination = directory + "/" + destination
	}
	if !valid(destination) {
		return "", errors.New("relocated patch path is invalid or exceeds 4096 bytes")
	}
	return destination, nil
}

// RelocateImportedPatch derives authoritative destination paths/modes from
// captured text, never caller-provided metadata. Headers and hunk text stay
// unchanged: Git receives the same strip/directory options during check/write.
func RelocateImportedPatch(text string, strip int, directory string) (domain.ImportedPatch, error) {
	doc, err := ParseImportedPatch(text)
	if err != nil {
		return doc, err
	}
	return RelocateParsedImportedPatch(doc, strip, directory)
}

// RelocateParsedImportedPatch maps an already captured parse for UI feedback
// without reparsing on each keystroke. It clears authority; adapter entry points
// must use RelocateImportedPatch to reparse Text before check or write.
func RelocateParsedImportedPatch(doc domain.ImportedPatch, strip int, directory string) (domain.ImportedPatch, error) {
	if strip == 0 {
		strip = 1
	}
	paths := make([]string, 0, len(doc.Paths))
	modes := make(map[string]string, len(doc.Modes))
	for _, source := range doc.Paths {
		destination, err := ResolveImportedPatchPath(source, strip, directory)
		if err != nil {
			return domain.ImportedPatch{}, err
		}
		if _, exists := modes[destination]; exists {
			return domain.ImportedPatch{}, fmt.Errorf("multiple patch files map to %q; change strip depth/directory or selection", destination)
		}
		paths = append(paths, destination)
		modes[destination] = doc.Modes[source]
	}
	doc.Paths, doc.Modes, doc.Strip, doc.Directory = paths, modes, strip, directory
	doc.Version = ""
	return doc, nil
}
