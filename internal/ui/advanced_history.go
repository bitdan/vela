package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

func isAdvancedHistoryCommand(id commandID) bool {
	switch id {
	case checkoutRevision, showRepositoryRevision, getRevision, cherryPickChanges, revertChanges, openRevisionEntry, restoreRevisionEntry, revisionParent, reloadRevisionTree, closeRevisionTree:
		return true
	}
	return false
}

func isHistoryWrite(kind domain.RepositoryOperationKind) bool {
	return kind == domain.CheckoutRevision || kind == domain.RestoreRevisionFiles || kind == domain.CherryPickSelected || kind == domain.RevertSelected
}

func (a *Workspace) historicalSelection(t commandTarget) (domain.Commit, []domain.File) {
	versionOnly := strings.HasPrefix(t.source, "revision:")
	t.source = strings.TrimPrefix(t.source, "revision:")
	commit, ok := a.operationCommit(t)
	if !ok && fixedObjectID(t.source) {
		commit.Hash = t.source
	}
	if !fixedObjectID(commit.Hash) {
		return domain.Commit{}, nil
	}
	if t.file.Path == "" {
		return commit, nil
	}
	result := []domain.File{t.file}
	r := a.repo()
	if !versionOnly && t.source == r.historicalSelectionRevision && r.historicalChosen.Has(t.file.Path) {
		result = nil
		for _, file := range commit.Files {
			if r.historicalChosen.Has(file.Path) {
				result = append(result, file)
			}
		}
	}
	return commit, result
}

func (a *Workspace) historyDirtyReason(kind domain.RepositoryOperationKind, paths []string) string {
	if kind == domain.CheckoutRevision || kind == domain.CherryPickSelected || kind == domain.RevertSelected {
		r := a.repo()
		for i := range r.tabs {
			if r.tabs[i].source == "project" && r.tabs[i].code != nil && r.tabs[i].code.Buffer.Modified() {
				return "Save or discard unsaved source buffers before changing historical files"
			}
		}
		if r.split != nil && r.split.source == "project" && r.split.code != nil && r.split.code.Buffer.Modified() {
			return "Save or discard unsaved source buffers before changing historical files"
		}
	}
	for _, path := range paths {
		if a.hasDirtySource(path) {
			return "Save or discard the unsaved source buffer first"
		}
	}
	return ""
}

func (a *Workspace) advancedHistoryDisabled(id commandID, t commandTarget) string {
	if id == closeRevisionTree {
		if a.revisionBrowser == nil {
			return "No historical repository is open"
		}
		return ""
	}
	if id == openRevisionEntry || id == restoreRevisionEntry || id == revisionParent || id == reloadRevisionTree {
		return a.revisionBrowserDisabled(id)
	}
	r := a.Model.Active().Repository
	if r.Root == "" {
		return "Open a local Git repository"
	}
	if a.repositoryWrite != "" || a.repositoryRead != nil || a.repositoryOperation != nil || a.indexConfirmation != nil || a.commitConfirmation != nil {
		return "Wait for the current repository operation"
	}
	commit, files := a.historicalSelection(t)
	if commit.Hash == "" {
		return "Select an immutable commit"
	}
	if id == showRepositoryRevision {
		if a.ReadRepositoryRevisionTree == nil {
			return "Historical tree reader is unavailable"
		}
		return ""
	}
	if a.OperateRepository == nil || r.IndexVersion == "" {
		return "Refresh the repository and connect Git operations"
	}
	if r.ResolvingConflicts() || r.Bisect.Active() {
		return "Finish the current Git operation first"
	}
	if len(r.Staged) > 0 || slices.ContainsFunc(r.Working, func(f domain.File) bool { return f.Status != "?" }) {
		return "Commit or shelve tracked changes before continuing"
	}
	kind := domain.CheckoutRevision
	if id != checkoutRevision {
		mergePicker := commit.Merge && (id == cherryPickChanges || id == revertChanges)
		if len(files) == 0 && !mergePicker || len(files) > 100 {
			return "Select 1 to 100 whole files"
		}
		kind = domain.RestoreRevisionFiles
		if id == cherryPickChanges || id == revertChanges {
			if commit.Merge && a.ReadRepositoryRevisionChanges == nil {
				return "Connect the merge comparison reader"
			}
			kind = domain.CherryPickSelected
			if r.CurrentBranch == "Detached HEAD" {
				return "Check out a local branch before applying selected changes"
			}
			if len(commit.Files) == 0 && !commit.Merge {
				return "Select changed files in the loaded Git Log"
			}
		}
	}
	var paths []string
	for _, file := range files {
		if id == getRevision && file.Status == "D" {
			return "This file is absent at this commit; browse an earlier revision"
		}
		paths = append(paths, file.Path)
		if kind == domain.CherryPickSelected && file.OldPath != "" {
			paths = append(paths, file.OldPath)
		}
	}
	return a.historyDirtyReason(kind, paths)
}

