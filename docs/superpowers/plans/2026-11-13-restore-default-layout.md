# Restore Default Layout

**Goal:** Provide a recoverable default workspace through View and Search Actions, as required by VELA_SPEC.md §4.2.

**Design:** Add one stable `workspace.restore-default-layout` command. Reset only five panel split percentages and four visibility flags using application.DefaultLayout. Restore the active tool pages to Commit and Log so all default regions are reachable. Preserve theme, settings, repositories, selections, drafts, editor buffers, cursor/scroll and split editor state. Existing shutdown persistence saves the resulting layout; no new format or dependency.

**Alternatives:** Resetting the whole Layout would erase preferences; recreating repoView would erase editor state. Both are rejected. A new layout manager is unnecessary.

**Scope:** Go 1.27.4 / MyGo v0.8.3. Existing working checkout; preserve untracked specs/reports. No Git feature expansion, commit, merge or push. Gate 2 remains subject to user confirmation.

**Implementation:**

- [x] Add Tester regressions for View restoration and keyboard action search, preferences/state preservation, and persistence roundtrip. Observe failure before implementation.
- [x] Register and execute the minimal command, expose in View and Search Actions.
- [x] Run UI and persistence packages, targeted vet and Windows build. Export and inspect 1440×900 dark/light screenshots.
- [x] Update product/current docs and record actual acceptance evidence and limitations.

**Review focus:** Hidden parents/children, altered split sizes, Project/Shelf/Console pages, modified and inactive repository editors, preference preservation and saved layout reload. Manual desktop/IDEA parity stays UNVERIFIED.
