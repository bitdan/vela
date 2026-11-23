package gitcli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"vela/internal/domain"
)

// ReadComparisonChanges aggregates a complete single-parent chain as a net
// tree diff. Gaps and merges stay separate, so unselected changes never leak in.
func (g *Executor) ReadComparisonChanges(ctx context.Context, root string, comparison domain.RevisionComparison, selected []string) ([]domain.ComparisonChangeGroup, error) {
	ctx, cancel := context.WithTimeout(canonicalHistory(ctx), 30*time.Second)
	defer cancel()
	if !validObjectID(comparison.Left) || !validObjectID(comparison.Right) || len(selected) == 0 || len(selected) > 1000 {
		return nil, errors.New("select between 1 and 1000 commits in a fixed comparison")
	}
	wanted := make(map[string]bool, len(selected))
	for _, oid := range selected {
		if !validObjectID(oid) || wanted[oid] {
			return nil, errors.New("invalid or duplicate selected commit")
		}
		wanted[oid] = true
	}
	common, err := g.read(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	common = strings.TrimSuffix(common, "\n")
	if err := checkLogBoundary(common, comparison.AncestryBoundary); err != nil {
		return nil, err
	}
	// Obtain real parent identities and membership together, without trusting UI
	// metadata or mutable refs. No path filter: it can rewrite ancestry.
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 16<<20, "rev-list", "--topo-order", "--parents", "--max-count=100001", comparison.Left, "^"+comparison.Right, "--")
	if err != nil {
		return nil, err
	}
	lines := strings.FieldsFunc(out.stdout, func(r rune) bool { return r == '\n' })
	if len(lines) > 100000 {
		return nil, errors.New("comparison selection exceeds 100000 commits")
	}
	var commits [][]string
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) > 0 && wanted[fields[0]] {
			commits = append(commits, fields)
		}
	}
	if len(commits) != len(wanted) {
		return nil, errors.New("selected commit is outside the fixed comparison range")
	}
	chain := len(commits) > 1
	for i, c := range commits {
		if len(c) > 2 || i+1 < len(commits) && (len(c) != 2 || c[1] != commits[i+1][0]) {
			chain = false
		}
	}
	var groups []domain.ComparisonChangeGroup
	if chain {
		oldest := commits[len(commits)-1]
		left, right := "", commits[0][0]
		if len(oldest) == 2 {
			left = oldest[1]
		} else {
			// Hashing without -w computes the format-specific empty tree ID and
			// leaves the object database untouched.
			left, err = g.read(ctx, root, "hash-object", "-t", "tree", "--stdin")
			if err != nil {
				return nil, err
			}
			left = strings.TrimSpace(left)
			if !validObjectID(left) {
				return nil, errors.New("invalid empty tree identity")
			}
		}
		files, err := g.readComparisonTreeChanges(ctx, root, left, right)
		if err != nil {
			return nil, err
		}
		groups = append(groups, domain.ComparisonChangeGroup{Title: fmt.Sprintf("Net changes · %d commits · %s → %s", len(commits), left[:8], right[:8]), Source: (domain.RevisionComparison{Left: left, Right: right}).Source(), Files: files})
	} else {
		total := 0
		for _, commit := range commits {
			count := max(1, len(commit)-1)
			for parent := 1; parent <= count; parent++ {
				mainline := 0
				if len(commit) > 2 {
					mainline = parent
				}
				files, err := g.ReadRevisionChanges(ctx, root, commit[0], mainline)
				if err != nil {
					return nil, err
				}
				total += len(files)
				if total > 10000 {
					return nil, errors.New("selection exceeds 10000 changed files")
				}
				source, title := commit[0], commit[0][:8]
				if len(commit) > 1 {
					source = (domain.RevisionComparison{Left: commit[parent], Right: commit[0]}).Source()
				}
				if len(commit) > 2 {
					title += fmt.Sprintf(" · Parent %d · %s", parent, commit[parent][:8])
				}
				groups = append(groups, domain.ComparisonChangeGroup{Title: title, Source: source, Files: files})
			}
		}
	}
	if err := checkLogBoundary(common, comparison.AncestryBoundary); err != nil {
		return nil, err
	}
	return groups, nil
}

func (g *Executor) readComparisonTreeChanges(ctx context.Context, root, left, right string) ([]domain.File, error) {
	out, err := g.runInputWithEnvLimit(ctx, root, "", nil, 2<<20, "diff", "--name-status", "-z", "--no-ext-diff", "--no-textconv", "--no-color", "--no-relative", "--find-renames=50%", left, right, "--")
	if err != nil {
		return nil, err
	}
	return parseComparisonChanges(out.stdout)
}
