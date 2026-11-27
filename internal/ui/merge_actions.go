package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const chooseMergeSource = "Select a branch or tag"

func findMergeConflict(repo domain.Repository, path string) (domain.MergeConflict, bool) {
	for _, conflict := range repo.Conflicts {
		if conflict.Path == path {
			return conflict, true
		}
	}
	return domain.MergeConflict{}, false
}

func (s *repositoryOperationDialog) configureMerge(repo domain.Repository, id commandID, target commandTarget) {
	s.label = commandLabel(id)
	switch id {
	case mergeBranch:
		s.label = "Merge"
		s.detail = "Merge the selected source into " + repo.CurrentBranch + "."
		s.op.Kind = domain.StartMerge
		s.mergeSource = target.source
		if s.mergeSource == "" {
			s.mergeSource = chooseMergeSource
		}
	case continueMerge:
		s.op.Kind = domain.ContinueMerge
		s.detail = "Finish the merge on " + repo.CurrentBranch + " using the staged files and Git's merge message."
	case abortMerge:
		s.op.Kind = domain.AbortMerge
		s.detail = "Abort the merge on " + repo.CurrentBranch + " and restore the pre-merge state. Changes made while resolving conflicts may be lost."
	default:
		conflict, _ := findMergeConflict(repo, target.file.Path)
		s.op.Source = conflict.Path
		switch id {
		case acceptCurrent:
			s.op.Kind = domain.AcceptCurrent
			s.label = conflictActionLabel(repo, id)
			side, _ := conflictPaneLabels(repo.Rebase, repo.Sequencer)
			side = strings.ToLower(side)
			s.detail = conflictSideDetail(conflict.Path, side, conflict.Current == "")
		case acceptIncoming:
			s.op.Kind = domain.AcceptIncoming
			s.label = conflictActionLabel(repo, id)
			_, side := conflictPaneLabels(repo.Rebase, repo.Sequencer)
			side = strings.ToLower(side)
			s.detail = conflictSideDetail(conflict.Path, side, conflict.Incoming == "")
		case markResolved:
			s.op.Kind = domain.MarkResolved
			s.detail = fmt.Sprintf("Stage the working tree version or deletion of %q as the resolution.", conflict.Path)
		}
	}
}

func conflictActionLabel(repo domain.Repository, id commandID) string {
	if repo.Rebase.Active() || repo.Sequencer.Active() {
		left, right := conflictPaneLabels(repo.Rebase, repo.Sequencer)
		if id == acceptCurrent {
			return "Accept " + left
		}
		if id == acceptIncoming {
			return "Accept " + right
		}
	}
	return commandLabel(id)
}

func (s *repositoryOperationDialog) configureRebase(repo domain.Repository, id commandID, target commandTarget) {
	s.label = commandLabel(id)
	commit := repo.Rebase.Stopped[:min(8, len(repo.Rebase.Stopped))]
	if commit == "" {
		commit = "the current commit"
	}
	switch id {
	case rebaseBranch:
		s.label, s.op.Kind = "Rebase", domain.StartRebase
		s.detail = "Replay " + repo.CurrentBranch + " onto the selected branch or tag. Replayed commits may receive new IDs."
		s.mergeSource = target.source
		if s.mergeSource == "" {
			s.mergeSource = chooseMergeSource
		}
	case continueRebase:
		s.op.Kind = domain.ContinueRebase
		s.detail = "Continue rebasing " + repo.Rebase.Branch + " with the staged resolution."
	case skipRebase:
		s.op.Kind = domain.SkipRebase
		s.detail = "Skip " + commit + " while rebasing " + repo.Rebase.Branch + ". Its changes and any resolution for this step will be discarded."
	case abortRebase:
		s.op.Kind = domain.AbortRebase
		s.detail = "Restore " + repo.Rebase.Branch + " to its pre-rebase tip. Any changes made while resolving this rebase may be lost."
	}
}

func conflictSideDetail(path, side string, deleted bool) string {
	if deleted {
		return fmt.Sprintf("Delete %q as resolved by the %s side. Any working edits to this file will be lost.", path, side)
	}
	return fmt.Sprintf("Replace %q with the %s version and stage it. Any working edits to this file will be lost.", path, side)
}

func (a *Workspace) mergeControls(c *ui.Context, p tokens) {
	s := a.Model.Active()
	repo := s.Repository
	title := "Merge in progress"
	actions := []commandID{continueMerge, abortMerge}
	if repo.Rebase.Active() {
		title = "Rebasing " + repo.Rebase.Branch
		actions = []commandID{continueRebase, skipRebase, abortRebase}
	} else if repo.Sequencer.Active() {
		title = sequenceTitle(repo.Sequencer) + " in progress"
		if repo.Sequencer.Remaining > 1 {
			title += fmt.Sprintf(" · %d commits remaining", repo.Sequencer.Remaining)
		}
		actions = []commandID{continueSequence, skipSequence, abortSequence}
	} else if len(repo.MergeHeads) == 0 {
		title = "Unmerged files · resolve and stage, then commit"
		actions = nil
	}
	ui.Column(c).FillWidth().Padding(gap, inset).Gap(gap).Children(func() {
		ui.Text(c, title+" · "+fileCount(len(repo.Conflicts))+" unresolved").Bold()
		ui.Row(c).Gap(gap).Wrap().Children(func() {
			for _, id := range actions {
				a.button(c, id, sequenceActionLabel(repo, id))
			}
		})
		if i := slices.IndexFunc(repo.Conflicts, func(conflict domain.MergeConflict) bool { return conflict.Path == s.Working }); i >= 0 {
			target := commandTarget{source: "working", file: domain.File{Path: s.Working, Status: "U"}}
			ui.Row(c).Gap(gap).Wrap().Children(func() {
				for _, id := range []commandID{acceptCurrent, acceptIncoming, markResolved} {
					label := conflictActionLabel(repo, id)
					reason := a.disabledFor(id, target)
					ui.Button(c.Key(string(id)), label).Disabled(reason != "").Tooltip(reason).OnClick(func() { a.run(id, target) })
				}
			})
		}
	})
	divider(c, p)
}
