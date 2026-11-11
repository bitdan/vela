package application

import "context"

// RepositoryInitializer is the application port for creating a local repository.
type RepositoryInitializer interface {
	Initialize(ctx context.Context, directory, branch string) (string, error)
}
