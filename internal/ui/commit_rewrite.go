package ui

import (
	"slices"
	"vela/internal/domain"
)

const (
	confirmHistoryRewrite commandID = "git.confirm-history-rewrite"
	cancelHistoryRewrite  commandID = "git.cancel-history-rewrite"
	refreshHistoryRewrite commandID = "git.preview-history-rewrite"
)

func (s *squashDialog) operationKind() domain.RepositoryOperationKind {
	if s.kind == 0 {
		return domain.SquashCommits
	}
	return s.kind
}
func (s *squashDialog) title() string {
	if s.operationKind() == domain.RewordCommit {
		return "Edit Commit Message"
	}
	return "Squash Commits"
}
func (a *Workspace) openCommitRewrite(id commandID, t commandTarget) {
	commit, ok := a.operationCommit(t)
	if !ok {
		return
	}
	a.squash = &squashDialog{kind: domain.RewordCommit, selection: []string{commit.Hash}}
	a.previewSquash()
}
func rewriteEntryCommand(id commandID) bool { return id == editCommit }
func (a *Workspace) rewriteEntryDisabled(id commandID, t commandTarget) string {
	if a.PreviewCommitRewrite == nil {
		return "Commit rewrite preview is unavailable"
	}
	if a.squash != nil || a.repositoryOperation != nil || a.patchDialog != nil || a.patchImport != nil || a.commitConfirmation != nil {
		return "Close the current repository dialog"
	}
	if reason := a.squashBaseReason(); reason != "" {
		return reason
	}
	commit, ok := a.operationCommit(t)
	if !ok || !fixedObjectID(commit.Hash) {
		return "Select one loaded commit"
	}
	ids := slices.Sorted(a.repo().logChosen.All())
	if len(ids) > 1 {
		return "Select exactly one commit"
	}
	return ""
}
