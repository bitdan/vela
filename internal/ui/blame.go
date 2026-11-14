package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"vela/internal/domain"
)

const showBlame commandID = "editor.show-blame"

func (a *Workspace) blameRevision(target commandTarget) string {
	source := target.source
	switch source {
	case "", "project", "working", "staged", "editor":
		return a.Model.Active().Repository.Head
	}
	for _, prefix := range []string{"revision:", "local:", "before:", "blame:"} {
		source = strings.TrimPrefix(source, prefix)
	}
	if fixedObjectID(source) {
		return source
	}
	return operationRef(a.Model.Active().Repository, source)
}

func fixedObjectID(source string) bool {
	return (len(source) == 40 || len(source) == 64) && strings.Trim(source, "0123456789abcdef") == ""
}

func blameText(doc domain.BlameDocument) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Blame · %s · %s\nCommitted content; working changes are not included.\n\n", doc.Path, doc.Revision)
	for _, line := range doc.Lines {
		fmt.Fprintf(&text, "%s  %s  %-18s  %6d │ %s\n", line.Commit, line.AuthoredAt.Format("2006-01-02"), strings.ReplaceAll(line.Author, "\t", " "), line.Line, line.Text)
	}
	return text.String()
}

func (a *Workspace) readFileSource(ctx context.Context, root, source string, file domain.File, done func(domain.File, error)) {
	if revision, ok := strings.CutPrefix(source, "blame:"); ok {
		if a.ReadRepositoryBlame == nil {
			done(file, errors.New("Blame reader is unavailable"))
			return
		}
		a.ReadRepositoryBlame(ctx, root, revision, file.Path, func(doc domain.BlameDocument, err error) {
			if err == nil {
				file.New = blameText(doc)
				file.Old = ""
			}
			done(file, err)
		})
		return
	}
	if a.ReadRepositoryFile == nil {
		done(file, errors.New("file reader is unavailable"))
		return
	}
	a.ReadRepositoryFile(ctx, root, source, file, done)
}
