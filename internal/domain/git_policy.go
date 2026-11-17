package domain

import (
	"fmt"
	"path"
	"strings"
)

// RepositorySafetyPolicy is an immutable copy of application preferences.
type RepositorySafetyPolicy struct {
	ProtectedBranches string
	CherryPickSuffix  bool
}

// ProtectedBranch uses semicolon-separated, case-sensitive path.Match globs.
// A slash separates branch components; release/* matches release/v1.
func ProtectedBranch(patterns, branch string) (bool, error) {
	if len(patterns) > 8192 {
		return false, fmt.Errorf("protected branch patterns exceed 8192 bytes")
	}
	parts := strings.Split(patterns, ";")
	if len(parts) > 128 {
		return false, fmt.Errorf("use at most 128 protected branch patterns")
	}
	matched := false
	for _, pattern := range parts {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if len(pattern) > 256 || strings.ContainsAny(pattern, "\x00\r\n") {
			return false, fmt.Errorf("invalid protected branch pattern %q", pattern)
		}
		ok, err := path.Match(pattern, branch)
		if err != nil {
			return false, fmt.Errorf("invalid protected branch pattern %q: %w", pattern, err)
		}
		matched = matched || ok
	}
	return matched, nil
}

// CheckOperation protects local history while allowing ordinary commits,
// merges and non-forced pushes. Recovery commands remain reachable.
func (p RepositorySafetyPolicy) CheckOperation(repo Repository, op RepositoryOperation) error {
	branch := repo.CurrentBranch
	switch op.Kind {
	case RenameBranch, DeleteBranch:
		branch = strings.TrimPrefix(op.Source, "refs/heads/")
	case ResetRevision, StartRebase:
	case UpdateTrackedBranch:
		if op.UpdateMethod != "Rebase" {
			return nil
		}
		branch = op.Branch
	default:
		return nil
	}
	protected, err := ProtectedBranch(p.ProtectedBranches, branch)
	if err != nil {
		return err
	}
	if protected {
		return fmt.Errorf("branch %q is protected; change Protected branches in Settings before rewriting, renaming or deleting it", branch)
	}
	return nil
}
