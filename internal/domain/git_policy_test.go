package domain

import "testing"

func TestProtectedResetRecoveryKeepsHead(t *testing.T) {
	p := RepositorySafetyPolicy{ProtectedBranches: "main;master"}
	r := Repository{CurrentBranch: "main", Head: "fixed-head"}
	for _, tc := range []struct {
		source, mode string
		allow        bool
	}{
		{"fixed-head", "Mixed", true}, {"fixed-head", "Hard", true},
		{"older-head", "Mixed", false}, {"older-head", "Hard", false},
		{"fixed-head", "Soft", false}, {"main", "Hard", false}, {"", "Mixed", false},
	} {
		err := p.CheckOperation(r, RepositoryOperation{Kind: ResetRevision, Source: tc.source, Mode: tc.mode})
		if (err == nil) != tc.allow {
			t.Fatalf("reset %s %s: %v; allow=%t", tc.source, tc.mode, err, tc.allow)
		}
	}
}
