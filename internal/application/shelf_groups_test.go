package application

import (
	"strings"
	"testing"
)

func TestShelfGroupsLifecycleAndValidation(t *testing.T) {
	var s ShelfGrouping
	id, err := s.Create("  中文 group  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create("中文 GROUP"); err == nil {
		t.Fatal("duplicate accepted")
	}
	for _, name := range []string{"", "  ", "bad\nname", strings.Repeat("字", 81), "Ungrouped", "All Shelves"} {
		if _, err = s.Create(name); err == nil {
			t.Fatalf("invalid name %q accepted", name)
		}
	}
	if err = s.Move("stable-shelf", id); err != nil {
		t.Fatal(err)
	}
	clone := CloneShelfGrouping(s)
	if err = clone.Rename(id, "renamed"); err != nil {
		t.Fatal(err)
	}
	clone.Assignments["stable-shelf"] = ""
	if s.Assignments["stable-shelf"] != id || s.Groups[0].Name != "中文 group" {
		t.Fatal("shallow clone")
	}
	if err = s.Rename(id, "renamed"); err != nil {
		t.Fatal(err)
	}
	s.Filter = id
	if err = s.Delete(id); err != nil {
		t.Fatal(err)
	}
	if len(s.Assignments) != 0 || s.Filter != "ungrouped" {
		t.Fatal("delete did not move entries to default")
	}
	if err = s.Move("stable-shelf", "missing"); err == nil {
		t.Fatal("missing target accepted")
	}
	s.Assignments = map[string]string{"unknown": "missing"}
	if s.Validate() == nil {
		t.Fatal("invalid persisted membership accepted")
	}
}
