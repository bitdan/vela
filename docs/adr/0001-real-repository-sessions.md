# ADR 0001: Add real repository identities to workspace JSON

Date: 2026-11-13. Status: accepted for this implementation under the user's continuous feature-completion authorization; desktop acceptance remains separate.

## Context

The original workspace.json only saved Mock tabs by display name. Real repositories and commit drafts were lost on normal exit. Two repositories can share a name, and saved Git status becomes stale immediately.

## Decision

Add optional ActiveRoot and Root fields to the existing workspace JSON. Each root records its own draft, selection identities and tab source/path, pin, cursor, scroll and Diff layout. Persist no Git status, credentials or file content. Existing fields remain readable; old Mock snapshots remain supported. Use Git's canonical root after opening, including Windows long-path normalization.

Fixed-commit tabs additionally record optional ChangeKind/OldPath identity metadata. These describe an immutable historical change, not current checkout status. This lets paginated file-history revisions and renamed/root-commit diffs restore even when outside the global loaded log. Revision text reads its blob directly without requiring a parent or a current project path. Existing JSON remains valid; older versions ignore these optional fields.

Bootstrap connects the existing asynchronous Application readers before requesting restoration. Only the last active real repository loads at startup; the others remain selectable recent entries. Tab contents are reread from the appropriate disk/index/commit source. Cancellation/request identity guards prevent late results from overriding user navigation. Missing repositories and failed tab identities remain retryable in saved state. Normal exit uses existing temporary-file and replacement persistence.

## Alternatives and consequences

- Eagerly reopening all repositories makes startup depend on every directory and multiplies Git work; rejected.
- Persisting complete Repository snapshots is faster to display but presents stale Git state and unnecessarily stores file data; rejected.
- A separate database adds dependencies without a demonstrated need; rejected.

The additive format requires no migration step. Rolling back reads the old fields but loses new real-session state on the next old-version save; back up workspace.json before rollback if it must be preserved. JSON contains local repository paths and commit-message drafts and uses the existing user config directory; it must not be committed to the project.

## Validation

UI tests cover same-name roots, draft/selection and tab roundtrip, lazy recent opening, missing roots, cancellation and current disk/index/history content via a temporary Git repository. Manual desktop acceptance and large-session performance remain UNVERIFIED.
