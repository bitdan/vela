package ui

import (
	"context"
	"errors"

	"vela/internal/domain"
)

func (a *Workspace) beginRepositoryWrite(root, message string, cancel context.CancelFunc) {
	a.stopBackgroundTasks()
	a.closeProjectSearchDialog()
	a.closeProjectFileDialog()
	a.closeBranchComparisonDialog()
	a.stopComparisonTabReads()
	stopRead(&a.logRead)
	a.stopSourceRefreshes()
	a.closeFileHistoryDialog()
	a.stopPatchExports()
	a.stopSourceDecision()
	a.repositoryWriteCancel = cancel
	a.repositoryCancelRequested = false
	stopRead(&a.sessionRestore)
	a.repositoryWrite = root
	stopRead(&a.repositoryFileRead)
	a.status = message
}

func (a *Workspace) finishRepositoryWrite(root string, repo domain.Repository, refreshErr error, message string) {
	if a.repositoryWriteCancel != nil {
		a.repositoryWriteCancel()
	}
	a.repositoryWriteCancel = nil
	a.repositoryCancelRequested = false
	a.repositoryWrite = ""
	active := a.Model.Active().Repository.Root == root
	if refreshErr == nil && repo.Root != root {
		refreshErr = errors.New("the repository root changed")
	}
	if refreshErr != nil {
		message += " · Could not refresh: " + refreshErr.Error()
	} else {
		if active {
			stopRead(&a.repositoryFileRead)
		}
		a.installRepository(repo)
	}
	for i := range a.Model.Sessions {
		if a.Model.Sessions[i].Repository.Root == root {
			if refreshErr != nil {
				a.Model.Sessions[i].Repository.IndexVersion = ""
			}
			a.repos[i].addConsole(message)
			break
		}
	}
	if active {
		a.status = message
	}
}
