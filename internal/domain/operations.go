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
	ResetRevision
	SaveStash
	ApplyStash
	DropStash
	CreateWorktree
	RemoveWorktree
	StartBisect
	BisectGood
	BisectBad
	BisectSkip
	ResetBisect
	AddRemote
	EditRemote
	RemoveRemote
	RenameBranch
	DeleteBranch
	CreateTag
	StageHunk
	UnstageHunk
	DiscardHunk
	SaveShelf
	ApplyShelf
	RenameShelf
	DeleteShelf
	RestoreShelf
	PurgeShelf
	CheckoutRevision
	RestoreRevisionFiles
	CherryPickSelected
	RevertSelected
	SquashCommits
	RewordCommit
	FixupCommits
	DropCommit
	UndoCommit
	CheckoutRebase
	CheckoutUpdate
	RenameRemote
	EditRemoteURLs
)

// SequencerState identifies the paused cherry-pick or revert and its recovery metadata.
type SequencerState struct {
	Kind, Commit, Original, Version string
	Remaining                       int
}

func (s SequencerState) Active() bool { return s.Kind != "" }

const MaxSequenceRecovery = 100

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
	FetchURLs, PushURLs            []string
	RewriteSelection               []string
	RewriteVersion                 string
	ConfirmRewrite                 bool
	SquashSelection                []string
	SquashVersion                  string
	ConfirmSquash                  bool
	ShelfHunks                     []ShelfHunkSelection
	Safety                         RepositorySafetyPolicy
	UpdateMethod, UpdateCleanup    string
	ConfirmUpdateCleanup           bool
	Network                        NetworkPolicy
	Paths                          []string
	RemoveApplied                  bool
	SelectShelfFiles               bool
	Kind                           RepositoryOperationKind
	Branch, Source, Remote, Target string
	Checkout, SetUpstream          bool
	Result, WorktreeVersion        string
	Conflict                       MergeConflict
	Mainline                       int
	Mode                           string
	Message                        string
	Path                           string
	BisectVersion                  string
	PatchVersion                   string
	HunkIndex                      int
	SelectPatchLines               bool
	PatchLines                     []int
	IncludeUntracked, RestoreIndex bool
}

// An absent record means whole-file restore; an explicit empty Hunk list is
// invalid and must never expand the selection.
type ShelfHunkSelection struct {
	Path, Version string
	Hunks         []int
}

// NetworkPolicy is copied at dispatch; it never reads mutable UI preferences.
type NetworkPolicy struct {
	FetchTags                               string
	DisableCredentialHelper, NonInteractive bool
}
