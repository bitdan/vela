package application

import (
	"context"

	"vela/internal/domain"
)

// RepositoryLogReader pages one immutable set of reachable commits.
type RepositoryLogReader interface {
	ReadCommitLog(context.Context, string, domain.CommitLogSnapshot, int, int) (domain.CommitLogPage, error)
}

type RepositoryReader interface {
	ReadRepository(context.Context, string) (domain.Repository, error)
	ReadFile(context.Context, string, string, domain.File) (domain.File, error)
}

type RepositoryRevisionTreeReader interface {
	ReadRevisionTree(context.Context, string, string, string) (domain.RevisionTree, error)
	ReadRevisionChanges(context.Context, string, string, int) ([]domain.File, error)
}

type RepositoryComparisonReader interface {
	ReadMissingBranchComparison(context.Context, domain.Repository, string) (domain.RevisionComparison, error)
	ReadBranchComparison(context.Context, domain.Repository, string, bool) (domain.RevisionComparison, error)
	ReadComparisonCommits(context.Context, string, domain.RevisionComparison, int, int) (domain.CommitLogPage, error)
}

type IndexWriter interface {
	UpdateIndex(context.Context, domain.Repository, bool, []domain.File) error
}

type CommitWriter interface {
	Commit(context.Context, domain.Repository, string, bool, ...CommitPolicy) error
}

// CommitPolicy is copied before dispatch so background work never reads UI state.
type CommitPolicy struct{ Inspections CommitSettings }

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

type RepositoryCloner interface {
	Clone(context.Context, string, string) (string, error)
}

type RepositoryBlameReader interface {
	ReadBlame(context.Context, string, string, string) (domain.BlameDocument, error)
}

type RepositoryFileWriter interface {
	SaveFile(context.Context, domain.Repository, domain.File, string) (domain.File, error)
}

type RepositoryPatchReader interface {
	ReadPatch(context.Context, domain.Repository, string, string) (domain.PatchDocument, error)
}

type RepositoryFileHistoryReader interface {
	ReadFileHistory(ctx context.Context, root, revision, path string, offset, limit int) (domain.FileHistoryPage, error)
}

type RepositoryPatchExporter interface {
	ReadChangesPatch(context.Context, domain.Repository, string, []domain.File) (domain.PatchExport, error)
}

type PatchFileWriter interface {
	SaveNewPatch(context.Context, domain.Repository, string, string) error
}
