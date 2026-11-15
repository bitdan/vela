package domain

// CommitLogSnapshot fixes the reachable commit set and labels for every page.
// Tips contain commit OIDs, never mutable ref names. Treat snapshots as immutable.
type CommitLogSnapshot struct {
	Tips     []string
	Refs     map[string][]string
	Boundary string
}

type CommitLogPage struct {
	Commits []Commit
	HasMore bool
}

// ReconnectCommitLog rebuilds loaded graph edges after appending a page. Original
// parent OIDs remain available even when their commits have not been loaded yet.
// The caller owns the slice and must not mutate it concurrently with a reader.
func ReconnectCommitLog(commits []Commit) {
	indices := make(map[string]int, len(commits))
	for i := range commits {
		indices[commits[i].Hash] = i
		commits[i].Parents = nil
	}
	for i := range commits {
		for _, hash := range commits[i].ParentHashes {
			if parent, ok := indices[hash]; ok && parent > i {
				commits[i].Parents = append(commits[i].Parents, parent)
			}
		}
	}
	var lanes []int
	pending := make(map[int]int)
	reserve := func(target int) int {
		for i, value := range lanes {
			if value == -1 {
				lanes[i] = target
				return i
			}
		}
		lanes = append(lanes, target)
		return len(lanes) - 1
	}
	for i := range commits {
		lane, ok := pending[i]
		if !ok {
			lane = reserve(i)
		}
		commits[i].Lane = lane
		lanes[lane] = -1
		delete(pending, i)
		for _, parent := range commits[i].Parents {
			if _, ok := pending[parent]; !ok {
				pending[parent] = reserve(parent)
			}
		}
	}
}
