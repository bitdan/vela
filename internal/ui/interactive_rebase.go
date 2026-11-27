package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/application"
	"vela/internal/domain"
)

const (
	confirmInteractiveRebase commandID = "git.confirm-interactive-rebase"
	cancelInteractiveRebase  commandID = "git.cancel-interactive-rebase"
	refreshInteractiveRebase commandID = "git.refresh-interactive-rebase"
	moveRebaseUp             commandID = "git.move-rebase-commit-up"
	moveRebaseDown           commandID = "git.move-rebase-commit-down"
)

var interactiveRebaseActions = []string{"Pick", "Reword", "Squash", "Fixup", "Drop"}

type interactiveRebaseEntry struct {
	Commit, Action, Message string
}

type interactiveRebaseDialog struct {
	snapshot          domain.Repository
	plan              domain.CommitRewritePreview
	entries           []interactiveRebaseEntry
	request           *readRequest
	cancel            context.CancelFunc
	problem           string
	selected          int
	ack, busy         bool
	list, descendants ui.ListState
}

func isInteractiveRebaseCommand(id commandID) bool {
	return id == rebaseFrom || id == confirmInteractiveRebase || id == cancelInteractiveRebase || id == refreshInteractiveRebase || id == moveRebaseUp || id == moveRebaseDown
}

func (a *Workspace) interactiveRebaseBaseReason() string {
	repo := a.Model.Active().Repository
	if reason := a.squashBaseReason(); reason != "" {
		return strings.ReplaceAll(reason, "Squash", "Interactive Rebase")
	}
	if err := a.Layout.VersionControl.Git.SafetyPolicy().CheckOperation(repo, domain.RepositoryOperation{Kind: domain.InteractiveRebase}); err != nil {
		return err.Error()
	}
	return ""
}

func (a *Workspace) interactiveRebaseDisabled(id commandID, target commandTarget) string {
	if id == cancelInteractiveRebase {
		if a.interactiveRebase == nil {
			return "No Interactive Rebase dialog"
		}
		return ""
	}
	if reason := a.interactiveRebaseBaseReason(); reason != "" {
		return reason
	}
	if id == rebaseFrom {
		if a.PreviewCommitRewrite == nil {
			return "Interactive Rebase preview is unavailable"
		}
		if a.interactiveRebase != nil || a.squash != nil || a.repositoryOperation != nil || a.patchDialog != nil || a.patchImport != nil || a.commitConfirmation != nil {
			return "Close the current repository dialog"
		}
		selection := a.interactiveRebaseSelection(target)
		if err := application.ValidateCommitRangeSelection(selection, 1); err != nil {
			return err.Error()
		}
		for _, commit := range selection {
			if !slices.ContainsFunc(a.Model.Active().Repository.History, func(candidate domain.Commit) bool { return candidate.Hash == commit }) {
				return "Refresh and select loaded commits"
			}
		}
		return ""
	}
	dialog := a.interactiveRebase
	if dialog == nil || dialog.busy || dialog.request != nil || dialog.snapshot.Root != a.Model.Active().Repository.Root {
		return "Wait for the Interactive Rebase preview in the original repository"
	}
	if id == refreshInteractiveRebase {
		return ""
	}
	if id == moveRebaseUp || id == moveRebaseDown {
		if dialog.plan.Version == "" || dialog.selected < 0 || dialog.selected >= len(dialog.entries) {
			return "Select a commit in the Rebase plan"
		}
		if id == moveRebaseUp && dialog.selected == 0 || id == moveRebaseDown && dialog.selected == len(dialog.entries)-1 {
			return "The commit is already at the edge of the plan"
		}
		return ""
	}
	repo := a.Model.Active().Repository
	if repo.Head != dialog.snapshot.Head || repo.IndexVersion != dialog.snapshot.IndexVersion || repo.WorktreeVersion != dialog.snapshot.WorktreeVersion {
		return "Repository changed; preview again"
	}
	if dialog.plan.Version == "" || !dialog.ack {
		return "Preview and confirm the final Rebase sequence"
	}
	if err := application.ValidateInteractiveRebasePlan(dialog.plan, dialog.operationSteps()); err != nil {
		return err.Error()
	}
	return ""
}

