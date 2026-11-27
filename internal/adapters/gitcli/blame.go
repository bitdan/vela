package gitcli

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"vela/internal/domain"
)

const maxBlameLines = 20000

func (g *Executor) ReadBlame(ctx context.Context, root, revision, path string) (domain.BlameDocument, error) {
	ctx = canonicalHistory(ctx)
	doc := domain.BlameDocument{Path: path, Revision: revision}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return doc, err
	}
	if !validGitPath(path) || !validObjectID(revision) {
		return doc, errors.New("select a committed file and valid revision")
	}
	text, err := g.readBlob(ctx, root, revision+":"+path)
	if err != nil {
		return doc, err
	}
	if strings.Count(text, "\n")+1 > maxBlameLines {
		return doc, fmt.Errorf("blame exceeds the %d-line preview limit", maxBlameLines)
	}
	output, err := g.read(ctx, root, "--literal-pathspecs", "blame", "--line-porcelain", "--encoding=UTF-8", "--no-textconv", revision, "--", path)
	if err != nil {
		return doc, err
	}
	doc.Text = text
	doc.Lines, err = parseBlame(output)
	return doc, err
}

func parseBlame(output string) ([]domain.BlameLine, error) {
	rows := strings.Split(output, "\n")
	var lines []domain.BlameLine
	for i := 0; i < len(rows); {
		if i == len(rows)-1 && rows[i] == "" {
			break
		}
		header := strings.Fields(rows[i])
		i++
		if len(header) < 3 || len(header) > 4 || !validObjectID(header[0]) {
			return nil, errors.New("invalid blame header")
		}
		original, e1 := strconv.Atoi(header[1])
		line, e2 := strconv.Atoi(header[2])
		if e1 != nil || e2 != nil || original < 1 || line != len(lines)+1 {
			return nil, errors.New("invalid blame line positions")
		}
		entry := domain.BlameLine{Commit: header[0], OriginalLine: original, Line: line}
		hasTime, hasAuthor, hasText := false, false, false
		for i < len(rows) {
			row := rows[i]
			i++
			if strings.HasPrefix(row, "\t") {
				entry.Text = strings.TrimPrefix(row, "\t")
				hasText = true
				break
			}
			key, value, _ := strings.Cut(row, " ")
			switch key {
			case "author":
				entry.Author = value
				hasAuthor = true
			case "author-time":
				epoch, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return nil, errors.New("invalid blame author time")
				}
				entry.AuthoredAt = time.Unix(epoch, 0).UTC()
				hasTime = true
			case "summary":
				entry.Summary = value
			}
		}
		if !hasTime || !hasAuthor || !hasText {
			return nil, errors.New("incomplete blame record")
		}
		lines = append(lines, entry)
		if len(lines) > maxBlameLines {
			return nil, errors.New("blame preview has too many lines")
		}
	}
	return lines, nil
}
