package application

import (
	"reflect"
	"testing"

	"vela/internal/domain"
)

func TestChangelistsLifecycleAndReconcile(t *testing.T) {
	var lists ChangelistState
	repo := domain.Repository{Working: []domain.File{{Path: "one.go"}, {Path: "two.go"}}, Staged: []domain.File{{Path: "one.go"}}}
	lists.Normalize(repo)
	if len(lists.Lists) != 1 || lists.Active != "default" || !reflect.DeepEqual(lists.Lists[0].Paths, []string{"one.go", "two.go"}) {
		t.Fatalf("default state: %+v", lists)
	}
	id, err := lists.Create("Feature 中文")
	if err != nil {
		t.Fatal(err)
	}
	if err := lists.Move([]string{"one.go"}, id); err != nil {
		t.Fatal(err)
	}
	lists.Active = id
	repo.Working = []domain.File{{Path: "renamed.go", OldPath: "one.go", Status: "R"}, {Path: "new.go"}, {Path: "two.go"}}
	repo.Staged = nil
	lists.Normalize(repo)
	if lists.Owner("renamed.go") != id || lists.Owner("new.go") != id || lists.Owner("two.go") != "default" {
		t.Fatalf("rename/new assignment: %+v", lists)
	}
	cloned := CloneChangelists(lists)
	cloned.Lists[1].Paths[0] = "mutated"
	if reflect.DeepEqual(cloned, lists) {
		t.Fatal("clone aliases member paths")
	}
	if err := lists.Rename(id, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if err := lists.Remove(id); err != nil {
		t.Fatal(err)
	}
	if lists.Active != "default" || lists.Owner("renamed.go") != "default" {
		t.Fatal("remove lost members/active list")
	}
}

func TestChangelistsRejectInvalidAndNormalizeLegacy(t *testing.T) {
	var lists ChangelistState
	lists.Normalize(domain.Repository{})
	for _, name := range []string{"", "Default", "default", "bad\nname", "bad\x00name"} {
		if _, err := lists.Create(name); err == nil {
			t.Fatalf("invalid name accepted %q", name)
		}
	}
	if err := lists.Remove("default"); err == nil {
		t.Fatal("default removed")
	}
	if err := lists.Move([]string{"missing"}, "missing"); err == nil {
		t.Fatal("missing list accepted")
	}
	lists = ChangelistState{Active: "missing", Lists: []Changelist{{ID: "default", Name: "Default", Paths: []string{"a", "a"}}, {ID: "duplicate", Name: "Default", Paths: []string{"a"}}}}
	lists.Normalize(domain.Repository{Working: []domain.File{{Path: "a"}}})
	if len(lists.Lists) != 1 || lists.Active != "default" || !reflect.DeepEqual(lists.Lists[0].Paths, []string{"a"}) {
		t.Fatalf("legacy normalization: %+v", lists)
	}
}

func TestChangelistsStagedRenameWithUnstagedEdit(t *testing.T) {
	var state ChangelistState
	state.Normalize(domain.Repository{Working: []domain.File{{Path: "old.go"}}})
	id, err := state.Create("Feature")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Move([]string{"old.go"}, id); err != nil {
		t.Fatal(err)
	}
	state.Normalize(domain.Repository{Working: []domain.File{{Path: "new.go", Status: "M"}}, Staged: []domain.File{{Path: "new.go", OldPath: "old.go", Status: "R"}}})
	if state.Owner("new.go") != id {
		t.Fatal("working side hid staged rename membership")
	}
}

func TestChangelistCommitPolicies(t *testing.T) {
	before := domain.Repository{
		Working: []domain.File{{Path: "partial.go"}, {Path: "untouched.go"}},
		Staged:  []domain.File{{Path: "committed.go"}, {Path: "partial.go"}},
	}
	after := domain.Repository{Working: []domain.File{{Path: "partial.go"}, {Path: "untouched.go"}}}
	var base ChangelistState
	base.Normalize(before)
	feature, err := base.Create("Feature")
	if err != nil {
		t.Fatal(err)
	}
	inactive, err := base.Create("Inactive")
	if err != nil {
		t.Fatal(err)
	}
	if err := base.Move([]string{"committed.go", "partial.go"}, feature); err != nil {
		t.Fatal(err)
	}
	if err := base.Move([]string{"untouched.go"}, inactive); err != nil {
		t.Fatal(err)
	}
	base.Active = "default"

	t.Run("keep remaining", func(t *testing.T) {
		state := CloneChangelists(base)
		if empty := state.ReconcileCommit(before, after); len(empty) != 0 {
			t.Fatalf("empty lists: %v", empty)
		}
		if state.Owner("partial.go") != feature || state.Owner("untouched.go") != inactive {
			t.Fatalf("remaining membership changed: %+v", state)
		}
	})

	t.Run("move remaining to active and remove empty", func(t *testing.T) {
		state := CloneChangelists(base)
		policy := state.Policy()
		policy.KeepRemaining = false
		state.SetPolicy(policy)
		empty := state.ReconcileCommit(before, after)
		if !reflect.DeepEqual(empty, []string{feature}) || state.Owner("partial.go") != "default" || state.Owner("untouched.go") != inactive {
			t.Fatalf("post-commit membership: empty=%v state=%+v", empty, state)
		}
		state.Active = feature
		if err := state.RemoveEmpty(empty); err != nil {
			t.Fatal(err)
		}
		if state.Active != "default" || len(state.Lists) != 2 || state.Owner("partial.go") != "default" {
			t.Fatalf("empty removal: %+v", state)
		}
	})

	t.Run("fully committed list", func(t *testing.T) {
		state := CloneChangelists(base)
		allCommitted := domain.Repository{Working: before.Working, Staged: []domain.File{{Path: "committed.go"}, {Path: "partial.go"}}}
		remaining := domain.Repository{Working: []domain.File{{Path: "untouched.go"}}}
		if empty := state.ReconcileCommit(allCommitted, remaining); !reflect.DeepEqual(empty, []string{feature}) {
			t.Fatalf("empty lists: %v", empty)
		}
	})
}

func TestChangelistEmptyRemovalIsAtomicAndProtectsDefault(t *testing.T) {
	var state ChangelistState
	state.Normalize(domain.Repository{Working: []domain.File{{Path: "remaining.go"}}})
	empty, err := state.Create("Empty")
	if err != nil {
		t.Fatal(err)
	}
	nonempty, err := state.Create("Nonempty")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Move([]string{"remaining.go"}, nonempty); err != nil {
		t.Fatal(err)
	}
	state.Lists[0].Paths = nil
	if err := state.RemoveEmpty([]string{empty, nonempty}); err == nil {
		t.Fatal("nonempty list accepted")
	}
	if state.Owner("remaining.go") != nonempty || len(state.Lists) != 3 {
		t.Fatalf("failed removal mutated state: %+v", state)
	}
	if err := state.RemoveEmpty([]string{"default"}); err == nil {
		t.Fatal("default list accepted")
	}
	if err := state.RemoveEmpty([]string{empty, empty}); err == nil {
		t.Fatal("duplicate list accepted")
	}
	if len(state.Lists) != 3 {
		t.Fatal("invalid removal deleted a list")
	}
}

func TestChangelistPolicyDefaultsAndClone(t *testing.T) {
	var state ChangelistState
	state.Normalize(domain.Repository{})
	if got, want := state.Policy(), DefaultChangelistSettings(); got != want {
		t.Fatalf("legacy defaults: got %+v want %+v", got, want)
	}
	clone := CloneChangelists(state)
	clone.Settings.KeepRemaining = false
	if !state.Policy().KeepRemaining {
		t.Fatal("clone aliases policy")
	}
	invalid := state.Policy()
	invalid.EmptyInactive = "invalid"
	state.SetPolicy(invalid)
	if state.Policy().EmptyInactive != "Show options" {
		t.Fatal("invalid empty-list policy was retained")
	}
}