func (a *Workspace) runAdvancedHistory(id commandID, t commandTarget) {
	switch id {
	case closeRevisionTree:
		a.closeRevisionBrowser()
		return
	case openRevisionEntry, restoreRevisionEntry, revisionParent, reloadRevisionTree:
		a.runRevisionBrowser(id)
		return
	case showRepositoryRevision:
		commit, _ := a.historicalSelection(t)
		a.openRevisionBrowser(commit.Hash)
		return
	}
	commit, files := a.historicalSelection(t)
	repo := a.Model.Active().Repository
	s := &repositoryOperationDialog{open: true, root: repo.Root, snapshot: repo, label: commandLabel(id)}
	s.op.Source = commit.Hash
	switch id {
	case checkoutRevision:
		s.op.Kind = domain.CheckoutRevision
		s.detail = fmt.Sprintf("Check out %s as Detached HEAD.\nCurrent branch: %s\nNo branch is moved. Create or check out a branch before committing; return through the branch menu.", commit.Hash, repo.CurrentBranch)
	case getRevision:
		s.op.Kind = domain.RestoreRevisionFiles
		s.detail = "Replace the selected working files with their complete versions at " + commit.Hash + ".\nHEAD and Index stay unchanged. Review and stage the restored files separately."
	case cherryPickChanges, revertChanges:
		sequence := cherryPick
		s.op.Kind = domain.CherryPickSelected
		if id == revertChanges {
			sequence = revertCommit
			s.op.Kind = domain.RevertSelected
		}
		kind := s.op.Kind
		s.configureSequence(repo, sequence, commit)
		s.op.Kind, s.label = kind, commandLabel(id)
		s.detail += "\nOnly the selected whole-file changes create a new commit. Unselected changes are excluded. Conflicts support Continue, Skip and Abort."
	}
	for _, file := range files {
		if !slices.Contains(s.op.Paths, file.Path) {
			s.op.Paths = append(s.op.Paths, file.Path)
		}
		if id != getRevision && file.OldPath != "" && !slices.Contains(s.op.Paths, file.OldPath) {
			s.op.Paths = append(s.op.Paths, file.OldPath)
		}
	}
	if commit.Merge && (id == cherryPickChanges || id == revertChanges) {
		s.historyChanges = &historyChangeSelection{seed: slices.Clone(s.op.Paths), selected: -1}
		s.op.Paths = nil
	}
	s.detail += "\nRepository: " + repo.Root
	a.closeRevisionBrowser()
	a.repositoryOperation = s
}

func (a *Workspace) historyOperationFields(c *ui.Context, s *repositoryOperationDialog) {
	if s.historyChanges != nil {
		a.historyComparisonFields(c, s)
		return
	}
	if !isHistoryWrite(s.op.Kind) || len(s.op.Paths) == 0 {
		return
	}
	ui.Text(c, fmt.Sprintf("%d selected paths", len(s.op.Paths)))
	ui.Scroll(c).FillWidth().MaxHeight(100).Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			for _, path := range s.op.Paths {
				ui.Text(c, path).SingleLine().Ellipsis("…").FillWidth()
			}
		})
	})
}

func (a *Workspace) refreshHistoricalWriteViews(root string, op domain.RepositoryOperation) {
	paths := slices.Clone(op.Paths)
	for i, session := range a.Model.Sessions {
		if session.Repository.Root != root {
			continue
		}
		if op.Kind != domain.RestoreRevisionFiles {
			for _, entry := range a.repos[i].tabs {
				if !slices.Contains(paths, entry.path) {
					paths = append(paths, entry.path)
				}
			}
			if split := a.repos[i].split; split != nil && !slices.Contains(paths, split.path) {
				paths = append(paths, split.path)
			}
			if preview := a.repos[i].preview; preview != nil && !slices.Contains(paths, preview.path) {
				paths = append(paths, preview.path)
			}
		}
	}
	for _, path := range paths {
		a.refreshOpenFileViews(root, path)
	}
}
