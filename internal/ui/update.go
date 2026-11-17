package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vela/internal/domain"
)

func updateTrackedChanges(repo domain.Repository) bool {
	if len(repo.Staged) > 0 {
		return true
	}
	for _, file := range repo.Working {
		if file.Status != "?" {
			return true
		}
	}
	return false
}

func (a *Workspace) configureUpdate(s *repositoryOperationDialog) {
	s.op.UpdateMethod = "Fast-forward"
	s.op.UpdateCleanup = "None"
	if s.op.Branch == s.snapshot.CurrentBranch {
		s.op.UpdateMethod = a.Layout.VersionControl.Git.UpdateMethod
		if s.op.UpdateMethod != "Merge" && s.op.UpdateMethod != "Rebase" {
			s.op.UpdateMethod = "Merge"
		}
		if updateTrackedChanges(s.snapshot) {
			s.op.UpdateCleanup = a.Layout.VersionControl.Git.CleanWorkingTree
			if s.op.UpdateCleanup != "Stash" && s.op.UpdateCleanup != "Shelve" {
				s.op.UpdateCleanup = "Shelve"
			}
		}
	}
	s.detail = "Fetch " + s.op.Remote + "/" + s.op.Target + " and update local branch " + s.op.Branch + "."
}

func (a *Workspace) updateDirtyBuffer(root string) bool {
	for i, session := range a.Model.Sessions {
		if session.Repository.Root != root {
			continue
		}
		r := &a.repos[i]
		for _, entry := range r.tabs {
			if entry.source == "project" && entry.modified() {
				return true
			}
		}
		for _, entry := range []*tab{r.split, r.preview} {
			if entry != nil && entry.source == "project" && entry.modified() {
				return true
			}
		}
	}
	return false
}

func (a *Workspace) updateDisabled(s *repositoryOperationDialog) string {
	if s.op.Kind != domain.UpdateTrackedBranch || s.op.UpdateMethod == "" {
		return ""
	}
	if s.updateFinished {
		return "Close this result and refresh before starting another update"
	}
	if s.op.Branch != s.snapshot.CurrentBranch {
		return ""
	}
	if a.updateDirtyBuffer(s.root) {
		return "Save or discard unsaved source documents before Update"
	}
	if updateTrackedChanges(s.snapshot) && !s.op.ConfirmUpdateCleanup {
		return "Confirm saving and restoring tracked changes"
	}
	return ""
}

func (a *Workspace) updateFields(c *ui.Context, p tokens, s *repositoryOperationDialog, busy bool) {
	if s.op.Kind != domain.UpdateTrackedBranch || s.op.UpdateMethod == "" {
		return
	}
	ui.Text(c, "Repository: "+s.root).FillWidth()
	ui.Text(c, "Local branch: "+s.op.Branch+" · upstream: "+s.op.Remote+"/"+s.op.Target).FillWidth()
	if s.op.Branch != s.snapshot.CurrentBranch {
		ui.Text(c, "Non-current branch: fast-forward only, without checkout or cleaning the current worktree.").FillWidth().TextColor(p.theme.TextMuted)
	} else {
		ui.Select(c, &s.op.UpdateMethod, []string{"Merge", "Rebase", "Fast-forward"}).Label("Update integration method").FillWidth().Disabled(busy || s.updateFinished)
		if s.op.UpdateMethod == "Rebase" {
			ui.Text(c, "Rebase replays local commits and changes their IDs. Local merge commits require Merge or external Git.").FillWidth().TextColor(p.theme.TextMuted)
		}
		if updateTrackedChanges(s.snapshot) {
			ui.Select(c, &s.op.UpdateCleanup, []string{"Stash", "Shelve"}).Label("Update working tree cleanup").FillWidth().Disabled(busy || s.updateFinished)
			ui.Text(c, "Save all tracked changes before integration, then attempt to restore the index and worktree on success. The backup is kept even after restoration. Untracked and ignored files are not saved or deleted. Cancellation, failure or a paused integration leaves the backup for explicit restoration.").FillWidth().TextColor(p.theme.TextMuted)
			ui.Checkbox(c, &s.op.ConfirmUpdateCleanup, "Save tracked changes, clean them, and restore after successful Update").Disabled(busy || s.updateFinished)
		}
	}
	if reason := a.updateDisabled(s); reason != "" {
		ui.Text(c, reason).FillWidth().TextColor(p.theme.TextMuted)
	}
}

func (a *Workspace) applyBranchUpdate(s *repositoryOperationDialog) {
	if s.cancel != nil || a.updateDisabled(s) != "" || a.repositoryWrite != "" || a.repositoryRead != nil || a.Model.Active().Repository.Root != s.root {
		return
	}
	locked := map[*editor.State]bool{}
	lock := func(entry *tab) {
		if entry == nil || entry.source != "project" || entry.code == nil {
			return
		}
		if _, ok := locked[entry.code]; !ok {
			locked[entry.code] = entry.code.ReadOnly
			entry.code.ReadOnly = true
		}
	}
	if s.op.Branch == s.snapshot.CurrentBranch {
		for i := range a.repo().tabs {
			lock(&a.repo().tabs[i])
		}
		lock(a.repo().split)
		lock(a.repo().preview)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	s.cancel, s.message = cancel, ""
	a.updateWriting = s.root
	a.beginRepositoryWrite(s.root, "Updating "+s.op.Branch+"…", cancel)
	a.UpdateRepository(ctx, s.snapshot, s.op, func(updated domain.Repository, result domain.UpdateResult, err, refreshErr error) {
		cancel()
		s.cancel = nil
		a.updateWriting = ""
		for state, readOnly := range locked {
			state.ReadOnly = readOnly
		}
		message := updateResultMessage(result, err)
		paused := refreshErr == nil && (updated.ResolvingConflicts() || len(updated.Conflicts) > 0)
		if err == nil || paused {
			s.open = false
		} else {
			s.message = message
			s.updateFinished = true
		}
		a.finishRepositoryWrite(s.root, updated, refreshErr, message)
		// Writes or cancellation can change any mutable path, including renames.
		a.refreshHistoricalWriteViews(s.root, domain.RepositoryOperation{Kind: domain.UpdateTrackedBranch})
		if paused && a.Model.Active().Repository.Root == s.root {
			a.run(showCommit, commandTarget{})
		}
	})
}

func updateResultMessage(result domain.UpdateResult, err error) string {
	stages := []string{}
	if result.Fetched {
		stages = append(stages, "Fetch completed")
	}
	if result.Cleaned {
		stages = append(stages, "tracked changes saved and cleaned")
	}
	if result.Integrated {
		stages = append(stages, result.Method+" completed")
	}
	if result.Restored {
		stages = append(stages, "tracked changes restored")
	}
	if len(stages) == 0 {
		stages = append(stages, "Update did not complete")
	}
	message := strings.Join(stages, " · ")
	if result.BackupOID != "" {
		message += fmt.Sprintf(" · Backup kept: %s %s · %s · %s", result.BackupKind, result.BackupID, result.BackupOID, result.BackupName)
		if !result.Restored {
			message += " · Finish/abort the Git operation, then explicitly restore this backup from Stash/Shelf"
		}
	} else if result.BackupName != "" {
		message += " · Inspect Stash/Shelf for backup named " + result.BackupName + " before retrying"
	}
	if err != nil {
		message += " · Stopped: " + err.Error()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			message += " · Git may have written changes before cancellation; inspect the refreshed branch, index and worktree"
		}
	}
	return message
}
