package ui

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"vela/internal/domain"
	velaruntime "vela/internal/runtime"
)

func (a *Workspace) TaskOperationName(op domain.RepositoryOperation) string {
	if s := a.repositoryOperation; s != nil && s.op.Kind == op.Kind && s.label != "" {
		return s.label
	}
	switch op.Kind {
	case domain.RewordCommit:
		return "Edit Commit Message"
	case domain.SquashCommits:
		return "Squash Commits"
	case domain.StageHunk:
		return "Stage Hunk"
	case domain.UnstageHunk:
		return "Unstage Hunk"
	case domain.DiscardHunk:
		return "Discard Hunk"
	case domain.SaveMergeResult:
		return "Save Merge Result"
	case domain.RestoreRevisionFiles:
		return "Restore Historical Files"
	case domain.CherryPickSelected:
		return "Cherry-Pick Selected Changes"
	case domain.RevertSelected:
		return "Revert Selected Changes"
	}
	return "Repository operation"
}

// TaskWriteCancellation captures the existing write owner's cancellation before
// dispatch. Explicit task cancellation must also reach parent-context guards
// such as Clone/Init navigation, even if success is already queued on the UI.
// Timer cleanup at normal completion must not invoke this hook.
func (a *Workspace) TaskWriteCancellation() context.CancelFunc {
	cancel, root := a.repositoryWriteCancel, a.repositoryWrite
	if cancel == nil && a.repositoryInit != nil {
		cancel = a.repositoryInit.cancel
	}
	if cancel == nil {
		return nil
	}
	return func() {
		cancel()
		if root != "" && a.repositoryWrite == root {
			a.repositoryCancelRequested = true
		}
	}
}

const (
	showTasks          commandID = "workspace.show-tasks"
	cancelSelectedTask commandID = "workspace.cancel-selected-task"
	clearTaskHistory   commandID = "workspace.clear-task-history"
)

func (a *Workspace) activeCancellableTask() velaruntime.TaskID {
	for _, item := range a.Tasks.Snapshot() {
		if item.Root == a.Model.Active().Repository.Root && a.Tasks.CanCancel(item.ID) {
			return item.ID
		}
	}
	return 0
}

func (a *Workspace) taskDisabled(id commandID, target commandTarget) (string, bool) {
	if target.taskID == 0 {
		target.taskID = uint64(a.taskSelection)
	}
	switch id {
	case showTasks:
		return "", true
	case cancelSelectedTask:
		if !a.Tasks.CanCancel(velaruntime.TaskID(target.taskID)) {
			return "Select a running task that has not been canceled", true
		}
		return "", true
	case clearTaskHistory:
		if len(a.Tasks.Snapshot()) == a.Tasks.ActiveCount() {
			return "No completed task history", true
		}
		return "", true
	}
	return "", false
}

func (a *Workspace) taskCommand(id commandID, target commandTarget) bool {
	if target.taskID == 0 {
		target.taskID = uint64(a.taskSelection)
	}
	switch id {
	case showTasks:
		a.tasksOpen = true
		a.Layout.HideGit = false
	case cancelSelectedTask:
		a.Tasks.Cancel(velaruntime.TaskID(target.taskID))
		a.status = "Cancellation requested · waiting for task callback; changes are not rolled back"
	case clearTaskHistory:
		a.Tasks.ClearCompleted()
		a.taskSelected = -1
		a.taskSelection = 0
	default:
		return false
	}
	return true
}

func (a *Workspace) taskPanel(c *ui.Context, p tokens) {
	items := a.Tasks.Snapshot()
	a.taskSelected = -1
	for i := range items {
		if items[i].ID == a.taskSelection {
			a.taskSelected = i
			break
		}
	}
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).FillWidth().Height(headerHeight).Padding(0, inset).Gap(gap).AlignItems(ui.Center).Children(func() {
			ui.Text(c, fmt.Sprintf("All repositories · %d active · %d completed (last 100)", a.Tasks.ActiveCount(), len(items)-a.Tasks.ActiveCount())).Grow(1).SingleLine().Ellipsis("…")
			ui.Button(c, "Clear Completed").Disabled(a.disabledFor(clearTaskHistory, commandTarget{}) != "").OnClick(func() { a.run(clearTaskHistory, commandTarget{}) })
		})
		divider(c, p)
		if len(items) == 0 {
			ui.Column(c).Fill().Padding(inset).Children(func() {
				ui.Text(c, "No tasks recorded in this session").TextColor(p.theme.TextMuted)
				ui.Text(c, "Repository reads, writes and background work appear here. History is kept in memory.").TextColor(p.theme.TextMuted)
			})
			return
		}
		ui.SplitPercent(c, &a.taskSplit, func() {
			a.taskList.Selected = &a.taskSelected
			a.taskList.Key = func(i int) any { return items[i].ID }
			a.taskList.Label = func(i int) string {
				return fmt.Sprintf("%d · %s · %s · %s", items[i].ID, items[i].Name, items[i].Root, items[i].State)
			}
			list := ui.List(c, &a.taskList, len(items), func(i int) {
				item := items[i]
				ui.Row(c).FillWidth().Height(rowHeight).Padding(0, gap).Gap(gap).AlignItems(ui.Center).ContextMenu(func(m *ui.Menu) {
					a.menuGroup(m, commandTarget{taskID: uint64(item.ID)}, cancelSelectedTask)
				}).Children(func() {
					ui.Text(c, fmt.Sprintf("#%d", item.ID)).Width(48).TextColor(p.theme.TextMuted)
					ui.Text(c, item.Name).Grow(1).SingleLine().Ellipsis("…").Tooltip(item.Root)
					ui.Text(c, string(item.State)).Width(85).TextColor(p.theme.TextMuted)
					ui.Text(c, item.Started.Format("15:04:05")).Width(64).TextColor(p.theme.TextMuted)
				})
			}).Fill().Label("Task Center tasks")
			if list.Changed() && a.taskSelected >= 0 && a.taskSelected < len(items) {
				a.taskSelection = items[a.taskSelected].ID
			}
		}, func() {
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Padding(inset).Gap(gap).Children(func() {
					if a.taskSelected < 0 || a.taskSelected >= len(items) {
						ui.Text(c, "Select a task to view its stages and result")
						return
					}
					item := items[a.taskSelected]
					ui.Text(c, fmt.Sprintf("#%d · %s · %s", item.ID, item.Name, item.State)).Bold()
					ui.Text(c, "Repository: "+item.Root).FillWidth()
					ui.Text(c, "Started: "+item.Started.Format(time.DateTime)).TextColor(p.theme.TextMuted)
					ui.Text(c, "Deadline: "+item.Deadline.Format(time.DateTime)).TextColor(p.theme.TextMuted)
					if !item.Finished.IsZero() {
						ui.Text(c, "Finished: "+item.Finished.Format(time.DateTime)).TextColor(p.theme.TextMuted)
					}
					target := commandTarget{taskID: uint64(item.ID)}
					ui.Button(c, "Cancel Task").Disabled(a.disabledFor(cancelSelectedTask, target) != "").OnClick(func() { a.run(cancelSelectedTask, target) })
					ui.Text(c, "Cancellation waits for the operation and refresh. It does not undo changes.").FillWidth().TextColor(p.theme.TextMuted)
					for _, phase := range item.Phases {
						ui.Text(c, phase.At.Format("15:04:05")+" · "+phase.Message).FillWidth()
					}
					if item.Result != "" {
						divider(c, p)
						ui.Text(c, item.Result).FillWidth()
					}
				})
			})
		}).SplitReset(52).Grow(1).FillWidth()
	})
}
