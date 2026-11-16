package application

import (
	"context"
	"vela/internal/domain"
)

// ProjectFileManager captures a reviewed disk identity and rechecks it on write.
type ProjectFileManager interface {
	InspectProjectPath(context.Context, domain.Repository, string) (domain.ProjectPathSnapshot, error)
	ExecuteProjectFileOperation(context.Context, domain.Repository, domain.ProjectFileOperation) error
}
