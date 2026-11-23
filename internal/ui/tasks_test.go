package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	native "github.com/egoist/mygo/ui"
	"vela/internal/application"
	velaruntime "vela/internal/runtime"
)

func TestTaskCenterNativeLifecycle(t *testing.T) {
	for _, theme := range []string{"dark", "light"} {
		t.Run(theme, func(t *testing.T) {
			layout := application.DefaultLayout()
			layout.Dark = theme == "dark"
			a := New(application.New(), layout)
			root := t.TempDir()
			a.Model.Active().Repository.Root = root
			tt := native.NewTester(a.View, 1440, 900)
			if err := tt.Click("Tasks"); err != nil {
				t.Fatal(err)
			}
			if !tt.HasText("No tasks recorded in this session") || a.disabledFor(clearTaskHistory, commandTarget{}) != "No completed task history" || a.disabledFor(cancelSelectedTask, commandTarget{}) != "Select a running task that has not been canceled" {
				t.Fatal("empty task state/reasons incorrect")
			}
			ctx, id, err := a.Tasks.Start(context.Background(), root, "Fetch origin", "fetch", time.Minute, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, other, err := a.Tasks.Start(context.Background(), "another repository", "Read history", "read", time.Minute, nil)
			if err != nil {
				t.Fatal(err)
			}
			a.Tasks.Finish(other, errors.New("remote unavailable"), "No local changes")
			tt.Frame()
			if err := tt.Click("Task Center tasks"); err != nil {
				t.Fatal(err)
			}
			tt.Key(0, native.KeyHome)
			tt.Key(0, native.KeyDown)
			if a.taskSelection != id || !tt.Focused("Task Center tasks") {
				t.Fatalf("keyboard selection/focus: %d", a.taskSelection)
			}
			if err := tt.RightClick("Fetch origin"); err != nil {
				t.Fatal(err)
			}
			if _, ok := tt.Access("Cancel Selected Task"); !ok {
				t.Fatal("task context command missing")
			}
			tt.Key(0, native.KeyEscape)
			bounds, ok := tt.Find("Task Center tasks")
			if !ok {
				t.Fatal("task list bounds missing")
			}
			before := a.taskSplit
			x, y := bounds.X+bounds.W+2, bounds.Y+bounds.H/2
			tt.Press(x, y)
			tt.Move(x+60, y)
			tt.Release(x+60, y)
			if a.taskSplit <= before {
				t.Fatalf("task splitter did not resize: %g -> %g", before, a.taskSplit)
			}
			if dir := os.Getenv("VELA_SCREENSHOTS"); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				writeScreenshot(t, tt, filepath.Join(dir, theme+"-task-center.png"))
			}
			if err := tt.Click("Cancel Task"); err != nil {
				t.Fatal(err)
			}
			if ctx.Err() != context.Canceled || a.Tasks.ActiveCount() != 1 || a.Tasks.Snapshot()[1].State != velaruntime.TaskCanceling {
				t.Fatal("cancel was treated as completed")
			}
			if a.disabledFor(cancelSelectedTask, commandTarget{}) == "" {
				t.Fatal("canceling task still cancellable")
			}
			a.Tasks.Finish(id, nil, "Fetch completed before cancellation")
			tt.Frame()
			if a.Tasks.Snapshot()[1].State != velaruntime.TaskSucceeded || !tt.HasText("Fetch completed before cancellation") {
				t.Fatal("late success not reported")
			}
			if err := tt.Click("Clear Completed"); err != nil {
				t.Fatal(err)
			}
			if len(a.Tasks.Snapshot()) != 0 || !tt.HasText("No tasks recorded in this session") {
				t.Fatal("clear did not restore empty state")
			}
			// Hide/restore the bottom panel without losing the task-center view.
			resized := a.taskSplit
			if err := tt.Click("Toggle Git"); err != nil {
				t.Fatal(err)
			}
			if !a.Layout.HideGit {
				t.Fatal("task panel did not hide")
			}
			if err := tt.Click("Toggle Git"); err != nil {
				t.Fatal(err)
			}
			if a.taskSplit != resized {
				t.Fatal("restoring panel lost task split")
			}
			if a.Layout.HideGit || !a.tasksOpen || !tt.HasText("No tasks recorded in this session") {
				t.Fatal("panel restore failed")
			}
		})
	}
}

func TestTaskCenterKeepsOriginalRepositoryOwnership(t *testing.T) {
	a := New(application.New(), application.DefaultLayout())
	a.Model.Active().Repository.Root = t.TempDir()
	root := a.Model.Active().Repository.Root
	_, id, err := a.Tasks.Start(context.Background(), root, "Write", "", time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.run(selectRepo, commandTarget{index: 1})
	if a.activeCancellableTask() != 0 {
		t.Fatal("old task became the new repository's active write")
	}
	a.Tasks.Finish(id, nil, "done")
	if a.Tasks.Snapshot()[0].Root != root {
		t.Fatal("task record moved repositories")
	}
}
