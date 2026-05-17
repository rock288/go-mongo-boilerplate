# Changelog

All notable changes to this project are documented here. Format based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); semver per
[SemVer 2.0.0](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-05-17

Initial public release.

### Highlights

- Package-by-feature layout (`internal/<feature>/`) with shared `internal/platform/` infra
- Compile-time DI via `google/wire` (no runtime reflection)
- MongoDB v2 driver with custom OTel `CommandMonitor` + JSON migrations
- Kafka producer + consumer (`twmb/franz-go`) with retry-topic / DLQ flow,
  jittered backoff, W3C `traceparent` propagation, header sanitization
- **SQS** producer + consumer (`aws-sdk-go-v2`) — mirrors Kafka routing
  semantics, app-level retry queue with `MessageDelaySeconds` backoff
  (capped 900s), DLQ via redrive + app-level fallback
- OpenTelemetry full-stack → SigNoz / any OTLP collector
- Resilience: jittered exponential retry (cenkalti/backoff v5),
  named circuit breakers (sony/gobreaker v2)
- SSRF-safe HTTP client (blocks loopback / RFC1918 / link-local / metadata IPs)
- Health split: `/healthz` liveness vs `/readyz` readiness with cached
  checkers and fail-after-N hysteresis
- Cursor pagination helper with generic `Page[T]` envelope
- Shared `httperror` envelope `{error: {code, message}}`
- `X-Request-ID` middleware: read from header or generate, propagate via
  context + response header + structured log
- Distroless multi-stage Docker (`TARGET=server|worker|migrate`)
- `make ci` runs lint (golangci-lint v2) + race tests + govulncheck + gitleaks
- `make scaffold name=X` clones `internal/user/` into a new feature folder
- `CLAUDE.md` + `AGENTS.md` for AI-agent collaboration (Claude Code / Cursor /
  Aider / Windsurf)
- LocalStack dev stack for SQS testing
- LICENSE (MIT), CONTRIBUTING, CODE_OF_CONDUCT (Contributor Covenant v2.1),
  GitHub issue + PR templates

### What's intentionally NOT included (YAGNI)

- Authentication / authorization (every team has different requirements)
- Integration tests with testcontainers (build per-feature when needed)
- CQRS / event sourcing (demand-driven)
- GraphQL (different concern)
- FIFO SQS support (Standard queue only; add when you need ordering)

[0.1.0]: https://github.com/rock288/go-mongo-boilerplate/releases/tag/v0.1.0
