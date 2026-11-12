package domain

// StashEntry keeps the immutable commit separate from the mutable reflog selector.
type StashEntry struct{ OID, Selector, Subject string }

// Worktree describes a checkout sharing refs and objects with its common repository.
type Worktree struct {
	Path, Head, Branch               string
	Bare, Detached, Locked, Prunable bool
}

// BisectState records a per-worktree search independently of the checked-out HEAD.
type BisectState struct {
	Original, OriginalHead, Candidate, Bad, FirstBad, Version string
	Good                                                      []string
	NoCheckout, Supported, Ambiguous                          bool
}

func (b BisectState) Active() bool { return b.Version != "" }
