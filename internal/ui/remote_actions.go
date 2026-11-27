package ui

import (
	"fmt"
	"slices"
	"vela/internal/domain"
)

const (
	addRemote      commandID = "git.add-remote"
	editRemote     commandID = "git.edit-remote"
	removeRemote   commandID = "git.remove-remote"
	renameRemote   commandID = "git.rename-remote"
	editRemoteURLs commandID = "git.edit-remote-urls"
)

func (s *repositoryOperationDialog) configureRemote(id commandID, target commandTarget) {
	s.op.Remote, s.op.Target = target.remote.Name, target.remote.URL
	switch id {
	case addRemote:
		s.label = "Add Remote"
		s.detail = "Add remote " + target.remote.Name + " at " + target.remote.URL + ". This does not fetch or push."
		s.op.Kind = domain.AddRemote
	case editRemote:
		s.label = "Edit Remote"
		s.detail = "Change the fetch URL of " + target.source + " to " + target.remote.URL + ". Explicit push URLs are kept. This does not fetch or push."
		s.op.Kind, s.op.Remote = domain.EditRemote, target.source
	case removeRemote:
		s.label = "Remove Remote"
		s.detail = "Remove remote " + target.source + ", its remote-tracking refs and tracking configuration. Local branches, commits and working files are kept. The remote server is unchanged."
		s.op.Kind, s.op.Remote = domain.RemoveRemote, target.source
	case renameRemote:
		s.label = "Rename Remote"
		s.detail = "Rename " + target.source + " to " + target.remote.Name + ". Git updates tracking references and local branch upstreams."
		s.op.Kind, s.op.Remote, s.op.Target = domain.RenameRemote, target.source, target.remote.Name
	case editRemoteURLs:
		s.label = "Edit Remote URLs"
		s.detail = fmt.Sprintf("Save %d fetch URL(s) and %d explicit push URL(s) for %s, in the order entered. Empty push list uses Git's fetch URL fallback.", len(target.remote.FetchURLs), len(target.remote.PushURLs), target.source)
		s.op = domain.RepositoryOperation{Kind: domain.EditRemoteURLs, Remote: target.source, FetchURLs: slices.Clone(target.remote.FetchURLs), PushURLs: slices.Clone(target.remote.PushURLs)}
	}
}

func (a *Workspace) confirmRemote(id commandID, original string, item domain.Remote) {
	if a.Model.Index != a.remotes.repository {
		return
	}
	if repo := a.Model.Active().Repository; repo.RemoteManagementVersion != a.remotes.version {
		a.remotes.open = false
		a.status = "Remote settings changed; reopen Git Remotes"
		return
	}
	a.remotes.open = false
	a.run(id, commandTarget{source: original, remote: item})
}
