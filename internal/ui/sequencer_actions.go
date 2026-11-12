package ui

import (
	"fmt"
	"slices"

	"vela/internal/domain"
)

func sequenceTitle(state domain.SequencerState) string {
	if state.Kind == "revert" {
		return "Revert"
	}
	return "Cherry-Pick"
}

func sequenceActionLabel(repo domain.Repository, id commandID) string {
	switch id {
	case continueSequence:
		return "Continue " + sequenceTitle(repo.Sequencer)
	case abortSequence:
		return "Abort " + sequenceTitle(repo.Sequencer)
	default:
		return commandLabel(id)
	}
}

func (a *Workspace) operationCommit(target commandTarget) (domain.Commit, bool) {
	history := a.Model.Active().Repository.History
	index := a.Model.Active().Commit
	if target.source != "" {
		index = slices.IndexFunc(history, func(c domain.Commit) bool { return c.Hash == target.source })
	} else if selected := a.repo().log.Selected; selected != nil && *selected < 0 {
		return domain.Commit{}, false
	}
	if index < 0 || index >= len(history) {
		return domain.Commit{}, false
	}
	return history[index], true
}

func (s *repositoryOperationDialog) configureSequence(repo domain.Repository, id commandID, commit domain.Commit) {
	s.label = sequenceActionLabel(repo, id)
	switch id {
	case cherryPick, revertCommit:
		s.op.Kind, s.op.Source = domain.StartCherryPick, commit.Hash
		s.label = "Cherry-Pick"
		impact := "Apply this commit as a new commit on "
		if id == revertCommit {
			s.op.Kind, s.label = domain.StartRevert, "Revert"
			impact = "Create a new commit reversing this commit on "
		}
		s.detail = fmt.Sprintf("%s%s.\nCommit: %s\n%s", impact, repo.CurrentBranch, commit.Hash, commit.Subject)
		if commit.Merge {
			s.mainline = "Select a parent"
			s.mainlines = []string{s.mainline}
			for i, parent := range commit.ParentHashes {
				s.mainlines = append(s.mainlines, fmt.Sprintf("%d · %s", i+1, parent))
			}
			s.detail += "\nChanges are calculated relative to the selected mainline parent."
			if id == revertCommit {
				s.detail += " Reverting a merge also affects which changes future merges can reintroduce."
			}
		}
	case continueSequence:
		s.op.Kind = domain.ContinueSequence
		s.detail = "Continue " + sequenceTitle(repo.Sequencer) + " on " + repo.CurrentBranch + " with the staged resolution. An empty result must be skipped."
	case skipSequence:
		s.op.Kind = domain.SkipSequence
		s.detail = "Skip " + repo.Sequencer.Commit + ". This step and its conflict resolution will be discarded."
	case abortSequence:
		s.op.Kind = domain.AbortSequence
		s.detail = "Abort " + sequenceTitle(repo.Sequencer) + " and restore the state before the operation. Changes made while resolving conflicts may be lost."
	}
}

func conflictPaneLabels(rebase domain.RebaseState, sequence domain.SequencerState) (string, string) {
	if rebase.Active() {
		return "Rebased", "Replayed Commit"
	}
	if sequence.Kind == "cherry-pick" {
		return "Current", "Picked Commit"
	}
	if sequence.Kind == "revert" {
		return "Current", "Reverted Parent"
	}
	return "Current", "Incoming"
}
