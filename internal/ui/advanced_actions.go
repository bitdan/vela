package ui

import (
	"fmt"
	"slices"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const dropStash commandID = "git.drop-stash"

func stashLabel(e domain.StashEntry) string {
	return fmt.Sprintf("%s · %s · %.8s", e.Selector, e.Subject, e.OID)
}

func (s *repositoryOperationDialog) configureStash(repo domain.Repository, id commandID, target commandTarget) {
	switch id {
	case stashChanges:
		s.label = "Stash Changes"
		s.detail = "Save Index and working changes. Ignored files are kept. Mock Shelf is separate."
		s.op.Kind = domain.SaveStash
	case unstashChanges, dropStash:
		s.label = "Apply Stash"
		s.detail = "Apply retains the stash. Drop is a separate confirmed operation."
		s.op.Kind = domain.ApplyStash
		if id == dropStash {
			s.label = "Drop Stash"
			s.detail = "Delete this stash entry permanently. Working files are kept."
			s.op.Kind = domain.DropStash
		}
		for _, e := range repo.Stashes {
			s.choices = append(s.choices, stashLabel(e))
			if s.choice == "" || e.OID == target.source || e.Selector == target.source {
				s.choice = stashLabel(e)
			}
		}
	}
}

func (s *repositoryOperationDialog) selectedStash() (domain.StashEntry, bool) {
	i := slices.Index(s.choices, s.choice)
	if i < 0 || i >= len(s.snapshot.Stashes) {
		return domain.StashEntry{}, false
	}
	return s.snapshot.Stashes[i], true
}

func (a *Workspace) stashFields(c *ui.Context, s *repositoryOperationDialog, busy bool) {
	switch s.op.Kind {
	case domain.SaveStash:
		ui.TextInput(c, &s.op.Message).Label("Stash message").Placeholder("Stash message").FillWidth().AutoFocus().Disabled(busy)
		ui.Checkbox(c, &s.op.IncludeUntracked, "Include untracked files").Disabled(busy)
	case domain.ApplyStash, domain.DropStash:
		if len(s.choices) == 0 {
			ui.Text(c, "No stashes")
			return
		}
		ui.Select(c, &s.choice, s.choices).Label("Stash entry").FillWidth().Disabled(busy)
		if entry, ok := s.selectedStash(); ok {
			ui.Text(c, entry.OID).FillWidth()
		}
		if s.op.Kind == domain.ApplyStash {
			ui.Checkbox(c, &s.op.RestoreIndex, "Restore staged state").Disabled(busy)
			ui.Button(c, "Drop Selected Stash…").Disabled(busy).OnClick(func() {
				entry, ok := s.selectedStash()
				if !ok {
					return
				}
				a.repositoryOperation = nil
				a.run(dropStash, commandTarget{source: entry.Selector})
			})
		}
	}
}

func resetImpact(mode string) string {
	switch mode {
	case "Soft":
		return "Move HEAD only. Keep Index and working files. Commits after the target leave this branch."
	case "Hard":
		return "Move HEAD and replace Index and tracked working files. Uncommitted tracked changes will be lost. Commits after the target leave this branch."
	default:
		return "Move HEAD and reset Index. Keep working files; staged changes become unstaged. Commits after the target leave this branch."
	}
}
