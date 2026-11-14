package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

type indexChange struct {
	root  string
	repo  domain.Repository
	stage bool
	files []domain.File
}

func (a *Workspace) indexFiles(stage bool, target commandTarget) []domain.File {
	r, repo := a.repo(), a.Model.Active().Repository
	files, chosen := repo.Staged, &r.stagedChosen
	if stage {
		files, chosen = repo.Working, &r.workingChosen
	}
	useSelection := target.source != "editor" && chosen.Has(target.file.Path)
	var result []domain.File
	for _, file := range files {
		if file.Path == target.file.Path || (useSelection && chosen.Has(file.Path)) {
			result = append(result, file)
		}
	}
	return result
}

func (a *Workspace) indexDisabled(stage bool, target commandTarget) string {
	if a.UpdateRepositoryIndex == nil {
		return "Index operations are unavailable"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || a.indexConfirmation != nil || a.commitConfirmation != nil || a.repositoryOperation != nil {
		return "Wait for the current repository operation to finish"
	}
	if a.Model.Active().Repository.IndexVersion == "" {
		return "Refresh the repository before changing the index"
	}
	if target.file.Path == "" {
		return "Select a changed file"
	}
	files := a.indexFiles(stage, target)
	if len(files) == 0 {
		return "Select a changed file"
	}
	for _, file := range files {
		if file.Status == "U" {
			return "Resolve the merge conflict before changing the index"
		}
	}
	return ""
}

func (a *Workspace) requestIndexChange(stage bool, target commandTarget) {
	repo := a.Model.Active().Repository
	change := &indexChange{root: repo.Root, repo: repo, stage: stage, files: a.indexFiles(stage, target)}
	other := repo.Working
	if stage {
		other = repo.Staged
	}
	selectedPaths := make(map[string]bool)
	for _, file := range change.files {
		selectedPaths[file.Path] = true
		if file.Status == "R" && file.OldPath != "" {
			selectedPaths[file.OldPath] = true
		}
	}
	for _, file := range other {
		if selectedPaths[file.Path] || (file.Status == "R" && selectedPaths[file.OldPath]) {
			a.indexConfirmation = change
			return
		}
	}
	a.applyIndexChange(change)
}

func (a *Workspace) confirmIndexChange(c *ui.Context) {
	change := a.indexConfirmation
	action, effect := "Unstage", "This removes the selected staged versions from the index. The working files are kept as they are."
	if change.stage {
		action, effect = "Stage", "This replaces the selected staged versions with the current working files."
	}
	paths := make([]string, 0, len(change.files))
	for _, file := range change.files {
		paths = append(paths, fmt.Sprintf("%q", file.Path))
	}
	open := true
	result := ui.AlertDialog(c, &open, action+" selected changes?", effect+"\n\n"+strings.Join(paths, "\n"), "Cancel", action)
	if !open || result == 1 {
		a.indexConfirmation = nil
	}
	if result == 1 && a.repositoryWrite == "" && a.Model.Active().Repository.Root == change.root {
		a.applyIndexChange(change)
	}
}

func (a *Workspace) applyIndexChange(change *indexChange) {
	verb, done := "Unstaging", "Unstaged"
	if change.stage {
		verb, done = "Staging", "Staged"
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.beginRepositoryWrite(change.root, fmt.Sprintf("%s %d file(s)…", verb, len(change.files)), cancel)
	a.UpdateRepositoryIndex(ctx, change.repo, change.stage, change.files, func(repo domain.Repository, writeErr, refreshErr error) {
		message := fmt.Sprintf("%s %d file(s)", done, len(change.files))
		if writeErr != nil {
			message = verb + " failed · " + writeErr.Error()
			if errors.Is(writeErr, context.Canceled) || errors.Is(writeErr, context.DeadlineExceeded) {
				message = "Git did not confirm the index operation · " + writeErr.Error()
			}
		}
		a.finishRepositoryWrite(change.root, repo, refreshErr, message)
	})
}
