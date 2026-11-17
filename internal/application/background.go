package application

import (
	"context"

	"vela/internal/domain"
)

type BackgroundRemotePolicy struct {
	AutoFetch, CheckIncoming bool
	Network                  domain.NetworkPolicy
}

type BackgroundRemoteResult struct {
	Fetched []string
	Skipped []string
	Changed bool
	Message string
}

type BackgroundRepository interface {
	RefreshRepository(context.Context, domain.Repository) (domain.Repository, error)
	CheckRepositoryRemote(context.Context, domain.Repository, BackgroundRemotePolicy) (BackgroundRemoteResult, error)
}
