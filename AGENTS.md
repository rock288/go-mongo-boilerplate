# AGENTS.md

This repo follows the conventions in **[CLAUDE.md](./CLAUDE.md)**.

Cursor, Aider, Windsurf, Copilot, and any other agent tool: read `CLAUDE.md` first. It documents commands, architecture, coding conventions, package boundaries, observability stack, and the "Adding a feature" checklist.

Quick links:
- **Commands:** `make run | worker | test | lint | ci | wire | mocks`
- **Architecture:** [`docs/architecture.md`](./docs/architecture.md)
- **API contract:** [`docs/api.openapi.yaml`](./docs/api.openapi.yaml)
- **Scaffold a new feature:** `make scaffold name=order` (clones `internal/user/` → `internal/order/`)
- **Quickstart:** [`README.md`](./README.md)

When in doubt: copy `internal/user/` — it's the canonical example.
