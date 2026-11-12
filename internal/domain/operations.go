package domain

import "errors"

// ErrRevertUnchanged reports that Git declined to create a revert commit and
// left HEAD, the index and tracked files unchanged without a recovery state.
var ErrRevertUnchanged = errors.New("revert created no commit; HEAD, index and tracked files are unchanged")

type RepositoryOperationKind uint8

const (
	CreateBranch RepositoryOperationKind = iota + 1
	CheckoutBranch
	FetchRemote
	PullBranch
	PushBranch
	UpdateTrackedBranch
	StartMerge
	ContinueMerge
	AbortMerge
	AcceptCurrent
	AcceptIncoming
	MarkResolved
	SaveMergeResult
	StartRebase
	ContinueRebase
	SkipRebase
	AbortRebase
	StartCherryPick
	StartRevert
	ContinueSequence
	SkipSequence
	AbortSequence
)

// SequencerState identifies the paused cherry-pick or revert and its recovery metadata.
type SequencerState struct {
	Kind, Commit, Original, Version string
	Remaining                       int
}

func (s SequencerState) Active() bool { return s.Kind != "" }

type RebaseState struct {
	Branch, Original, Onto, Stopped, Step string
}

func (r RebaseState) Active() bool { return r.Original != "" }

type MergeConflict struct {
	Path, Base, Current, Incoming string
	Submodule                     bool
}

type Upstream struct {
	Remote, Branch string
}

type RepositoryOperation struct {
	Kind                           RepositoryOperationKind
	Branch, Source, Remote, Target string
	Checkout, SetUpstream          bool
	Result, WorktreeVersion        string
	Conflict                       MergeConflict
	Mainline                       int
}
