# Changelog

## Unreleased

- Include third-party license notices in release archives.
- Update dependencies to address GO-2026-5024 and GO-2026-5970; the affected
  functions were not found reachable by the CLI vulnerability scan.
- Add automated vulnerability checks and contributor/security guidance.
- Refresh installation and shell completion instructions.

## 0.1.1 — 2026-09-13

- Derive the version from Git tags when building or installing with Make.
- View the selected project with `plane project view`.
- Select a default project interactively with `plane project use`.
- Simplify release titles to the version tag and improve ignore rules.

## 0.1.0 — 2026-09-13

Initial release of Plane CLI, a Go client for Plane Cloud and self-hosted Plane.

- Interactive work item browser with filtering and scrollable details.
- Work item creation, editing, state transitions, assignment, comments, and deletion.
- Project, state, label, cycle, and module management.
- Human-readable project keys and name/email resolution.
- Cycle and module membership commands.
- Private local configuration, environment overrides, and interactive setup.
- JSON and table output, shell completion, and browser links.
- Pagination, request timeouts, and bounded retries for rate-limited reads.
- Automated tests on Linux, macOS, and Windows.
- Versioned release archives for amd64 and arm64, with SHA-256 checksums.

Live compatibility must be verified against your Plane deployment. Attachments,
pages, custom fields, OAuth, and saved board/view creation are not included.
