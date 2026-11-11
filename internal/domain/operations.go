package domain

type RepositoryOperationKind uint8

const (
	CreateBranch RepositoryOperationKind = iota + 1
	CheckoutBranch
	FetchRemote
	PullBranch
	PushBranch
	UpdateTrackedBranch
)

type Upstream struct {
	Remote, Branch string
}

type RepositoryOperation struct {
	Kind                           RepositoryOperationKind
	Branch, Source, Remote, Target string
	Checkout, SetUpstream          bool
}
