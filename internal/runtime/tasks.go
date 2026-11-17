package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type TaskID uint64
type TaskState string

const (
	TaskRunning   TaskState = "Running"
	TaskCanceling TaskState = "Canceling"
	TaskSucceeded TaskState = "Succeeded"
	TaskFailed    TaskState = "Failed"
	TaskCanceled  TaskState = "Canceled"
	TaskTimedOut  TaskState = "Timed out"
)

type TaskPhase struct {
	At      time.Time
	Message string
}

// TaskInfo is a bounded, in-memory record. Root always belongs to the original
// request; navigating to another repository never transfers its ownership.
type TaskInfo struct {
	ID                          TaskID
	Root, Name, Key             string
	State                       TaskState
	Started, Finished, Deadline time.Time
	Phases                      []TaskPhase
	Result                      string
}

func (t TaskInfo) Active() bool { return t.Finished.IsZero() }

type runningTask struct {
	ctx    context.Context
	cancel context.CancelFunc
	stop   context.CancelFunc
}

// Tasks is confined to the UI thread. Workers receive contexts and report
// through Bootstrap's main-thread dispatcher, never mutate this state directly.
// At most 32 requests and 100 completed records are retained; phases and text
// are capped as well. This does not schedule Git or replace Executor locks.
type Tasks struct {
	next    TaskID
	items   []TaskInfo
	running map[TaskID]runningTask
}

func (t *Tasks) Start(parent context.Context, root, name, key string, timeout time.Duration, cancelOwner context.CancelFunc) (context.Context, TaskID, error) {
	if err := parent.Err(); err != nil {
		return nil, 0, err
	}
	if len(t.running) >= 32 {
		return nil, 0, errors.New("task limit reached; wait for running tasks")
	}
	for _, item := range t.items {
		if task, ok := t.running[item.ID]; ok && task.ctx.Err() == nil && item.Root == root && item.Key == key && key != "" {
			return nil, 0, fmt.Errorf("task %d is already running: %s", item.ID, item.Name)
		}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	t.next++
	id := t.next
	deadline, _ := ctx.Deadline()
	t.items = append(t.items, TaskInfo{ID: id, Root: root, Name: taskText(name), Key: key, State: TaskRunning, Started: time.Now(), Deadline: deadline})
	if t.running == nil {
		t.running = make(map[TaskID]runningTask)
	}
	t.running[id] = runningTask{ctx: ctx, stop: cancel, cancel: func() {
		cancel()
		if cancelOwner != nil {
			cancelOwner()
		}
	}}
	t.Phase(id, "Started")
	return ctx, id, nil
}

func (t *Tasks) Phase(id TaskID, message string) {
	for i := range t.items {
		item := &t.items[i]
		if item.ID != id || !item.Active() {
			continue
		}
		if len(item.Phases) >= 32 {
			item.Phases = slices.Delete(item.Phases, 1, 2)
		}
		item.Phases = append(item.Phases, TaskPhase{time.Now(), taskText(message)})
		return
	}
}

func (t *Tasks) Finish(id TaskID, err error, result string) {
	task, ok := t.running[id]
	if !ok {
		return
	}
	for i := range t.items {
		item := &t.items[i]
		if item.ID != id {
			continue
		}
		item.State = TaskSucceeded
		if err != nil {
			item.State = TaskFailed
			if errors.Is(err, context.Canceled) {
				item.State = TaskCanceled
			}
			if errors.Is(err, context.DeadlineExceeded) {
				item.State = TaskTimedOut
			}
			result = strings.TrimSpace(result + "\n" + err.Error())
		} else if task.ctx.Err() != nil {
			result = strings.TrimSpace(result + "\nOperation returned success after cancellation/deadline; inspect the recorded result. Cancellation does not roll back changes.")
		}
		if result == "" {
			result = "Completed"
		}
		item.Result = taskText(result)
		t.Phase(id, string(item.State))
		item.Finished = time.Now()
		break
	}
	task.stop()
	delete(t.running, id)
	completed := len(t.items) - len(t.running)
	for completed > 100 {
		i := slices.IndexFunc(t.items, func(item TaskInfo) bool { return !item.Active() })
		t.items = slices.Delete(t.items, i, i+1)
		completed--
	}
}

func (t *Tasks) Cancel(id TaskID) bool {
	task, ok := t.running[id]
	if !ok || task.ctx.Err() != nil {
		return false
	}
	task.cancel()
	t.Phase(id, "Cancellation requested; waiting for operation and refresh. Changes are not rolled back.")
	for i := range t.items {
		if t.items[i].ID == id {
			t.items[i].State = TaskCanceling
		}
	}
	return true
}

func (t *Tasks) CancelAll() {
	for id := range t.running {
		t.Cancel(id)
	}
}
func (t *Tasks) ActiveCount() int { return len(t.running) }
func (t *Tasks) CanCancel(id TaskID) bool {
	task, ok := t.running[id]
	return ok && task.ctx.Err() == nil
}
func (t *Tasks) ClearCompleted() {
	t.items = slices.DeleteFunc(t.items, func(item TaskInfo) bool { return !item.Active() })
}
func (t *Tasks) Snapshot() []TaskInfo {
	items := slices.Clone(t.items)
	for i := range items {
		items[i].Phases = slices.Clone(items[i].Phases)
		if task, ok := t.running[items[i].ID]; ok && task.ctx.Err() != nil {
			items[i].State = TaskCanceling
		}
	}
	slices.Reverse(items)
	return items
}

func taskText(s string) string {
	const limit = 4096
	if len(s) <= limit {
		return s
	}
	return strings.ToValidUTF8(s[:limit], "") + "… (truncated)"
}
