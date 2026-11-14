package domain

// FileHistoryPage is a bounded slice of history at a fixed starting commit.
// Each commit retains the file's path at that revision, including renames.
type FileHistoryPage struct {
	Commits []Commit
	HasMore bool
}
