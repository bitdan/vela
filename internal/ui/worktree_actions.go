package ui

import (
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

// Retain cached tabs and drafts, but prevent writes through a removed checkout.
func (a *Workspace) invalidateRemovedWorktree(path string) {
	for i := range a.Model.Sessions {
		root := a.Model.Sessions[i].Repository.Root
		equal := filepath.Clean(root) == filepath.Clean(path)
		if runtime.GOOS == "windows" {
			equal = strings.EqualFold(filepath.Clean(root), filepath.Clean(path))
		}
		if equal {
			a.Model.Sessions[i].Repository.IndexVersion = ""
			a.repos[i].addConsole("Worktree removed. Cached tabs and drafts are retained; refresh before further Git operations.")
		}
	}
}

const (
	removeWorktree commandID = "git.remove-worktree"
	openWorktree   commandID = "git.open-worktree"
)

func (s *repositoryOperationDialog) configureWorktree(repo domain.Repository, id commandID, target commandTarget) {
	if id == newWorktree {
		s.label = "New Worktree"
		s.detail = "Create a separate checkout at a new absolute directory. Choose an existing local branch, create a new branch, or use a detached commit."
		s.op = domain.RepositoryOperation{Kind: domain.CreateWorktree, Source: repo.Head, Mode: "Detached"}
		if ref := operationRef(repo, target.source); strings.HasPrefix(ref, "refs/heads/") {
			s.op.Source = ref
			s.op.Mode = "Branch"
		}
		return
	}
	s.label = "Remove Worktree"
	s.op.Kind = domain.RemoveWorktree
	s.management = id == showWorktrees
	s.detail = "Remove this clean linked worktree directory. Its branch is kept. Dirty or locked worktrees are refused."
	if s.management {
		s.detail = "Open a checkout or select another linked worktree to remove. Removal requires a separate confirmation."
	}
	for _, entry := range repo.Worktrees {
		s.choices = append(s.choices, entry.Path)
		if s.choice == "" || entry.Path == target.source {
			s.choice = entry.Path
		}
	}
}

func (s *repositoryOperationDialog) selectedWorktree() (domain.Worktree, int) {
	i := slices.Index(s.choices, s.choice)
	if i < 0 || i >= len(s.snapshot.Worktrees) {
		return domain.Worktree{}, -1
	}
	return s.snapshot.Worktrees[i], i
}

func (a *Workspace) worktreeFields(c *ui.Context, s *repositoryOperationDialog, busy bool) {
	switch s.op.Kind {
	case domain.CreateWorktree:
		ui.TextInput(c, &s.op.Path).Label("New worktree directory").Placeholder("Absolute path to a new directory").FillWidth().AutoFocus().Disabled(busy)
		if s.op.BisectVersion != "" {
			ui.Text(c, "Detached candidate checkout")
			ui.Text(c, s.op.Source).FillWidth()
			return
		}
		ui.Select(c, &s.op.Mode, []string{"Detached", "Branch", "New Branch"}).Label("Worktree mode").Disabled(busy)
		if s.op.Mode == "New Branch" {
			ui.TextInput(c, &s.op.Branch).Label("New worktree branch").Placeholder("New local branch name").FillWidth().Disabled(busy)
		}
		ui.TextInput(c, &s.op.Source).Label("Worktree source").FillWidth().Disabled(busy)
	case domain.RemoveWorktree:
		if s.management {
			ui.Select(c, &s.choice, s.choices).Label("Worktree path").FillWidth().Disabled(busy)
		} else {
			ui.Text(c, s.choice).FillWidth()
		}
		entry, _ := s.selectedWorktree()
		ui.Text(c, entry.Head).FillWidth()
		ui.Text(c, entry.Branch).FillWidth()
		if entry.Locked {
			ui.Text(c, "Locked")
		}
		if entry.Prunable {
			ui.Text(c, "Missing / prunable")
		}
		if s.management {
			ui.Row(c).Gap(gap).Children(func() {
				ui.Button(c, "Open Worktree").Disabled(busy || a.ReadRepository == nil || entry.Path == "" || entry.Prunable || entry.Bare).OnClick(func() { a.repositoryOperation = nil; a.run(openWorktree, commandTarget{source: entry.Path}) })
				ui.Button(c, "New Worktree…").Disabled(busy).OnClick(func() { a.repositoryOperation = nil; a.run(newWorktree, commandTarget{}) })
			})
		}
	}
}

func worktreeCommandDisabled(s *repositoryOperationDialog) bool {
	if s.op.Kind == domain.CreateWorktree {
		return strings.TrimSpace(s.op.Path) == "" || strings.TrimSpace(s.op.Source) == "" ||
			(s.op.Mode == "New Branch" && strings.TrimSpace(s.op.Branch) == "")
	}
	if s.op.Kind == domain.RemoveWorktree {
		entry, index := s.selectedWorktree()
		return index <= 0 || entry.Path == s.root || entry.Locked || entry.Prunable || entry.Bare
	}
	return false
}
