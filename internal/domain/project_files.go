package domain

// ProjectPathSnapshot identifies the on-disk tree reviewed before a file action.
type ProjectPathSnapshot struct {
	Path, Version string
	Directory     bool
}

type ProjectFileAction string

const (
	CreateProjectFile      ProjectFileAction = "create-file"
	CreateProjectDirectory ProjectFileAction = "create-directory"
	MoveProjectPath        ProjectFileAction = "move"
	CopyProjectPath        ProjectFileAction = "copy"
	TrashProjectPath       ProjectFileAction = "trash"
	IgnoreProjectPath      ProjectFileAction = "ignore"
)

// ProjectFileOperation changes worktree files only, never HEAD or the Index.
type ProjectFileOperation struct {
	Action      ProjectFileAction
	Source      ProjectPathSnapshot
	Destination string
}
