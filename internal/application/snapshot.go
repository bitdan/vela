package application

import "vela/internal/domain"

// WorkspaceSnapshot stores identities and view positions, never cached Git data.
type WorkspaceSnapshot struct {
	ActiveRepository string
	ActiveRoot       string `json:",omitempty"`
	Repositories     []RepositorySnapshot
}

type RepositorySnapshot struct {
	Changelists                       *ChangelistState `json:",omitempty"`
	WorkingSelection, StagedSelection []string         `json:",omitempty"`
	Name                              string
	Root                              string `json:",omitempty"`
	Message                           string `json:",omitempty"`
	Working                           string `json:",omitempty"`
	Staged                            string `json:",omitempty"`
	Branch                            string `json:",omitempty"`
	Commit                            string `json:",omitempty"`
	Historical                        string `json:",omitempty"`
	ActiveTab                         string
	Tabs                              []TabSnapshot
}

type TabSnapshot struct {
	Comparison   *ComparisonSnapshot `json:",omitempty"`
	Source, Path string
	ChangeKind   string `json:",omitempty"`
	OldPath      string `json:",omitempty"`
	Code, Pinned bool
	Cursor       int
	ScrollY      float64
	Unified      bool   `json:",omitempty"`
	Bookmarks    []int  `json:",omitempty"`
	FileType     string `json:",omitempty"`
}

// ComparisonSnapshot stores object identities and view state, never Git data.
type ComparisonSnapshot struct {
	Ref, Label, RightLabel, Left, Right, Boundary string
	Split                                         float32
	Sides                                         [2]ComparisonSideSnapshot
}

type ComparisonSideSnapshot struct {
	Filter, Draft                  domain.ComparisonLogFilter
	Selected                       []string
	ActiveCommit, File, FileSource string
	Loaded                         int
	Split, DetailSplit             float32
	ClosedGroups                   []string
}
