# ADR 0004: Durable shelves and local changelist metadata

Date 2026-11-14. Implemented under explicit Shelf/Changelist feature authorization. Native desktop acceptance remains separate.

Shelves use private Git trees/commits and compare-and-swap refs scoped to each GitDir (linked worktrees have separate scopes). The shelf commit has base HEAD and a selected-index commit as parents, allowing Git's documented stash-compatible merge machinery to restore original stage/worktree distinction. The object is published before cleanup. refs/stash and its reflog are not used or changed. Binary payloads live in Git objects, not workspace JSON. Normal push of a branch does not publish these refs; broad --mirror pushes may include private refs. Private refs retain objects through GC until permanent deletion; copying only workspace.json does not back up shelves. Log snapshots explicitly enumerate heads/remotes/tags and exclude private shelves.

Rename creates a new private commit with the same trees/parents and changes the ref with expected OID. Soft-delete and restore move refs transactionally; permanent deletion requires explicit confirmation and drops the ref. Retained on cancellation, application failure and conflict. An optional successful-apply cleanup moves the entire applied entry to trash. No partial-file apply in the first version.

The working snapshot starts from the selected-index tree, preserving staged mode changes with core.filemode=false and forced ignored additions. Limits: 100 paths, 16 KiB path arguments, 16 MiB per working file and 64 MiB total, 200 entries including trash. UI hides the extra changelist controls row during Bisect/conflicts to keep action controls visible; the same registry commands remain available through View Options.

Changelists are optional Application session JSON metadata: stable IDs, validated names, canonical changed-file paths and active ID. Default is reserved; files belong to exactly one list. Changes to Git index/working tree are independent. Snapshots and restoration deep-clone member slices; old sessions get Default. Refresh reconciles changed paths and renames. This is whole-file grouping; a setting that previously suggested split hunks must not claim support. All staged changes remain visible and the existing commit operation uses the entire Index.

Alternatives rejected: ordinary stash aliasing, push/drop-based stash transfer, storing unversioned patch payloads in workspace JSON, SQLite or a new generic manager. No new dependency, persistence service or write-lock scheme. Rolling back ignores changelist JSON fields (old saves lose them) and leaves private refs accessible through Git. Removing refs intentionally permits later GC; no automatic destructive migration. Documentation remains local per user.

References: https://git-scm.com/docs/git-stash (stash-like commit ancestry and apply), https://git-scm.com/docs/git-commit-tree, https://git-scm.com/docs/git-update-index, https://git-scm.com/docs/git-update-ref .


## 2026-11-18: selective whole-file restoration

Unshelve now resolves explicit path selections against the captured/current shelf identity. Empty explicit selections fail; legacy omitted paths still mean the complete entry. A temporary index overlays only selected saved Index and Worktree entries onto the original Base, producing a stash-compatible transient commit. Both complete Base-to-projected-tree path sets must be subsets of the selection: update-index --index-info permits file/directory replacement, so checking only queried tree rows would allow unchecked deletions. No live Index or shelf ref changes during preparation.

Partial application always retains the entire original entry and rejects RemoveApplied. Full successful application may retain existing CAS move-to-trash behavior. Rename halves remain independent saved paths. Original object reads ignore replacement objects, and metadata/ref/checkout/worktree identities are rechecked before writing. Target-side renames/conflicts still use Git merge semantics; all open project editors must be saved and stay read-only until the callback, then all mutable root views refresh. No new persistence format or dependency. Function/integration/native validation remains UNVERIFIED per user request.
