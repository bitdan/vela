package domain

// SquashPreview binds the selected range and every rewritten descendant to a
// fresh repository/ref identity. Git objects remain immutable; Version is opaque.
type SquashPreview struct {
	Version, Parent, HeadTree, SelectedTree  string
	Author, AuthorEmail, AuthorDate, Message string
	Selected, Replay                         []string
}
