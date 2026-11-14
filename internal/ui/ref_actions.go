package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

func localBranchRef(repo domain.Repository, source string) string {
	if source == "" {
		source = repo.CurrentBranch
	}
	if !strings.HasPrefix(source, "refs/heads/") {
		source = "refs/heads/" + source
	}
	if repo.Refs[source] == "" {
		return ""
	}
	return source
}

func (s *repositoryOperationDialog) configureRef(repo domain.Repository, id commandID, target commandTarget) {
	switch id {
	case newTag:
		s.label = "Create Tag"
		s.op = domain.RepositoryOperation{Kind: domain.CreateTag, Source: operationRef(repo, target.source)}
		s.detail = "Create a local lightweight tag at " + s.op.Source + ". Existing tags are kept. This does not push or sign the tag."
	case renameBranch:
		s.label = "Rename Branch"
		s.op = domain.RepositoryOperation{Kind: domain.RenameBranch, Source: localBranchRef(repo, target.source)}
		s.detail = "Rename local branch " + s.op.Source + ". Its commits and working files are kept; the remote branch is unchanged."
	case deleteBranch:
		s.label = "Delete Branch"
		s.op = domain.RepositoryOperation{Kind: domain.DeleteBranch, Source: localBranchRef(repo, target.source)}
		s.detail = "Delete local branch " + s.op.Source + " at " + repo.Refs[s.op.Source] + ". Only branches fully merged into current HEAD can be deleted. Working files and the remote branch are kept. No force deletion."
	}
}

func refFields(c *ui.Context, s *repositoryOperationDialog, busy bool) {
	switch s.op.Kind {
	case domain.RenameBranch:
		ui.Text(c, "New local branch name")
		ui.TextInput(c, &s.op.Branch).Label("Local branch name").FillWidth().AutoFocus().Disabled(busy)
	case domain.CreateTag:
		ui.Text(c, "Tag name")
		ui.TextInput(c, &s.op.Target).Label("Tag name").FillWidth().AutoFocus().Disabled(busy)
	}
}

func refFieldsDisabled(s *repositoryOperationDialog) bool {
	if s.op.Kind == domain.CreateTag {
		name := strings.TrimSpace(s.op.Target)
		return name == "" || strings.HasPrefix(name, "-") || s.snapshot.Refs["refs/tags/"+name] != ""
	}
	if s.op.Kind == domain.RenameBranch {
		name := strings.TrimSpace(s.op.Branch)
		return name == "" || strings.HasPrefix(name, "-") || s.snapshot.Refs["refs/heads/"+name] != ""
	}
	return false
}
