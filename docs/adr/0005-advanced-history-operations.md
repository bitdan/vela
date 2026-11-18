# ADR 0005: Immutable revision browsing and selected history operations

2026-11-15. Base 9b58e1f. User authorizes advanced history implementation in the existing native application; Gate 2 desktop acceptance remains separate. Documentation stays local.

## Decision

Use the existing system Git Executor/RepositoryOperator and serialized write lock. Add the narrow RepositoryRevisionTreeReader for ls-tree direct children and explicit-parent file comparison, Domain-only DTOs, Bootstrap goroutines and RunOnMain callbacks. Directory and comparison output is bounded to 2 MiB / 10,000 rows; UI virtualizes and cancels stale requests. Transient dialog state is not persisted. Opened full-OID version tabs reuse existing identities and session restoration.

Checkout Revision is explicit Detached HEAD checkout, never branch reset or force. Restore Revision Files writes only the selected existing regular snapshot files to the worktree, preserving HEAD/Index. Clean tracked state, worktree/checkout versions, index flags, filters, filesystem parents and untracked/ignored collisions are checked. Windows selections reject case/trim aliases and aliases against distinct current tracked names; checkout validates target tree collision keys (bounded 16 MiB tree-name output). No silent rollback or overwrite retry on failure.

Selected Cherry-pick/Revert uses a separate temporary GIT_INDEX_FILE. Start at the selected source parent (or empty tree for a root commit), replace only selected paths with source versions, include both rename sides, write-tree and commit-tree. Keep original author/date and message provenance, then use existing Git sequence with this immutable internal object. Merge parents are chosen explicitly; UI reloads that exact parent's changed-file list, including parent-2-only paths and merges with empty first-parent diffs. New commits/conflicts use normal Git machinery; original user refs are unchanged. Revert's synthetic subject includes original OID because Git-generated revert messages quote the subject but omit its body. Internal objects are unreferenced after successful application and may be collected; paused sequencers use normal Git recovery metadata. No new persistent ref or session schema.

Target-side renames may write paths outside original source selection. Guard every dirty project buffer for selected sequencer starts, refresh all live mutable file views after operations and continuation/skip/abort, and preserve edits made after background dispatch. Success refresh invalidates mutable previews; immutable historical tabs remain intact.

Canonical historical contexts set GIT_NO_REPLACE_OBJECTS=1 in the Executor; historical readers/writes and file previews opt in. This matches the existing Log snapshot policy. Otherwise refs/replace could change the content/parents of a supposedly fixed OID. No global mutable environment or shell interpolation is used.

## Alternatives

Whole-tree checkout to browse would mutate user's project; flat eager recursive tree would scale poorly. Direct git apply --3way of a selected patch would not provide Git's sequencer abort/continue state. Full original cherry-pick followed by undoing unselected files would risk unrelated changes and conflicts. Temporary synthetic objects preserve Git merge semantics without those paths. No new dependency or generic manager.

Rollback leaves existing version tabs readable and requires no migration. The feature is whole-file only, requires a local committed repository and clean tracked tree, and deliberately rejects unsupported links/submodules/custom filters/hidden flags. Deleted-at-source restore requires selecting an earlier commit. OS-level failure/cancellation can leave partial worktree changes; report inspection rather than invent a safe automatic reversal.

References: [ls-tree](https://git-scm.com/docs/git-ls-tree), [restore](https://git-scm.com/docs/git-restore), [commit-tree](https://git-scm.com/docs/git-commit-tree), [checkout](https://git-scm.com/docs/git-checkout), [cherry-pick](https://git-scm.com/docs/git-cherry-pick), [revert](https://git-scm.com/docs/git-revert).
