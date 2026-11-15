package domain

// RevisionTree contains direct children of an immutable commit directory.
type RevisionTree struct {
	Revision, Directory string
	Entries             []RevisionTreeEntry
}

type RevisionTreeEntry struct {
	Path, Kind, Mode, OID string
	Size                  int64
}

func (e RevisionTreeEntry) Regular() bool {
	return e.Kind == "blob" && (e.Mode == "100644" || e.Mode == "100755")
}
