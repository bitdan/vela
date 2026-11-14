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
