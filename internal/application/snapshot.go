package application

// WorkspaceSnapshot stores stable identities and view positions for mock tabs.
type WorkspaceSnapshot struct {
	ActiveRepository string
	Repositories     []RepositorySnapshot
}

type RepositorySnapshot struct {
	Name      string
	ActiveTab string
	Tabs      []TabSnapshot
}

type TabSnapshot struct {
	Source, Path string
	Code, Pinned bool
	Cursor       int
	ScrollY      float64
}
