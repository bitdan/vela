package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func startTestTask(t *testing.T, tasks *Tasks, root, key string) (context.Context, TaskID) {
	t.Helper()
	ctx, id, err := tasks.Start(context.Background(), root, "Read", key, time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ctx, id
}

func TestTasksCapacityEvictionAndRepositoryIdentity(t *testing.T) {
	var tasks Tasks
	var active []TaskID
	for i := 0; i < 32; i++ {
		_, id := startTestTask(t, &tasks, "repo-a", fmt.Sprint(i))
		active = append(active, id)
	}
	if _, _, err := tasks.Start(context.Background(), "repo-b", "Overflow", "", time.Minute, nil); err == nil {
		t.Fatal("33rd active task accepted")
	}
	for _, id := range active {
		tasks.Finish(id, nil, "done")
	}
	_, held := startTestTask(t, &tasks, "repo-b", "held")
	for i := 0; i < 105; i++ {
		_, id := startTestTask(t, &tasks, "repo-a", "")
		tasks.Finish(id, nil, "done")
	}
	snapshot := tasks.Snapshot()
	if len(snapshot) != 101 || tasks.ActiveCount() != 1 {
		t.Fatalf("history cap: %d / %d", len(snapshot), tasks.ActiveCount())
	}
	for _, item := range snapshot {
		if item.ID <= active[31] {
			t.Fatal("old completed record was retained")
		}
		if item.ID == held && (!item.Active() || item.Root != "repo-b") {
			t.Fatal("active record evicted or reassigned")
		}
	}
	_, a := startTestTask(t, &tasks, "repo-a", "same")
	_, b := startTestTask(t, &tasks, "repo-b", "same")
	if _, _, err := tasks.Start(context.Background(), "repo-a", "Duplicate", "same", time.Minute, nil); err == nil {
		t.Fatal("duplicate query accepted")
	}
	tasks.ClearCompleted()
	if len(tasks.Snapshot()) != 3 {
		t.Fatal("clear removed running records")
	}
	tasks.Finish(held, nil, "")
	tasks.Finish(a, nil, "")
	tasks.Finish(b, nil, "")
}

func TestTasksCancellationIsARequestAndCompletionIsTruthful(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want TaskState
	}{{"success", nil, TaskSucceeded}, {"failed", errors.New("write failed"), TaskFailed}, {"canceled", context.Canceled, TaskCanceled}, {"deadline", context.DeadlineExceeded, TaskTimedOut}} {
		t.Run(tc.name, func(t *testing.T) {
			var tasks Tasks
			owner := 0
			ctx, id, err := tasks.Start(context.Background(), "original", "Write", "", time.Minute, func() { owner++ })
			if err != nil {
				t.Fatal(err)
			}
			if !tasks.Cancel(id) || tasks.Cancel(id) || ctx.Err() != context.Canceled || owner != 1 {
				t.Fatal("cancel request was lost or repeated")
			}
			if tasks.ActiveCount() != 1 || tasks.Snapshot()[0].State != TaskCanceling || tasks.CanCancel(id) {
				t.Fatal("cancel claimed premature completion")
			}
			tasks.Finish(id, tc.err, "durable result")
			item := tasks.Snapshot()[0]
			if item.State != tc.want || item.Active() || item.Root != "original" || !strings.Contains(item.Result, "durable result") || tasks.ActiveCount() != 0 {
				t.Fatalf("completion: %+v", item)
			}
			if tc.err == nil && !strings.Contains(item.Result, "does not roll back") {
				t.Fatal("late success omitted cancellation explanation")
			}
			tasks.Finish(id, errors.New("duplicate callback"), "")
			if tasks.Snapshot()[0].State != tc.want {
				t.Fatal("duplicate callback changed final state")
			}
		})
	}
}

func TestTasksBoundedPhasesAndSnapshotsAreIndependent(t *testing.T) {
	var tasks Tasks
	_, id := startTestTask(t, &tasks, "repo", "")
	for i := 0; i < 40; i++ {
		tasks.Phase(id, fmt.Sprint(i))
	}
	snapshot := tasks.Snapshot()
	if len(snapshot[0].Phases) != 32 || snapshot[0].Phases[0].Message != "Started" || snapshot[0].Phases[31].Message != "39" {
		t.Fatal("phase retention lost first/latest checkpoint")
	}
	snapshot[0].Root = "changed"
	snapshot[0].Phases[0].Message = "changed"
	if item := tasks.Snapshot()[0]; item.Root != "repo" || item.Phases[0].Message != "Started" {
		t.Fatal("snapshot aliases mutable records")
	}
	tasks.Finish(id, nil, "")
	tasks.Phase(id, "late")
	if tasks.Snapshot()[0].Phases[31].Message != "Succeeded" {
		t.Fatal("late phase changed finished task")
	}
}
