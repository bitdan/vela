package application

import (
	"context"
	"vela/internal/domain"
)

type RepositoryCommitRewriteReader interface {
	PreviewCommitRewrite(context.Context, domain.Repository, domain.RepositoryOperationKind, []string) (domain.CommitRewritePreview, error)
}
