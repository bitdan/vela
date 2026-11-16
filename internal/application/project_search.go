package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	MaxProjectSearchMatches   = 1000
	MaxProjectSearchFiles     = 20000
	MaxProjectSearchFileBytes = 2 << 20
	MaxProjectSearchBytes     = 64 << 20
)

var ErrProjectSearchSkipped = errors.New("unsupported search file")

type ProjectSearchQuery struct {
	Text       string
	MatchCase  bool
	Regex      bool
	PathFilter string
}

// ProjectSearchInput is an immutable snapshot of project scope and unsaved text.
type ProjectSearchInput struct {
	Root           string
	Paths          []string
	Query          ProjectSearchQuery
	Overlays       map[string]string
	IncludeIgnored bool
	Replacement    string
}
type ProjectSearchMatch struct {
	Path, Preview, Version   string
	Start, End, Line, Column int
	InMemory                 bool
	RawVersion, Replacement  string
}
type ProjectSearchResult struct {
	Matches                      []ProjectSearchMatch
	Scanned, Skipped, Unreadable int
	Limited                      bool
	FirstError                   string
}
type ProjectSearcher interface {
	SearchProject(context.Context, ProjectSearchInput) (ProjectSearchResult, error)
}

// ProjectSearchVersion fingerprints the LF text used by native editor offsets.
func ProjectSearchVersion(text string) string {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(text, "\r\n", "\n")))
	return hex.EncodeToString(sum[:])
}

// ValidProjectSearchPath excludes traversal and Git metadata on all platforms.
func ValidProjectSearchPath(name string) bool {
	if name == "" || path.IsAbs(name) || path.Clean(name) != name || name == "." || strings.ContainsAny(name, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || strings.EqualFold(strings.TrimRight(part, " ."), ".git") || strings.HasPrefix(strings.ToLower(part), "git~") || strings.HasPrefix(strings.ToLower(part), ".git~") {
			return false
		}
	}
	return true
}

// SearchProject performs bounded matching; read owns filesystem IO.
// Unsupported files and read errors are counted explicitly rather than treated
// as complete searches. Callers must keep input immutable until it returns.
func SearchProject(ctx context.Context, input ProjectSearchInput, read func(context.Context, string) (string, error)) (ProjectSearchResult, error) {
	var out ProjectSearchResult
	q := input.Query
	if len(input.Replacement) > 4096 || !utf8.ValidString(input.Replacement) || strings.ContainsRune(input.Replacement, 0) {
		return out, errors.New("replacement must be UTF-8 text up to 4096 bytes without NUL")
	}
	matcher, err := CompileProjectSearch(q)
	if err != nil {
		return out, err
	}
	replacementTemplate := strings.ReplaceAll(input.Replacement, "\r\n", "\n")
	filter, err := CompileProjectPathFilter(q.PathFilter)
	if err != nil {
		return out, err
	}
	paths := slices.Clone(input.Paths)
	slices.Sort(paths)
	paths = slices.Compact(paths)
	eligible := make([]string, 0, min(len(paths), MaxProjectSearchFiles))
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if !ValidProjectSearchPath(name) {
			return out, fmt.Errorf("invalid project search path %q", name)
		}
		if !filter(name) {
			continue
		}
		if len(eligible) == MaxProjectSearchFiles {
			out.Limited = true
			break
		}
		eligible = append(eligible, name)
	}
	paths = eligible
	total, replacementBytes := 0, 0
	for _, name := range paths {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if !ValidProjectSearchPath(name) {
			return out, fmt.Errorf("invalid project search path %q", name)
		}
		if !filter(name) {
			continue
		}
		text, memory := input.Overlays[name]
		if !memory {
			if read == nil {
				return out, errors.New("project search reader is unavailable")
			}
			var err error
			text, err = read(ctx, name)
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if err != nil {
				if errors.Is(err, ErrProjectSearchSkipped) {
					out.Skipped++
				} else {
					out.Unreadable++
					if out.FirstError == "" {
						out.FirstError = fmt.Sprintf("%s · %v", name, err)
					}
				}
				continue
			}
		}
		if len(text) > MaxProjectSearchFileBytes || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
			out.Skipped++
			continue
		}
		if len(text) > MaxProjectSearchBytes-total {
			out.Limited = true
			break
		}
		total += len(text)
		out.Scanned++
		rawVersion := ProjectSearchRawVersion(text)
		text = strings.ReplaceAll(text, "\r\n", "\n")
		remaining := MaxProjectSearchMatches - len(out.Matches)
		positions := matcher.FindAllStringSubmatchIndex(text, remaining+1)
		if len(positions) == 0 {
			continue
		}
		version := ProjectSearchVersion(text)
		line, column, scan, lineStart, lineEnd := 1, 1, 0, 0, -1
		preview := ""
		for i, pos := range positions {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			if i == remaining {
				out.Limited = true
				return out, nil
			}
			between := text[scan:pos[0]]
			if newline := strings.LastIndexByte(between, '\n'); newline >= 0 {
				line += strings.Count(between, "\n")
				lineStart = scan + newline + 1
				column = utf8.RuneCountInString(text[lineStart:pos[0]]) + 1
			} else {
				column += utf8.RuneCountInString(between)
			}
			scan = pos[0]
			if lineEnd < pos[0] {
				lineEnd = len(text)
				if n := strings.IndexByte(text[lineStart:], '\n'); n >= 0 {
					lineEnd = lineStart + n
				}
				preview = searchLinePreview(text[lineStart:lineEnd])
			}
			replacement := replacementTemplate
			if q.Regex {
				replacement, err = projectMatchReplacement(matcher, replacementTemplate, text, pos)
				if err != nil {
					return out, err
				}
			}
			replacementBytes += len(replacement)
			if replacementBytes > MaxProjectSearchBytes {
				return out, errors.New("replacement previews exceed 64 MiB")
			}
			out.Matches = append(out.Matches, ProjectSearchMatch{Path: name, Preview: preview, Version: version, RawVersion: rawVersion, Replacement: replacement, Start: pos[0], End: pos[1], Line: line, Column: column, InMemory: memory})
		}
	}
	return out, ctx.Err()
}

func searchLinePreview(line string) string {
	line = strings.TrimSpace(line)
	count := 0
	for offset := range line {
		if count == 400 {
			return line[:offset] + "…"
		}
		count++
	}
	return line
}
