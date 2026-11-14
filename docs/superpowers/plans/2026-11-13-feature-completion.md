# Feature completion implementation plan

> **For agentic workers:** Use superpowers:executing-plans to implement each batch with failing tests, implementation, targeted verification and a final independent review.

**Goal:** Reduce the verified functionality gaps in the user's inventory within a maximum five-hour work session starting 2026-11-13 15:36 UTC (deadline 20:36 UTC).

**Architecture:** Retain Go 1.27.3+, MyGo v0.8.3 and the existing native layout. UI invokes Application ports; Git operations remain in the unified adapter with context and stale-state verification. Extend JSON session persistence additively and document compatibility in an ADR.

**Spec:** VELA_SPEC.md, ARCHITECTURE.md and the user's authorization to continuously implement the inventory. No commit, push or publication. Existing uncommitted layout restoration changes are preserved. Work in the existing checkout to retain those changes; no worktree migration.

**Execution note:** Tasks 1–13 were implemented in successive batches. On session resumption after the original wall-clock cutoff, no new batch was started; remaining work is limited to verification, necessary fixes and delivery documentation. Final results and remaining gaps are recorded in `docs/feature-completion-report.md`; the plan is not itself acceptance evidence. The initial read-only source boundary below was expanded explicitly by Task 8; unsaved-content persistence remains excluded.

## Design and alternatives

Persist repository identities and view state, not Git data. Reopen the previously active real repository asynchronously once ports are connected. Keep other repositories as recent entries, loading on explicit selection. Restored tabs reread content through RepositoryReader; failed or stale requests cannot switch the current repository. Missing repositories remain retryable and never block startup. This avoids eager loading every recent repository and avoids persisting stale Git snapshots. Existing Mock JSON remains readable. Commit drafts and independent selections belong to each repository; buffers remain read-only for real source tabs. Merge drafts and ordinary editable-file persistence are outside this batch.

Subsequent batches extend existing Operation routing for real remote editing, safe branch rename/delete and lightweight tag creation, where time permits. No forced deletion or history overwrite. Native confirmations bind to the opened repository snapshot. UI validation is complemented by adapter tests on temporary repositories.

## Review focus

- Identical repository display names at different absolute paths must not merge sessions.
- Missing repositories or files must not block startup or erase retryable saved state.
- User navigation during asynchronous restore must win over late callbacks.
- Old JSON without new fields must still load and restore Mock tabs.
- Ref/config mutation after confirmation must be detected before writes; cancellation must retain original data.

## Task 1: Real repository sessions and recent repositories

Files: application/snapshot.go, ui/snapshot.go, new ui/session_restore.go and ui/session_restore_test.go, ui/repository_read.go, ui/workspace.go, ui/commands.go, bootstrap/run.go, adapters/session.go and tests; docs/adr session format decision.

- [ ] Add failing tests for real snapshot identities, roundtrip drafts/selections, async active repository/tab recovery, missing paths, stale restoration and legacy Mock JSON.
- [ ] Extend snapshots with additive Root/ActiveRoot and per-repository draft/selection fields. Restore only validated file identities using async read ports and context cancellation. Persist recent entries separately from loaded sessions.
- [ ] Add repository-selector recent entries with full-path labels and shared command routing. Start restoration after Bootstrap connects reader ports.
- [ ] Run `go test ./internal/ui ./internal/adapters ./internal/application -count=1`, relevant vet and Windows build. Export Dark/Light Tester screenshots at 1440x900. Expected PASS; manual desktop/IDEA confirmation remains UNVERIFIED.

## Task 2: Real remote management

Files: domain/operations.go, adapters/gitcli/remote.go and remote tests, ui/remotes.go and related tests, operation routing.

- [ ] Write failing temporary-repository tests for add/edit/remove, stale config and invalid names; UI tests for confirmation/cancel and refresh isolation.
- [ ] Add operations with existing serialized executor, snapshot checks and explicit confirmation. Never execute Mock-only mutation for real repositories.
- [ ] Run targeted Git/UI tests and vet; capture dual-theme dialog evidence.

## Task 3: Safe branch and tag management

Files: existing domain operation model, new adapter refs module/tests, UI operation dialogs/tests and command routing.

- [ ] Write failing tests for rename/delete/create-tag, existing refs, checked-out worktrees, stale snapshots and invalid input.
- [ ] Implement non-force operations, explicit target/impact confirmation and shared command registry entries.
- [ ] Run targeted integration/UI tests, vet and Windows build; document limitations.

## Task 4: Verification and delivery

- [ ] Reconcile remaining time before opening another batch. Prefer fully tested work over partially wired menus.
- [ ] Update current.md, product/architecture specifications, acceptance table and remaining inventory.
- [ ] Review the complete change independently, fix important findings with regression tests, and leave all files uncommitted.