func (a *Workspace) interactiveRebaseSelection(target commandTarget) []string {
	selection := slices.Sorted(a.repo().logChosen.All())
	if target.source != "" && !a.repo().logChosen.Has(target.source) {
		selection = []string{target.source}
	}
	if len(selection) == 0 {
		if commit, found := a.operationCommit(target); found {
			selection = []string{commit.Hash}
		}
	}
	return selection
}

func (a *Workspace) runInteractiveRebase(id commandID, target commandTarget) {
	switch id {
	case rebaseFrom:
		a.interactiveRebase = &interactiveRebaseDialog{}
		a.previewInteractiveRebase(a.interactiveRebaseSelection(target))
	case refreshInteractiveRebase:
		a.previewInteractiveRebase(nil)
	case moveRebaseUp, moveRebaseDown:
		dialog := a.interactiveRebase
		offset := -1
		if id == moveRebaseDown {
			offset = 1
		}
		other := dialog.selected + offset
		dialog.entries[dialog.selected], dialog.entries[other] = dialog.entries[other], dialog.entries[dialog.selected]
		dialog.selected = other
		dialog.ack = false
	case confirmInteractiveRebase:
		a.applyInteractiveRebase()
	case cancelInteractiveRebase:
		dialog := a.interactiveRebase
		if dialog.busy {
			if dialog.cancel != nil {
				dialog.cancel()
			}
			dialog.problem = "Cancellation requested; waiting for Git and refresh"
		} else {
			stopRead(&dialog.request)
			a.interactiveRebase = nil
		}
	}
}

func (a *Workspace) previewInteractiveRebase(selection []string) {
	dialog := a.interactiveRebase
	dialog.snapshot = a.Model.Active().Repository
	if selection == nil {
		selection = interactiveRebaseSelection(dialog.entries)
	}
	dialog.plan = domain.CommitRewritePreview{}
	dialog.entries = nil
	dialog.selected = 0
	dialog.ack = false
	dialog.problem = ""
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	request := &readRequest{cancel: cancel}
	stopRead(&dialog.request)
	dialog.request = request
	a.PreviewCommitRewrite(ctx, dialog.snapshot, domain.InteractiveRebase, slices.Clone(selection), func(plan domain.CommitRewritePreview, err error) {
		if a.interactiveRebase != dialog || dialog.request != request || a.Model.Active().Repository.Root != dialog.snapshot.Root {
			return
		}
		if err == nil {
			err = ctx.Err()
		}
		stopRead(&dialog.request)
		if err != nil {
			dialog.problem = "Could not preview Interactive Rebase · " + err.Error()
			return
		}
		actual := slices.Clone(plan.Selected)
		expected := slices.Clone(selection)
		slices.Sort(actual)
		slices.Sort(expected)
		if plan.Version == "" || len(plan.RebaseCommits) != len(plan.Selected) || !slices.Equal(actual, expected) || len(plan.Selected)+len(plan.Replay) > 1000 {
			dialog.problem = "Git returned an invalid Interactive Rebase preview"
			return
		}
		for index, commit := range plan.RebaseCommits {
			if commit.Commit != plan.Selected[len(plan.Selected)-1-index] {
				dialog.problem = "Git returned an invalid Interactive Rebase commit identity"
				return
			}
		}
		dialog.plan = plan
		for _, commit := range plan.RebaseCommits {
			dialog.entries = append(dialog.entries, interactiveRebaseEntry{Commit: commit.Commit, Action: "Pick", Message: commit.Message})
		}
	})
}

