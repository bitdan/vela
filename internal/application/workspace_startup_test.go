package application

import "testing"

func TestProductionWorkspaceDoesNotAllocateDemoRepositories(t *testing.T) {
	a := NewWorkspace()
	if !a.Production || len(a.Sessions) != 1 || a.Active().Repository.Name != "" || a.Active().Repository.Root != "" || len(a.Active().Repository.History) != 0 {
		t.Fatal("normal startup contains demo data")
	}
	demo := NewDemo()
	if demo.Production || len(demo.Sessions) != 2 || len(demo.Active().Repository.History) == 0 {
		t.Fatal("explicit demo unavailable")
	}
}
