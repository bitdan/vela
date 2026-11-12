package application

import (
	"context"

	"vela/internal/domain"
)

type RepositoryReader interface {
	ReadRepository(context.Context, string) (domain.Repository, error)
	ReadFile(context.Context, string, string, domain.File) (domain.File, error)
}

type IndexWriter interface {
	UpdateIndex(context.Context, domain.Repository, bool, []domain.File) error
}

type CommitWriter interface {
	Commit(context.Context, domain.Repository, string, bool) error
}

type RepositoryOperator interface {
	ExecuteRepositoryOperation(context.Context, domain.Repository, domain.RepositoryOperation) error
}

type MergeDocumentReader interface {
	ReadMergeDocument(context.Context, domain.Repository, string) (domain.MergeDocument, error)
}

// RepositoryInitializer is the application port for creating a local repository.
type RepositoryInitializer interface {
	Initialize(ctx context.Context, directory, branch string) (string, error)
}
