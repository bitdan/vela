package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui/editor"
	"vela/internal/application"
	"vela/internal/domain"
)

func projectReplacementPreview(value string) string {
	if value == "" {
		return "(empty)"
	}
	value = searchReplacementExcerpt(value)
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r", "\\r"), "\n", "\\n")
}

func searchReplacementExcerpt(value string) string {
	count := 0
	for i := range value {
		if count == 200 {
			return value[:i] + "…"
		}
		count++
	}
	return value
}

func (a *Workspace) projectReplaceDisabled(s *projectSearchDialog) string {
	if !s.replaceMode || s.root == "" || a.ReplaceProjectFiles == nil {
		return "Open Replace in Files for a local repository"
	}
	if s.loading || a.repositoryWrite != "" || a.repositoryRead != nil {
		return "Wait for the current task"
	}
	if a.Model.Active().Repository.ResolvingConflicts() || a.Model.Active().Repository.Bisect.Active() {
		return "Finish the current Git operation first"
	}
	if len(s.result.Matches) == 0 {
		return "Find matches to preview first"
	}
	if s.result.Limited || s.result.Unreadable > 0 {
		return "Narrow the search until results are complete and readable"
	}
	if !s.confirmed {
		return "Confirm replacing the previewed matches"
	}
	for _, m := range s.result.Matches {
		if m.InMemory || a.projectFileDirty(s.root, m.Path) {
			return "Save matching unsaved documents and run Find again"
		}
	}
	return ""
}

func (a *Workspace) applyProjectReplacement() {
	s := a.projectSearch
	input := application.ProjectReplaceInput{Query: application.ProjectSearchQuery{Text: s.query, MatchCase: s.matchCase, Regex: s.regex, PathFilter: s.pathFilter}, Replacement: s.replacement, Matches: slices.Clone(s.result.Matches)}
	repo := a.Model.Active().Repository
	paths := map[string]bool{}
	for _, m := range input.Matches {
		paths[m.Path] = true
	}
	locked := map[*editor.State]bool{}
	lock := func(entry *tab) {
		if entry == nil || entry.source != "project" || entry.code == nil || !paths[entry.path] {
			return
		}
		if _, exists := locked[entry.code]; exists {
			return
		}
		locked[entry.code] = entry.code.ReadOnly
		entry.code.ReadOnly = true
	}
	for i := range a.repo().tabs {
		lock(&a.repo().tabs[i])
	}
	lock(a.repo().split)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	a.closeProjectSearchDialog()
	a.beginRepositoryWrite(repo.Root, "Replacing project text…", cancel)
	a.projectReplaceWriting = repo.Root
	a.ReplaceProjectFiles(ctx, repo, input, func(updatedRepo domain.Repository, result application.ProjectReplaceResult, err, refreshErr error) {
		a.projectReplaceWriting = ""
		for state, readOnly := range locked {
			state.ReadOnly = readOnly
		}
		message := fmt.Sprintf("Replaced %d matches in %d files", result.Replacements, len(result.Paths))
		if err != nil {
			message += " · Stopped: " + err.Error()
		}
		a.finishRepositoryWrite(repo.Root, updatedRepo, refreshErr, message)
		for name := range paths {
			a.refreshOpenFileViews(repo.Root, name)
		}
		for i, session := range a.Model.Sessions {
			if session.Repository.Root == repo.Root {
				for _, name := range result.Paths {
					a.repos[i].addConsole("Replaced project text · " + name)
				}
				break
			}
		}
	})
}
