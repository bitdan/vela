package application

import (
	"errors"
	"strings"
	"unicode/utf8"

	"vela/internal/domain"
)

// UniformTextLineEnding validates raw content without normalizing it. A final
// unterminated line is allowed; an empty/one-line file has no determined EOL.
func UniformTextLineEnding(text string) (string, error) {
	if !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return "", errors.New("patch requires UTF-8 text without NUL")
	}
	style := ""
	for len(text) > 0 {
		line, rest, terminated := strings.Cut(text, "\n")
		next := ""
		if terminated {
			next = "\n"
			if strings.HasSuffix(line, "\r") {
				next = "\r\n"
				line = strings.TrimSuffix(line, "\r")
			}
		}
		if strings.ContainsRune(line, '\r') {
			return "", errors.New("patch does not support bare CR")
		}
		if next != "" {
			if style != "" && next != style {
				return "", errors.New("patch does not support mixed line endings")
			}
			style = next
		}
		text = rest
	}
	return style, nil
}

// ImportedPatchLineEnding distinguishes native LF Git framing from content
// CRLF. No-newline markers describe the preceding content line, not an LF EOL.
func ImportedPatchLineEnding(file domain.PatchDocument) (string, error) {
	if strings.ContainsAny(file.Header+file.Payload, "\r") {
		return "", errors.New("patch headers and binary envelope must use native LF framing")
	}
	style := ""
	for _, h := range file.Hunks {
		lines := strings.SplitAfter(strings.TrimSuffix(h.Text, "\n"), "\n")
		if len(lines) == 0 || strings.ContainsRune(lines[0], '\r') {
			return "", errors.New("patch hunk headers must use native LF framing")
		}
		for i := 1; i < len(lines); i++ {
			line := lines[i]
			if strings.HasPrefix(line, "\\ No newline at end of file") {
				continue
			}
			if len(line) == 0 || (line[0] != ' ' && line[0] != '+' && line[0] != '-') {
				return "", errors.New("invalid patch content line")
			}
			// SplitAfter above omitted the final framing LF, restore it unless
			// the following marker says it wasn't part of the content.
			if i == len(lines)-1 {
				line += "\n"
			}
			if i+1 < len(lines) && strings.HasPrefix(lines[i+1], "\\ No newline at end of file") {
				line = strings.TrimSuffix(line, "\n")
			}
			next, err := UniformTextLineEnding(line[1:])
			if err != nil {
				return "", err
			}
			if next != "" {
				if style != "" && style != next {
					return "", errors.New("patch does not support mixed line endings in a file")
				}
				style = next
			}
		}
	}
	return style, nil
}