func interactiveRebaseSelection(entries []interactiveRebaseEntry) []string {
	selection := make([]string, len(entries))
	for index, entry := range entries {
		selection[index] = entry.Commit
	}
	return selection
}

func (dialog *interactiveRebaseDialog) operationSteps() []domain.InteractiveRebaseStep {
	steps := make([]domain.InteractiveRebaseStep, len(dialog.entries))
	for index, entry := range dialog.entries {
		steps[index] = domain.InteractiveRebaseStep{Commit: entry.Commit, Action: domain.InteractiveRebaseAction(entry.Action)}
		if entry.Action == "Reword" {
			steps[index].Message = entry.Message
		}
	}
	return steps
}

func (a *Workspace) applyInteractiveRebase() {
	dialog := a.interactiveRebase
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	dialog.cancel = cancel
	dialog.busy = true
	dialog.problem = ""
	restoreEditors := a.lockShelfEditors()
	op := domain.RepositoryOperation{Kind: domain.InteractiveRebase, RebaseSteps: dialog.operationSteps(), RebaseVersion: dialog.plan.Version, ConfirmRewrite: dialog.ack}
	a.beginRepositoryWrite(dialog.snapshot.Root, "Interactive Rebase…", cancel)
	a.OperateRepository(ctx, dialog.snapshot, op, func(updated domain.Repository, err, refreshErr error) {
		cancel()
		restoreEditors()
		dialog.cancel = nil
		dialog.busy = false
		message := "Interactive Rebase completed"
		if err != nil {
			message = "Interactive Rebase stopped · " + err.Error()
			dialog.problem = message
			dialog.plan.Version = ""
			dialog.ack = false
		}
		if updated.Rebase.Active() {
			message += " · Rebase is paused; resolve and use Continue, Skip or Abort"
		}
		a.finishRepositoryWrite(dialog.snapshot.Root, updated, refreshErr, message)
		a.refreshHistoricalWriteViews(dialog.snapshot.Root, op)
		if err == nil || updated.Rebase.Active() || a.Model.Active().Repository.Root != dialog.snapshot.Root {
			if a.interactiveRebase == dialog {
				a.interactiveRebase = nil
			}
		}
		if err == nil {
			for index, session := range a.Model.Sessions {
				if session.Repository.Root == dialog.snapshot.Root {
					a.repos[index].logChosen.Clear()
				}
			}
		}
	})
}

