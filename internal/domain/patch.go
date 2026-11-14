package domain

// PatchDocument contains a fresh, single-file Git patch, independent of the UI diff.
// Version is an opaque adapter fingerprint and must accompany a write request.
type PatchDocument struct {
	Path, Source, Version, Header string
	Hunks                         []PatchHunk
}

type PatchHunk struct {
	Text                                   string
	OldStart, OldCount, NewStart, NewCount int
}

// PatchExport is a captured Git artifact, not an instruction to modify a tree.
type PatchExport struct {
	Text, Source, Revision string
	Paths                  []string
}
