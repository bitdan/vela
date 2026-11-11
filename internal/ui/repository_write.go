package ui

import (
	"errors"

	"vela/internal/domain"
)

func (a *Workspace) beginRepositoryWrite(root, message string) {
	a.repositoryWrite = root
	stopRead(&a.repositoryFileRead)
	a.status = message
}

func (a *Workspace) finishRepositoryWrite(root string, repo domain.Repository, refreshErr error, message string) {
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
