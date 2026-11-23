package ui

import "vela/internal/domain"

func (a *Workspace) protectedCommandReason(id commandID, target commandTarget) string {
	repo := a.Model.Active().Repository
	if repo.Root == "" {
		return ""
	}
	var op domain.RepositoryOperation
	switch id {
	case rebaseBranch:
		op.Kind = domain.StartRebase
	case resetRevision:
		op.Kind = domain.ResetRevision
		if commit, ok := a.operationCommit(target); ok {
			op.Source = commit.Hash
		}
		op.Mode = "Mixed"
	case renameBranch:
		op.Kind, op.Source = domain.RenameBranch, localBranchRef(repo, target.source)
	case deleteBranch:
		op.Kind, op.Source = domain.DeleteBranch, localBranchRef(repo, target.source)
	default:
		return ""
	}
	if err := a.Layout.VersionControl.Git.SafetyPolicy().CheckOperation(repo, op); err != nil {
		return err.Error()
	}
	return ""
}
