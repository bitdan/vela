package domain

// PatchDocument contains a fresh, single-file Git patch, independent of the UI diff.
// Version is an opaque adapter fingerprint and must accompany a write request.
type PatchDocument struct {
	Path, Source, Version, Header string
	// Kind is A/D/R/B/P for atomic imported operations. Empty denotes a text
	// modification with selectable hunks. Decoded binary bytes never enter editor buffers.
	Kind, OldPath, Payload string
	OldMode, NewMode       string
	Binary                 bool
	OldSize, NewSize       int
	Hunks                  []PatchHunk
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
