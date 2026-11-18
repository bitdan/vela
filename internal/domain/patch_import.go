package domain

// ImportedPatch contains the exact bounded input and a target-specific preview
// fingerprint. Version is issued by the adapter after repository preflight.
type ImportedPatch struct {
	Text, Target, Version string
	Directory             string
	Strip                 int
	Reverse               bool
	Files                 []PatchDocument
	Paths                 []string
	Modes                 map[string]string
	Hunks                 int
}
