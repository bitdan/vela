package ui

import (
	"slices"
	"vela/internal/application"
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
	if s.operationKind() == domain.FixupCommits {
		return "Fixup Selected Commits"
	}
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
	kind, ids := domain.RewordCommit, []string{commit.Hash}
	if id == fixupCommit {
		kind, ids = domain.FixupCommits, slices.Sorted(a.repo().logChosen.All())
	}
	a.squash = &squashDialog{kind: kind, selection: ids}
	a.previewSquash()
}
func rewriteEntryCommand(id commandID) bool { return id == editCommit || id == fixupCommit }
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
	if id == fixupCommit {
		if t.source != "" && !a.repo().logChosen.Has(t.source) {
			return "Select the continuous commit range in Log first"
		}
		if err := application.ValidateSquashSelection(ids); err != nil {
			return err.Error()
		}
		return ""
	}
	if len(ids) > 1 {
		return "Select exactly one commit"
	}
	return ""
}
