package application

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
	"vela/internal/domain"
)

type RepositoryLineHistoryReader interface {
	ReadLineHistory(context.Context, domain.Repository, domain.LineHistoryQuery) (domain.LineHistoryPage, error)
}

func ValidateLineHistoryQuery(q domain.LineHistoryQuery) error {
	if err := ValidateCommitRangeSelection([]string{q.Revision}, 1); err != nil {
		return err
	}
	lines := strings.Count(q.Text, "\n")
	if q.Text != "" && !strings.HasSuffix(q.Text, "\n") {
		lines++
	}
	if q.Path == "" || q.DiskVersion == "" || len(q.Text) > 2<<20 || !utf8.ValidString(q.Text) || strings.ContainsRune(q.Text, 0) || lines > 20000 || q.Start < 1 || q.End < q.Start || q.End > lines || q.Offset < 0 || q.Limit < 1 || q.Limit > 100 || q.Offset > 1000-q.Limit {
		return errors.New("select committed text lines (2 MiB / 20,000 lines; 100 per page / 1,000 history entries)")
	}
	return nil
}

// ProjectLineHistoryPatch is a read-only projection of Git's -L hunks. It is
// never used to generate or apply a write patch.
func ProjectLineHistoryPatch(raw string) ([]domain.LineHistoryHunk, error) {
	if len(raw) > 2<<20 || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return nil, errors.New("range patch exceeds text preview limit")
	}
	rows := strings.SplitAfter(raw, "\n")
	var result []domain.LineHistoryHunk
	for i := 0; i < len(rows); {
		row := rows[i]
		if !strings.HasPrefix(row, "@@ ") {
			if row == "" || row == "\n" || strings.HasPrefix(row, "diff --git ") || strings.HasPrefix(row, "--- ") || strings.HasPrefix(row, "+++ ") || importedIndexHeader.MatchString(strings.TrimSuffix(row, "\n")) || strings.HasPrefix(row, "new file mode ") || strings.HasPrefix(row, "deleted file mode ") || strings.HasPrefix(row, "old mode ") || strings.HasPrefix(row, "new mode ") || strings.HasPrefix(row, "rename from ") || strings.HasPrefix(row, "rename to ") || strings.HasPrefix(row, "similarity index ") {
				i++
				continue
			}
			return nil, fmt.Errorf("invalid line-history patch header: %q", row)
		}
		m := patchHunkHeader.FindStringSubmatch(row)
		if m == nil {
			return nil, errors.New("invalid line-history hunk")
		}
		values := [4]int{}
		for n := range values {
			v := m[n+1]
			if v == "" {
				v = "1"
			}
			num, err := strconv.Atoi(v)
			if err != nil || num > 1<<30 {
				return nil, errors.New("invalid range positions")
			}
			values[n] = num
		}
		h := domain.LineHistoryHunk{OldStart: values[0], NewStart: values[2]}
		var before, after bytes.Buffer
		oldCount, newCount := 0, 0
		last := byte(0)
		i++
		for i < len(rows) {
			line := rows[i]
			if line == "\\ No newline at end of file\n" {
				if last == 0 {
					return nil, errors.New("orphan range newline marker")
				}
				if last == '-' || last == ' ' {
					before.Truncate(before.Len() - 1)
				}
				if last == '+' || last == ' ' {
					after.Truncate(after.Len() - 1)
				}
				last = 0
				i++
				continue
			}
			if oldCount == values[1] && newCount == values[3] {
				break
			}
			if !strings.HasSuffix(line, "\n") {
				break
			}
			switch line[0] {
			case ' ':
				before.WriteString(line[1:])
				after.WriteString(line[1:])
				oldCount++
				newCount++
			case '-':
				before.WriteString(line[1:])
				oldCount++
			case '+':
				after.WriteString(line[1:])
				newCount++
			default:
				return nil, errors.New("invalid range content")
			}
			last = line[0]
			i++
			if oldCount > values[1] || newCount > values[3] {
				return nil, errors.New("range content exceeds counts")
			}
		}
		if oldCount != values[1] || newCount != values[3] {
			return nil, errors.New("incomplete range content")
		}
		h.Before, h.After = before.String(), after.String()
		result = append(result, h)
		if len(result) > 10000 {
			return nil, errors.New("too many range hunks")
		}
	}
	if len(result) == 0 {
		return nil, errors.New("line-history record has no native hunks")
	}
	return result, nil
}
