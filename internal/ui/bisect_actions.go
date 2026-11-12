package ui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
)

const (
	startBisect          commandID = "git.start-bisect"
	bisectGood           commandID = "git.bisect-good"
	bisectBad            commandID = "git.bisect-bad"
	bisectSkip           commandID = "git.bisect-skip"
	resetBisect          commandID = "git.reset-bisect"
	showBisectCandidate  commandID = "git.show-bisect-candidate"
	createBisectWorktree commandID = "git.bisect-create-worktree"
)

func isBisectCommand(id commandID) bool {
	return id == startBisect || id == bisectGood || id == bisectBad || id == bisectSkip || id == resetBisect || id == createBisectWorktree
}

func bisectDisabled(repo domain.Repository, id commandID) string {
	if id == startBisect {
		if repo.Bisect.Active() {
			return "End the active Bisect first"
		}
		if repo.Head == "" || repo.ResolvingConflicts() || len(repo.Conflicts) > 0 {
			return "Select a repository with commits and finish pending operations"
		}
		return ""
	}
	if !repo.Bisect.Active() {
		return "No Bisect is in progress"
	}
	if id == resetBisect {
		return ""
	}
	if !repo.Bisect.Supported {
		return "Classify this external checkout or custom-term Bisect using Git"
	}
	if id == createBisectWorktree {
		if !repo.Bisect.NoCheckout || repo.Bisect.Candidate == "" || repo.Bisect.Ambiguous {
			return "Select an exact candidate in a supported no-checkout search"
		}
		return ""
	}
	if repo.Bisect.FirstBad != "" || repo.Bisect.Ambiguous {
		return "Search finished. End Bisect before starting another search"
	}
	return ""
}

func (s *repositoryOperationDialog) configureBisect(repo domain.Repository, id commandID) {
	s.op.Source = repo.Bisect.Candidate
	switch id {
	case createBisectWorktree:
		s.label = "Create Candidate Worktree"
		s.detail = "Create a separate checkout of this exact candidate for testing. The search checkout stays in place. Open the result from Worktrees, then return to this repository to classify the candidate."
		s.op = domain.RepositoryOperation{Kind: domain.CreateWorktree, Mode: "Detached", Source: repo.Bisect.Candidate, BisectVersion: repo.Bisect.Version}
	case startBisect:
		s.label = "Start Bisect"
		s.op.Kind = domain.StartBisect
		s.op.Source = repo.Head
		s.detail = "Search between a known good ancestor and a bad commit. No checkout: HEAD, Index and working files stay in place. Test the candidate revision separately, then mark it."
	case bisectGood:
		s.label = "Mark Good"
		s.op.Kind = domain.BisectGood
		s.detail = "Classify this exact candidate as good and select the next candidate."
	case bisectBad:
		s.label = "Mark Bad"
		s.op.Kind = domain.BisectBad
		s.detail = "Classify this exact candidate as bad and select the next candidate."
	case bisectSkip:
		s.label = "Skip Candidate"
		s.op.Kind = domain.BisectSkip
		s.detail = "Skip this untestable candidate. Skipped commits can prevent identifying a unique first bad commit."
	case resetBisect:
		s.label = "End Bisect"
		s.op.Kind = domain.ResetBisect
		s.detail = "Remove this search's classifications and end Bisect. An externally started checkout search returns to its original checkout; tracked files must be clean."
	}
}

func bisectSummary(state domain.BisectState) string {
	if state.FirstBad != "" {
		return "First bad commit · " + state.FirstBad
	}
	if state.Ambiguous {
		return "Bisect is inconclusive because remaining candidates were skipped"
	}
	return "Bisect candidate · " + state.Candidate
}

func bisectFields(c *ui.Context, s *repositoryOperationDialog, busy bool) {
	if s.op.Kind == domain.StartBisect {
		ui.Text(c, "Bad commit or full reference")
		ui.TextInput(c, &s.op.Source).Label("Bisect bad revision").FillWidth().Disabled(busy)
		ui.Text(c, "Good ancestor commit or full reference")
		ui.TextInput(c, &s.op.Target).Label("Bisect good revision").FillWidth().Disabled(busy)
	} else if s.op.Kind >= domain.BisectGood && s.op.Kind <= domain.ResetBisect {
		ui.Text(c, s.op.Source).FillWidth()
		ui.Text(c, fmt.Sprintf("Original checkout: %s", s.snapshot.Bisect.Original)).FillWidth()
	}
}

func (a *Workspace) bisectControls(c *ui.Context, p tokens) {
	state := a.Model.Active().Repository.Bisect
	ui.Column(c).FillWidth().Padding(gap, inset).Gap(gap).Children(func() {
		ui.Text(c, bisectSummary(state)).SingleLine().Ellipsis("…").Bold()
		if state.NoCheckout {
			ui.Text(c, "No checkout · test the candidate revision separately").TextColor(p.theme.TextMuted)
		}
		button := func(id commandID, title string) {
			reason := a.disabledFor(id, commandTarget{})
			ui.Button(c.Key(string(id)), title).Label(commandLabel(id)).Tooltip(commandLabel(id) + " " + reason).
				Disabled(reason != "").OnClick(func() { a.run(id, commandTarget{}) })
		}
		ui.Row(c).Gap(gap).Wrap().Children(func() {
			button(showBisectCandidate, "Show in Log")
			button(createBisectWorktree, "Test Worktree…")
		})
		ui.Row(c).Gap(gap).Wrap().Children(func() {
			button(bisectGood, "Good")
			button(bisectBad, "Bad")
			button(bisectSkip, "Skip")
			button(resetBisect, "End Bisect…")
		})
	})
	divider(c, p)
}

func (a *Workspace) selectBisectCandidate() {
	repo := a.Model.Active().Repository
	index := slices.IndexFunc(repo.History, func(c domain.Commit) bool { return c.Hash == repo.Bisect.Candidate })
	if index < 0 {
		return
	}
	a.run(clearLogFilters, commandTarget{})
	a.run(selectCommit, commandTarget{index: index})
	a.run(showLog, commandTarget{})
	a.repo().logScroll = true
}

func bisectNeedsRange(op domain.RepositoryOperation) bool {
	return op.Kind == domain.StartBisect && (strings.TrimSpace(op.Source) == "" || strings.TrimSpace(op.Target) == "")
}
