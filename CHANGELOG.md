# Changelog

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
