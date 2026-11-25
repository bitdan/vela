package application

import (
	"errors"
	"fmt"
	"strings"

	"vela/internal/domain"
)

type ChangedPatchLine struct {
	Ordinal int
	Text    string
}
type patchContentLine struct {
	text      string
	noNewline bool
	ordinal   int
}

func patchContentLines(h domain.PatchHunk) ([]patchContentLine, error) {
	_, body, ok := strings.Cut(h.Text, "\n")
	if !ok {
		return nil, errors.New("invalid hunk header")
	}
	var out []patchContentLine
	ordinal := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		if line == "" {
			continue
		}
		if line == "\\ No newline at end of file\n" {
			if len(out) == 0 || out[len(out)-1].noNewline {
				return nil, errors.New("invalid newline marker")
			}
			out[len(out)-1].noNewline = true
			continue
		}
		if len(line) < 2 || !strings.ContainsRune(" +-", rune(line[0])) || !strings.HasSuffix(line, "\n") {
			return nil, errors.New("invalid patch content")
		}
		i := -1
		if line[0] != ' ' {
			i = ordinal
			ordinal++
		}
		out = append(out, patchContentLine{text: line, ordinal: i})
	}
	return out, nil
}

func ChangedPatchLines(h domain.PatchHunk) ([]ChangedPatchLine, error) {
	lines, err := patchContentLines(h)
	if err != nil {
		return nil, err
	}
	var out []ChangedPatchLine
	for _, l := range lines {
		if l.ordinal >= 0 {
			out = append(out, ChangedPatchLine{l.ordinal, l.text})
		}
	}
	return out, nil
}

// SelectPatchLines constructs a partial patch against the actual target side.
// For unstage, unchecked additions remain context and unchecked removals vanish.
// The caller applies the returned patch in reverse. Content bytes stay exact.
func SelectPatchLines(doc domain.PatchDocument, hunk int, selected []int, reverse bool) (string, error) {
	bad := func() (string, error) {
		return "", errors.New("select 1–10000 unique changed lines in this Git hunk")
	}
	if hunk < 0 || hunk >= len(doc.Hunks) || len(selected) == 0 || len(selected) > 10000 {
		return bad()
	}
	h := doc.Hunks[hunk]
	lines, err := patchContentLines(h)
	if err != nil {
		return "", err
	}
	valid := 0
	for _, l := range lines {
		if l.ordinal >= 0 {
			valid++
		}
	}
	chosen := make(map[int]bool, len(selected))
	for _, i := range selected {
		if i < 0 || i >= valid || chosen[i] {
			return bad()
		}
		chosen[i] = true
	}
	var projected []patchContentLine
	appendLine := func(l patchContentLine) {
		if !chosen[l.ordinal] {
			if !reverse && l.text[0] == '+' || reverse && l.text[0] == '-' {
				return
			}
			l.text = " " + l.text[1:]
		}
		projected = append(projected, l)
	}
	// Pair adjacent removal/addition runs by row. Turning unchecked removals
	// into context before every addition would move replacements past them.
	for i := 0; i < len(lines); {
		if lines[i].text[0] == ' ' {
			projected = append(projected, lines[i])
			i++
			continue
		}
		var removed, added []patchContentLine
		for i < len(lines) && lines[i].text[0] != ' ' {
			if lines[i].text[0] == '-' {
				removed = append(removed, lines[i])
			} else {
				added = append(added, lines[i])
			}
			i++
		}
		for n := 0; n < max(len(removed), len(added)); n++ {
			if n < len(removed) {
				appendLine(removed[n])
			}
			if n < len(added) {
				appendLine(added[n])
			}
		}
	}
	oldCount, newCount := 0, 0
	oldEOF, newEOF := false, false
	var body strings.Builder
	for _, l := range projected {
		if l.text[0] != '+' {
			if oldEOF {
				return "", errors.New("selection cannot preserve an unterminated internal line")
			}
			oldCount++
			oldEOF = l.noNewline
		}
		if l.text[0] != '-' {
			if newEOF {
				return "", errors.New("selection cannot preserve an unterminated internal line")
			}
			newCount++
			newEOF = l.noNewline
		}
		body.WriteString(l.text)
		if l.noNewline {
			body.WriteString("\\ No newline at end of file\n")
		}
	}
	position := h.OldStart
	if h.OldCount == 0 {
		position++
	}
	if reverse {
		position = h.NewStart
		if h.NewCount == 0 {
			position++
		}
	}
	oldStart, newStart := position, position
	if oldCount == 0 {
		oldStart--
	}
	if newCount == 0 {
		newStart--
	}
	raw := doc.Header + fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount) + body.String()
	if _, err := ParseTextPatch(raw, doc.Path); err != nil {
		return "", err
	}
	return raw, nil
}