## Task 5: Real commit inspections

Files: application/repository.go, gitcli/commit.go plus commit policy tests, bootstrap/run.go, ui/commit_actions.go, settings.go and UI tests.

- [ ] Prove enabled subject/body/blank-line inspections currently do not prevent real commits, and new staged TODO inspection is ignored.
- [ ] Add optional immutable CommitPolicy to the application writer port, preserving existing adapter callers via variadic options. Capture policy on the UI thread before background execution. Recheck policy inside the serialized commit operation after checkout validation. Only inspect added staged lines for TODO; never inspect unstaged text.
- [ ] Add UI disabled reasons for invalid messages; communicate that TODO checks apply to staged additions and other IDE inspections remain unavailable. Keep the Mock-only clear-after-preview preference's existing semantics.
- [ ] Run targeted real temporary-repository and UI tests, vet and build. Expected PASS; no real development repository commits.

## Execution ledger

- 2026-11-13: User authorized continuous implementation up to five hours. No new Git business beyond the inventory is inferred. Gate 2 user signature remains PARTIAL; implementation authorization does not assert acceptance.
- Task 1: initial UI/adapters/application baseline PASS (73 tests); expanded suite PASS (92 tests), targeted vet and Windows build PASS. Independent review found three P2 issues (staged source metadata, tab navigation and pending active identity); regression tests observed RED then GREEN for all three. Additional complete UI regression is running with Tasks 2/3.
- Tasks 2/3: real remote management and non-force branch/tag adapter and UI tests observed RED then GREEN. Remote removal explicitly rejects inherited config and custom fetch mappings rather than guessing their deletion impact. Commit/push steps omitted per session authorization boundary.

## Task 6: Clone repository

Add Application.RepositoryCloner, gitcli/clone.go with temporary bare remote integration tests, and native ui/repository_clone.go with Tester coverage. Use credential-free supported URL or absolute local source and a new absolute target directory with an existing parent outside a Git repository. Refuse any existing target, nested repository and unsupported remote helper; create no recursive submodules or template hooks. Execute via the existing background Bootstrap and unified Executor with cancellable Context. Failures/cancellation never trigger application-level recursive cleanup. Show destination and result and open only a successful clone if user navigation has not superseded the request. Connect the existing Clone menu and Actions command. Test success, existing files, nested targets, invalid source, cancellation, UI disable/busy/confirmation/error/navigation behavior; dual themes at 1440x900.

## Task 7: Read-only Blame

Add domain.BlameDocument/BlameLine, application.RepositoryBlameReader and an adapter parser for Git --line-porcelain. Query a fixed committed OID and literal path, preserving author/date/full commit identities and source text; never attribute unstaged changes. Bound text to existing 2MiB previews and 20,000 lines. The UI formats attribution into a read-only native editor tab with explicit Blame context, shares file-read cancellation guards, and persists source/path identity for requery on restore. Add Show Blame to project/change/history/editor menus. Tests cover multiple authors/commits, rename, Unicode/spaces/newline path, unsupported binary/untracked input, cancel, parsing malformed output and UI stale callbacks, read-only tabs and session identity. No Git writes.

## Task 8: Edit and save real source files

Add DiskVersion to domain.File and an Application.RepositoryFileWriter port implemented under the existing adapter write lock. Only regular UTF-8 project files with a captured disk fingerprint become editable when the writer is connected; revision/Blame/Diff/symlink views remain read-only. Save via temp file in the same directory and atomic replacement, retaining file mode and original line endings. Revalidate root/HEAD/Index and raw file fingerprint, reject symlink traversal, read-only or missing targets, and reread immediately before replacement. Cancellation/error keeps original disk data and editor modifications. Capture submitted buffer state; edits made during background save remain modified. Refresh only the original repository, preserve split-buffer identity, do not overwrite dirty tabs on reopen/refresh, and expose Save through unified command, toolbar/context menu, Actions and Ctrl+S. Tests use temporary repositories, external modifications and cancellation, and MyGo Tester edits/shortcuts/dirty-close protection. Unsaved content remains in memory and needs explicit discard on exit; no claim of draft file-content persistence.

## Task 9: Stage / Unstage / Discard Git hunks

