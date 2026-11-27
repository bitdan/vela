package domain

// LineHistoryQuery binds a Project range to exact committed and disk bytes.
type LineHistoryQuery struct {
	Revision, Path, Text, DiskVersion string
	Start, End, Offset, Limit         int
}
type LineHistoryHunk struct {
	OldStart, NewStart int
	Before, After      string
}
type LineHistoryEntry struct {
	Commit Commit
	Hunks  []LineHistoryHunk
}
type LineHistoryPage struct {
	Entries []LineHistoryEntry
	HasMore bool
}