func (a *Workspace) interactiveRebaseView(c *ui.Context, p tokens) {
	dialog := a.interactiveRebase
	open := true
	width, height := c.Size()
	dialog.list.Handle.OnShortcut(c, ui.Alt, ui.KeyUp, func() {
		if a.disabledFor(moveRebaseUp, commandTarget{}) == "" {
			a.run(moveRebaseUp, commandTarget{})
		}
	})
	dialog.list.Handle.OnShortcut(c, ui.Alt, ui.KeyDown, func() {
		if a.disabledFor(moveRebaseDown, commandTarget{}) == "" {
			a.run(moveRebaseDown, commandTarget{})
		}
	})
	ui.DialogBase(c, &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, .28))
		panel.Width(min(width-32, 1000)).MaxHeight(height-48).Background(p.panel).Border(1, p.theme.Border).Radius(6).Column().Label("Interactive Rebase dialog").Children(func() {
			heading(c, p, "Interactive Rebase", dialog.snapshot.CurrentBranch)
			ui.Scroll(c).FillWidth().MaxHeight(height - 160).Children(func() {
				ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
					ui.Text(c, dialog.snapshot.Root).FillWidth().SingleLine().Ellipsis("…")
					if dialog.request != nil {
						ui.Text(c, "Checking private commit range and remote-tracking refs…")
					}
					if dialog.plan.Version != "" {
						ui.Text(c, fmt.Sprintf("Final sequence · %d selected commits · %d affected descendants replayed afterward", len(dialog.entries), len(dialog.plan.Replay))).FillWidth()
						ui.Text(c, "Oldest first. Alt+Up / Alt+Down moves the selected commit. Drop removes a commit and its changes.").FillWidth().TextColor(p.theme.TextMuted)
						dialog.list.Selected = &dialog.selected
						dialog.list.Key = func(index int) any { return dialog.entries[index].Commit }
						dialog.list.Label = func(index int) string {
							entry := dialog.entries[index]
							return entry.Action + " " + entry.Commit + " " + rebaseSubject(entry.Message)
						}
						ui.List(c, &dialog.list, len(dialog.entries), func(index int) {
							entry := &dialog.entries[index]
							ui.Row(c).FillWidth().Height(rowHeight).Gap(gap).AlignItems(ui.Center).Children(func() {
								if ui.Select(c, &entry.Action, interactiveRebaseActions).Label("Rebase action " + entry.Commit).Width(110).Disabled(dialog.busy).Changed() {
									dialog.selected = index
									dialog.ack = false
								}
								label := entry.Commit + " · " + rebaseSubject(entry.Message)
								ui.Text(c, label).Grow(1).SingleLine().Ellipsis("…").Tooltip(label)
							})
						}).FillWidth().Height(min(float32(len(dialog.entries))*rowHeight, 8*rowHeight)).Label("Interactive Rebase final sequence")
						if dialog.selected >= 0 && dialog.selected < len(dialog.entries) {
							entry := &dialog.entries[dialog.selected]
							ui.TextArea(c, &entry.Message).Label("Reword message " + entry.Commit).Height(90).FillWidth().Disabled(dialog.busy || entry.Action != "Reword")
						}
						ui.Text(c, "Affected descendants (unchanged Pick steps):").FillWidth()
						dialog.descendants.Key = func(index int) any { return dialog.plan.Replay[index] }
						dialog.descendants.Label = func(index int) string { return dialog.plan.Replay[index] }
						ui.List(c, &dialog.descendants, len(dialog.plan.Replay), func(index int) {
							commit := dialog.plan.Replay[index]
							ui.Text(c, "Pick "+commit).FillWidth().SingleLine().Ellipsis("…")
						}).FillWidth().Height(min(float32(len(dialog.plan.Replay))*rowHeight, 4*rowHeight)).Label("Interactive Rebase affected descendants")
					}
					ui.Text(c, "Git natively rewrites the selected range and replays later commits. Authors and author dates are retained; conflicts use the three-way editor and Rebase Continue, Skip or Abort. Cancellation does not promise rollback.").FillWidth().TextColor(p.theme.TextMuted)
					ui.Checkbox(c, &dialog.ack, "Rewrite this final sequence and its affected descendants").Disabled(dialog.busy || dialog.request != nil || dialog.plan.Version == "")
					if dialog.problem != "" {
						ui.Text(c, dialog.problem).FillWidth().TextColor(p.theme.Danger)
					}
				})
			})
			ui.Row(c).FillWidth().Padding(inset).Gap(gap).Justify(ui.End).Children(func() {
				for _, action := range []struct {
					id    commandID
					label string
				}{{moveRebaseUp, "Move Up"}, {moveRebaseDown, "Move Down"}, {cancelInteractiveRebase, "Cancel"}, {refreshInteractiveRebase, "Refresh Preview"}, {confirmInteractiveRebase, "Run Interactive Rebase"}} {
					reason := a.disabledFor(action.id, commandTarget{})
					ui.Button(c, action.label).Label(action.label).Disabled(reason != "").Tooltip(reason).OnClick(func() { a.run(action.id, commandTarget{}) })
				}
			})
		})
	})
	if !open {
		a.run(cancelInteractiveRebase, commandTarget{})
	}
}

func rebaseSubject(message string) string {
	subject, _, _ := strings.Cut(message, "\n")
	return strings.TrimSpace(subject)
}