Add an independent Application Patch parser/model and RepositoryPatchReader. Generate actual Git unified patches for a single tracked regular UTF-8 modified file, retaining quoted paths and no-final-newline markers. Native Diff toolbar/menu opens an async Hunk dialog, which selects and previews Git hunks independently of MyGo's visual diff algorithm. Stage applies to Index, Unstage reverses Index, Discard reverses working file after an explicit destructive acknowledgement. Operations pass only path, source, hunk index and version; adapter regenerates/version-checks the patch under its existing write lock, preflights with git apply --check and applies without --reject or force. Context/request guards, original repository refresh and cancellation use existing mechanisms. Reject additions/deletions/renames/copies, binary/submodule/symlink/mode changes initially; Discard additionally rejects CRLF/filters/encoding transforms and unsaved source buffers. Test distinct multi-hunk edits, partial stage/unstage/discard, stale disk/index, cancellation/failure unchanged state, Unicode/special paths and missing final newline; UI tests for selection, preview, confirmation/cancel and double themes. Unsupported cases get explicit errors, not incorrect patches.

## Task 10: Real paginated file history

Replace the real Show History loaded-log filter with a dedicated native File History dialog. Query Git --follow at a frozen starting commit, page 100 commits at a time with request identity, cancellation, retry and clear empty/error/loading states. Keep the global Commit Log and main layout unchanged; no writes or history rewrite. Each row retains its historical path/status, so Show Diff/Open Revision reads the correct renamed file and disallows opening a deleted version. Virtual native list supports mouse/keyboard selection; buttons/context menus route through stable commands. Switching roots, closing or starting a write cancels reads; late callbacks cannot affect a new dialog. Tests use temporary rename histories, multiple pages, special paths, fixed-commit results across new commits, canceled/invalid requests, and MyGo Tester double-theme pagination/selection/retry/preview isolation. Global log pagination, line-selection history and directory history remain separate future work.

## Task 11: Configurable workspace shortcuts

Add a native Shortcuts settings editor for thirteen existing global workspace commands. Application stores canonical Ctrl/Alt/Shift chords and stable command IDs, validates keys, duplicate effective bindings and protected editor/panel/OS chords; empty override disables a binding, absent override uses defaults. Layout gains an optional Keymap pointer whose published maps are immutable, preserving legacy comparable Layout values and old JSON. Drafts clone mappings; Assign/Disable/Reset Selected/Restore Defaults edit only the draft, Apply/OK publish it and Cancel discards it. Runtime dispatch and menu hints read the same effective binding and route through the existing registry. Default Ctrl+F4 close alias remains until Close Tab is explicitly changed; Double Shift and local editor/panel shortcuts remain fixed. Test normalization/invalid/conflicts, JSON roundtrip/backup, native assign/apply/cancel/disable/reset, old-binding removal and new-key behavior, dual-theme visual evidence and existing shortcuts regression. Document limited initial scope and keep a reference of fixed/default shortcuts.

## Task 12: Real Patch copy and export

Enable existing Copy as Patch / Create Patch commands for selected tracked working/staged changes and fixed historical changes. Query canonical Git --binary/full-index patches without running external diff/textconv, validate checkout and file identities, use bounded executor stdout (16 MiB), reject untracked/conflicted selections and non-UTF-8 textual patch output explicitly. Preserve renamed/deleted/binary/root-commit patch semantics. Copy asynchronously with navigation/cancel guards and a content-free result notice. Create Patch opens a read-only native preview, then the locked MyGo Save dialog; a filesystem adapter creates a new patch file through an owned same-directory temp and atomic no-overwrite Link. Refuse existing destinations and Git metadata paths, preserve old files on cancel/error, never stage/commit, refresh original repository after file creation. Reuse unified commands/context cancellation and do not log patch bodies. Tests apply exported text/binary/rename/deletion patches in temporary clones, verify exact index/worktree/HEAD, bounds/stale/cancel/invalid cases; filesystem tests no overwrite/metadata/cancellation and UI copy/create/cancel/late-root/failure plus both themes. Import/apply of external patches remains deferred.

## Task 13: Source close decisions and reload

For editable real Project tabs, individual Close/Ctrl+W/Ctrl+F4 opens Save and Close / Discard and Close / Cancel rather than remaining disabled. Mock behavior and bulk-close dirty-tab protection stay intact. Bind confirmation to root/tab/buffer; save completion closes only the original tab when the exact submitted buffer is now clean. Save failure/newer typing retains the document; canceling a queued successful save suppresses closing. Discard explicitly resets only the unsaved editor buffer and closes, without disk writes. Factor per-repo tab removal so original-repo completion cannot switch navigation. Implement Reload from Disk for open real Project tabs via the existing async read port, confirm before discarding unsaved content, preserve edits on read failure/cancel/newer typing, update shared split metadata, and refresh repository metadata after success when the reader is available. Navigation/write/exit cancels pending decisions/reads. Add native buttons/context menu/Actions and dual-theme tests for confirmation, save/error/race, discard, reload success/error/cancel and old-root isolation. Historical views remain read-only; no unsaved-content persistence is claimed.
