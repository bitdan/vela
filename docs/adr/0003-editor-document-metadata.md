# ADR 0003: Optional source editor metadata in workspace sessions

Date: 2026-11-14. Status: implementation under continued feature authorization; desktop acceptance remains separate.

Add optional Bookmarks (zero-based line numbers) and FileType (empty automatic, Plain Text, or MyGo language name) to TabSnapshot. These describe a view, not Git data or file content. Existing workspace JSON and atomic replacement remain in use; no dependency/new store is needed.

Split views share the same in-memory metadata alongside the Buffer. Snapshots clone slices so mutating a session payload cannot change live bookmarks. On restore, sort/deduplicate valid line numbers, discard out-of-range entries, and fall back to automatic language detection if unavailable. Bookmark positions are numeric through edits, without claiming semantic tracking. Historical versions and repositories retain separate tab identities.

Alternatives: global path-only settings would mix repositories and historical versions; storing file contents is unrelated to this feature; native diagnostic anchors would misuse diagnostics and require extra tracking. None is adopted.

Compatibility: no migration for old JSON. Older builds ignore new optional fields and drop them on their next save. Back up workspace.json before rollback if metadata must be preserved. No credentials/content are added. User requires documentation remain local, so this ADR is not part of the code commit.
