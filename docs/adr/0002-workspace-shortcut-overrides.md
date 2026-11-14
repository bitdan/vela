# ADR 0002: Optional workspace shortcut overrides

Date: 2026-11-13. Status: accepted for implementation under the continuous feature-completion authorization; cross-platform/manual acceptance remains separate.

## Context

Shortcuts settings previously displayed a reference only. Workspace bindings were repeated in dispatch and menu hints, and changing them required source changes. Existing layouts and layout-restore comparisons use value fields and must remain compatible.

## Decision

Add optional `Layout.Keymap *application.KeymapSettings`, with `Bindings` keyed by stable Command Registry IDs and canonical Windows Ctrl/Alt/Shift chords. A missing key uses its default; an explicit empty string disables it. Default-equal overrides normalize away. Only thirteen existing workspace actions are configurable initially. Double Shift and local editor/panel shortcuts stay fixed; existing native focus priority is retained.

Published binding maps are immutable. Settings clone mappings for drafts, validate assignment/reset and the full effective draft before Apply, and publish a new cloned settings object. Cancel cannot mutate live settings. Runtime dispatch, menu hints and editor hints resolve the same effective mapping. The default Ctrl+F4 close alias is active only while Close Tab has no override. Protected chords include the supported first strokes of locked MyGo v0.8.3 editor bindings, local panel bindings and OS closure keys; tests compare against the actual locked editor API.

Load/Save normalize and validate mappings. Invalid keymaps use the existing invalid-layout error and backup policy; invalid writes leave the previous JSON untouched. Old JSON lacks Keymap and retains all defaults. Older builds ignore the optional field and discard overrides when they next save; back up layout.json before rollback. No dependency or new persistence file is introduced.

## Alternatives and consequences

- A separate keymap file adds coordinated saves without a demonstrated need; rejected.
- A map directly in Layout breaks its existing comparable value contract; the optional pointer preserves that contract. Pointer equality compares identity, so new keymap roundtrip tests inspect binding values.
- Rebinding native editor commands or multi-stroke sequences requires a broader scope/priority editor; deferred.

Native Tester covers assignment, conflict/disable, Apply/Cancel isolation, reset collisions and actual key dispatch in both themes. Adapter tests cover canonical JSON roundtrip, invalid write protection and backups. Manual Windows desktop input is UNVERIFIED in this environment because the automation launch returned access denied (0x80070005); other platforms are UNVERIFIED.
