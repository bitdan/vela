package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"vela/internal/domain"
)

func CompileProjectSearch(q ProjectSearchQuery) (*regexp.Regexp, error) {
	if q.Text == "" || len(q.Text) > 4096 || !utf8.ValidString(q.Text) || strings.ContainsAny(q.Text, "\r\n\x00") {
		return nil, errors.New("enter a single-line query up to 4096 UTF-8 bytes")
	}
	expression := q.Text
	if !q.Regex {
		expression = regexp.QuoteMeta(expression)
	}
	if !q.MatchCase {
		expression = "(?i)" + expression
	}
	matcher, err := regexp.Compile(expression)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression: %w", err)
	}
	return matcher, nil
}

type projectPathMask struct {
	pattern                   string
	exclude, basename, prefix bool
}

// Masks use path.Match syntax. A trailing slash matches a subtree, and masks
// without a slash match basenames at every depth. ! masks exclude paths.
func CompileProjectPathFilter(value string) (func(string) bool, error) {
	if len(value) > 4096 {
		return nil, errors.New("path filter exceeds 4096 bytes")
	}
	var masks []projectPathMask
	hasInclude := false
	for _, item := range strings.Split(value, ";") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		m := projectPathMask{exclude: strings.HasPrefix(item, "!")}
		if m.exclude {
			item = strings.TrimPrefix(item, "!")
		}
		if item == "" || strings.ContainsAny(item, "\\\x00\r\n") || strings.HasPrefix(item, "/") || strings.Contains(item, "../") {
			return nil, errors.New("use project-relative path masks separated by semicolons")
		}
		m.pattern, m.prefix = item, strings.HasSuffix(item, "/")
		m.basename = !strings.Contains(item, "/")
		if _, err := path.Match(item, ""); err != nil {
			return nil, fmt.Errorf("invalid path mask %q: %w", item, err)
		}
		if !m.exclude {
			hasInclude = true
		}
		masks = append(masks, m)
	}
	return func(name string) bool {
		included := !hasInclude
		for _, m := range masks {
			candidate := name
			if m.basename {
				candidate = path.Base(name)
			}
			matched, _ := path.Match(m.pattern, candidate)
			if m.prefix {
				matched = false
				for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
					if ok, _ := path.Match(strings.TrimSuffix(m.pattern, "/"), parent); ok {
						matched = true
						break
					}
				}
			}
			if matched {
				if m.exclude {
					return false
				}
				included = true
			}
		}
		return included
	}, nil
}

func ProjectSearchRawVersion(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Expand one capture token at a time so a short template cannot allocate an
// unbounded string by repeating a large capture many times.
func projectMatchReplacement(matcher *regexp.Regexp, template, text string, match []int) (string, error) {
	var out strings.Builder
	appendText := func(value string) error {
		if len(value) > MaxProjectSearchFileBytes-out.Len() {
			return errors.New("expanded replacement exceeds 2 MiB")
		}
		out.WriteString(value)
		return nil
	}
	for template != "" {
		before, after, found := strings.Cut(template, "$")
		if err := appendText(before); err != nil {
			return "", err
		}
		if !found {
			break
		}
		if after == "" {
			if err := appendText("$"); err != nil {
				return "", err
			}
			break
		}
		if after[0] == '$' {
			if err := appendText("$"); err != nil {
				return "", err
			}
			template = after[1:]
			continue
		}
		end := 0
		if after[0] == '{' {
			if n := strings.IndexByte(after, '}'); n >= 0 {
				end = n + 1
			}
		} else {
			for i, r := range after {
				if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
					break
				}
				end = i + utf8.RuneLen(r)
			}
		}
		if end == 0 {
			if err := appendText("$"); err != nil {
				return "", err
			}
			template = after
			continue
		}
		piece := string(matcher.ExpandString(nil, "$"+after[:end], text, match))
		if err := appendText(piece); err != nil {
			return "", err
		}
		template = after[end:]
	}
	return out.String(), nil
}

type ProjectReplaceInput struct {
	Query       ProjectSearchQuery
	Replacement string
	Matches     []ProjectSearchMatch
}

// ProjectReplaceResult reports durable progress, including on partial failure.
type ProjectReplaceResult struct {
	Paths        []string
	Replacements int
}

type ProjectReplacer interface {
	ReplaceProject(context.Context, domain.Repository, ProjectReplaceInput) (ProjectReplaceResult, error)
}

// ReplaceProjectText checks every previewed match against the same query and
// exact disk bytes. It preserves uniform LF/CRLF and refuses mixed line endings.
func ReplaceProjectText(text string, input ProjectReplaceInput, matches []ProjectSearchMatch) (string, error) {
	if len(text) > MaxProjectSearchFileBytes || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return "", errors.New("replace requires a regular UTF-8 text file up to 2 MiB")
	}
	if len(input.Replacement) > 4096 || !utf8.ValidString(input.Replacement) || strings.ContainsRune(input.Replacement, 0) {
		return "", errors.New("replacement must be UTF-8 text up to 4096 bytes without NUL")
	}
	matcher, err := CompileProjectSearch(input.Query)
	if err != nil {
		return "", err
	}
	lf := strings.ReplaceAll(text, "\r\n", "\n")
	crlf := strings.Contains(text, "\r\n")
	if crlf && strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
		return "", errors.New("mixed LF/CRLF line endings cannot be replaced")
	}
	positions := matcher.FindAllStringSubmatchIndex(lf, MaxProjectSearchMatches+1)
	if len(positions) != len(matches) || len(matches) == 0 {
		return "", errors.New("match set changed; run Find again")
	}
	version, raw := ProjectSearchVersion(lf), ProjectSearchRawVersion(text)
	replacementTemplate := strings.ReplaceAll(input.Replacement, "\r\n", "\n")
	var out strings.Builder
	previous := 0
	for i, pos := range positions {
		m := matches[i]
		if m.InMemory || m.Version != version || m.RawVersion != raw || m.Start != pos[0] || m.End != pos[1] {
			return "", errors.New("file or preview changed; run Find again")
		}
		replacement := replacementTemplate
		if input.Query.Regex {
			replacement, err = projectMatchReplacement(matcher, replacementTemplate, lf, pos)
			if err != nil {
				return "", err
			}
		}
		if replacement != m.Replacement {
			return "", errors.New("replacement preview changed; run Find again")
		}
		if out.Len()+pos[0]-previous+len(replacement) > MaxProjectSearchFileBytes {
			return "", errors.New("replacement output exceeds 2 MiB")
		}
		out.WriteString(lf[previous:pos[0]])
		out.WriteString(replacement)
		previous = pos[1]
	}
	if out.Len()+len(lf)-previous > MaxProjectSearchFileBytes {
		return "", errors.New("replacement output exceeds 2 MiB")
	}
	out.WriteString(lf[previous:])
	result := out.String()
	if crlf {
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}
	if len(result) > MaxProjectSearchFileBytes {
		return "", errors.New("replacement output exceeds 2 MiB")
	}
	return result, nil
}
