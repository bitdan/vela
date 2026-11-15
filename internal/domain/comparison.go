package domain

import "strings"

// RevisionComparison fixes two commit identities. Empty Right identifies tracked
// working files guarded by WorktreeVersion; changed content must be queried again.
type RevisionComparison struct {
	Left, Right                       string
	WorktreeVersion, AncestryBoundary string
	Files                             []File
}

func (c RevisionComparison) Source() string {
	right := c.Right
	if right == "" {
		right = "working"
		if c.WorktreeVersion != "" {
			right += "@" + c.WorktreeVersion
		}
	}
	return "compare:" + c.Left + ":" + right
}

func ParseComparisonSource(source string) (left, right string, ok bool) {
	value, yes := strings.CutPrefix(source, "compare:")
	if !yes {
		return "", "", false
	}
	left, right, ok = strings.Cut(value, ":")
	valid := func(id string) bool {
		return (len(id) == 40 || len(id) == 64) && strings.Trim(id, "0123456789abcdef") == ""
	}
	kind, version, versioned := strings.Cut(right, "@")
	working := right == "working" || versioned && kind == "working" && len(version) == 64 && valid(version)
	if !ok || !valid(left) || !working && !valid(right) {
		return "", "", false
	}
	if working {
		right = ""
	}
	return left, right, true
}

func ComparisonWorktreeVersion(source string) string {
	if _, _, ok := ParseComparisonSource(source); !ok {
		return ""
	}
	_, version, _ := strings.Cut(source, "@")
	return version
}
