package application

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

var patchIndexHeader = regexp.MustCompile(`^index [0-9a-f]+\.\.[0-9a-f]+ (100644|100755)\n$`)
var patchHunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@[^\n]*\n$`)

// ParseTextPatch accepts only ordinary tracked-file content changes. It retains
// Git's exact quoted headers and newline markers; Git apply remains authoritative.
func ParseTextPatch(raw, path string) (domain.PatchDocument, error) {
	doc := domain.PatchDocument{Path: path}
	bad := func() (domain.PatchDocument, error) {
		return domain.PatchDocument{}, fmt.Errorf("unsupported or malformed text patch for %q", path)
	}
	if len(raw) > 2<<20 || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) || !strings.HasSuffix(raw, "\n") {
		return bad()
	}
	lines := strings.SplitAfter(raw, "\n")
	lines = lines[:len(lines)-1]
	// Git quotes the complete a/path when a filename needs C-style quoting.
	if len(lines) < 5 || !strings.HasPrefix(lines[0], "diff --git ") || !patchIndexHeader.MatchString(lines[1]) || !(strings.HasPrefix(lines[2], "--- a/") || strings.HasPrefix(lines[2], "--- \"a/")) || !(strings.HasPrefix(lines[3], "+++ b/") || strings.HasPrefix(lines[3], "+++ \"b/")) {
		return bad()
	}
	doc.Header = strings.Join(lines[:4], "")
	for i := 4; i < len(lines); {
		match := patchHunkHeader.FindStringSubmatch(lines[i])
		if match == nil {
			return bad()
		}
		vals := [4]int{}
		for n := range vals {
			value := match[n+1]
			if value == "" {
				value = "1"
			}
			v, err := strconv.Atoi(value)
			if err != nil || v > 1<<30 {
				return bad()
			}
			vals[n] = v
		}
		h := domain.PatchHunk{OldStart: vals[0], OldCount: vals[1], NewStart: vals[2], NewCount: vals[3]}
		if h.OldStart == 0 && h.OldCount != 0 || h.NewStart == 0 && h.NewCount != 0 {
			return bad()
		}
		start := i
		i++
		oldCount, newCount, changes := 0, 0, 0
		previousContent := false
		for i < len(lines) && !strings.HasPrefix(lines[i], "@@ ") {
			line := lines[i]
			if line == "\\ No newline at end of file\n" {
				if !previousContent {
					return bad()
				}
				previousContent = false
				i++
				continue
			}
			switch line[0] {
			case ' ':
				oldCount++
				newCount++
			case '-':
				oldCount++
				changes++
			case '+':
				newCount++
				changes++
			default:
				return bad()
			}
			previousContent = true
			i++
		}
		if oldCount != h.OldCount || newCount != h.NewCount || changes == 0 {
			return bad()
		}
		h.Text = strings.Join(lines[start:i], "")
		doc.Hunks = append(doc.Hunks, h)
	}
	if len(doc.Hunks) == 0 {
		return bad()
	}
	return doc, nil
}
