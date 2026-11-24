package application

import (
	"testing"
	"vela/internal/domain"
)

func TestShelfHunkSelectionTotalLimit(t *testing.T) {
	indices := make([]int, 5001)
	for i := range indices {
		indices[i] = i
	}
	selections := []domain.ShelfHunkSelection{{Path: "a.txt", Version: "a", Hunks: indices}, {Path: "b.txt", Version: "b", Hunks: indices}}
	if err := ValidateShelfHunks([]string{"a.txt", "b.txt"}, selections); err == nil {
		t.Fatal("multiple paths bypassed total Hunk limit")
	}
	selections[1].Hunks = indices[:4999]
	if err := ValidateShelfHunks([]string{"a.txt", "b.txt"}, selections); err != nil {
		t.Fatal(err)
	}
	for _, s := range []domain.ShelfHunkSelection{{Path: "a.txt", Version: "v"}, {Path: "a.txt", Version: "v", Hunks: []int{0, 0}}, {Path: "missing", Version: "v", Hunks: []int{0}}, {Path: "a.txt", Hunks: []int{0}}} {
		if err := ValidateShelfHunks([]string{"a.txt"}, []domain.ShelfHunkSelection{s}); err == nil {
			t.Fatal("unsafe Hunk selection accepted")
		}
	}
}
