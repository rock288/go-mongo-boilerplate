# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make run            # cmd/server on :8002
make dev            # cmd/server with air hot-reload (auto-installed)
# feature:worker:start
make worker         # cmd/worker (Kafka consumer)
make dev-worker     # cmd/worker with air hot-reload
# feature:worker:end
make build          # builds server, worker, migrate into ./bin/ with ldflags version
make migrate-up     # apply migrations (also: migrate-down, migrate-version)
make wire           # regenerate cmd/{server,worker}/wire_gen.go
make mocks          # regenerate internal/*/mocks/ via mockery v3
make test           # gotestsum (auto-installed) wrapping go test, writes coverage.out
make test-race      # gotestsum -- -race
make test-cov       # open HTML coverage report
make lint           # golangci-lint (pinned, auto-installed)
make vuln           # govulncheck ./...
make secrets        # gitleaks detect
make ci             # lint + test + test-race + vuln + secrets
make docker-build   # build server / worker / migrate distroless images
make docker-scan    # trivy image scan (HIGH/CRITICAL fail)
# feature:observability:start
make signoz-up      # start local SigNoz stack (UI at :3301, OTLP :4317)
make signoz-down    # tear down SigNoz stack
# feature:observability:end
```

Single test: `go test ./internal/user -run TestUserService_Register -v`

First-time setup CLIs: `wire`, `mockery` (v3), `migrate` (with `mongodb` build tag), and optionally `gitleaks` + `trivy` for `make ci`/`make docker-scan`. See README for install commands.

## Architecture

**Package-by-feature with platform layer.** Each feature directory (`internal/user`, `internal/role`) is self-contained: `model.go`, `repository.go`, `service.go`, `handler.go`, `dto.go`, `routes.go`, `errors.go`, `wire.go`. Shared infrastructure lives in `internal/platform/{config,logger,database,httpserver,kafka,health,observability,resilience,httpclient}`.

**Compile-time DI via google/wire.** Two root injectors:
- `cmd/server/wire.go` → `InitializeServer` composes `platformSet` + feature `ProviderSet`s + `ProvideRouter`
- `cmd/worker/wire.go` → `InitializeWorker` for the Kafka consumer

Each feature exports a `ProviderSet` from its `wire.go` (constructors only — interfaces returned, not concrete types). `cmd/{server,worker}/providers.go` holds local providers (config slicers, router wiring, app struct).

**`wire_gen.go` is generated** — never edit by hand. After changing a `ProviderSet` or root injector, run `make wire`.

**Interface naming: no `I` prefix.** `UserService` is the interface, `userService` is the impl. Constructors (`NewUserService`) return the interface. This matters for mockery and for wire — provider sets reference constructors, not concrete types.

**Mocks are generated** under `internal/<feature>/mocks/`. After adding/changing an interface, add it to `.mockery.yaml` under `packages` and run `make mocks`. Tests import the mocks package and use testify-style expectations.

## Coding Conventions

Conventions the linter cannot enforce. Linter (`golangci-lint`) handles formatting, import order, unused symbols, error-wrap shadowing, etc. — do not duplicate those here.

**Naming**
- Interfaces: no `I` prefix. `UserService` (interface) / `userService` (impl). Constructors `NewUserService` return the interface.
- Files: `model.go`, `repository.go`, `service.go`, `handler.go`, `dto.go`, `routes.go`, `errors.go`, `wire.go` — one concern per file, no `utils.go`.
- Test files: `<unit>_test.go` next to the code; test names `Test<Type>_<Method>_<Scenario>` (e.g. `TestUserService_Register_DuplicateEmail`).
- Errors: sentinel `ErrXxx` in feature-local `errors.go`. No global `errors` package.
- Context vars: always named `ctx`, always first arg.

**Package boundaries**
- Package-by-feature. Cross-feature calls go through the target feature's exported interface — never reach into another feature's concrete types or repositories.
- `internal/platform/*` may be imported by any feature; features must not import each other's `internal` symbols.
- Domain models (`model.go`) hold no transport or persistence tags beyond what the repo/handler needs. Keep DTOs in `dto.go`.

**Constructors & DI**
- Constructors return interfaces, accept dependencies as interfaces. No package-level singletons.
- No `init()` for business logic — only for registering things that have no other hook (rare).
- Logger, tracer, config: injected, not read from globals inside functions.

**Context & cancellation**
- `context.Context` is always the first parameter; never stored in a struct.
- Pass `ctx` through to every I/O call (DB, HTTP, Kafka). Do not swap to `context.Background()` unless intentionally detaching for a background goroutine — and document why on the line above.
- Honor cancellation: respect `ctx.Err()` before long loops; do not swallow `context.Canceled` / `context.DeadlineExceeded`.

**Errors**
- Wrap with `fmt.Errorf("doing X: %w", err)` — include the operation, not just the variable name.
- Compare with `errors.Is` / `errors.As`, never string-match.
- Map domain errors → HTTP at the handler edge only. Services return domain errors; repositories return domain errors (translating driver errors).
- Use `resilience.Permanent(err)` to opt out of retry inside a retryable op.

**Logging**
- Get the logger from `ctx` (or injected). Never `log.Println` / `fmt.Println` in business code.
- Structured fields only: `logger.Info("user registered", "user_id", id)`. No `fmt.Sprintf` into the message.
- Do not log secrets, full request bodies, or PII at info level.

**Concurrency**
- Every goroutine has a clear lifecycle: bounded by `ctx`, a `sync.WaitGroup`, or an `errgroup.Group`. No fire-and-forget.
- Channels owned by the producer; producer closes. Receiver never closes.
- Mutex zero-value usable; do not copy a struct that contains a `sync.Mutex` (pass by pointer).

**Testing** (go-kit style — unit tests only, demonstration-focused)
- Table-driven tests by default; one `t.Run(name)` per case.
- `t.Parallel()` in pure unit tests; never in tests that touch shared mock registry or env vars.
- Mocks via mockery; use testify `mock.Anything` sparingly — prefer specific argument matchers so a contract change surfaces as a test failure.
- One test file per production file (`recovery.go` ↔ `recovery_test.go`); shared helpers in `helpers_test.go` per package.
- Integration / E2E tests are NOT included in the boilerplate — when you add a feature that needs a real Mongo or Kafka, build the integration harness for THAT feature; do not pre-build infrastructure on speculation.

**Comments**
- Exported symbols get a doc comment starting with the symbol name (`// UserService handles ...`).
- Inline comments explain *why*, not *what*. Skip comments that restate the code.

## Config

`config/config.yaml` carries defaults. Override **any** value via env vars with the pattern `APP_<SECTION>__<KEY>` — note the **double underscore** for nesting (koanf convention). Example: `APP_SERVER__PORT=9090`, `APP_MONGO__URI=mongodb://...`, `APP_OBSERVABILITY__SAMPLE_RATIO=0.1`.

**Exception:** `cmd/migrate` reads `MONGO_URI` and `MONGO_DATABASE` directly (no `APP_` prefix) because it runs standalone without loading the koanf config tree.

`Config.Validate()` runs at boot and rejects unsafe combinations: observability `enabled=true` with `insecure=false` requires a real `ingestion_key`; CORS `allow_credentials=true` combined with `"*"` in origins is fatal.

<!-- feature:observability:start -->
## Observability

**Stack:** OpenTelemetry SDK → SigNoz (self-hosted compose or cloud). Initialised in `internal/platform/observability/otel.go` (`Init` returns a shutdown func).

- Tracing: OTLP gRPC, BatchSpanProcessor, `ParentBased(TraceIDRatioBased)` sampler.
- Metrics: OTLP gRPC, periodic reader 60s.
- Logging: stdout slog with auto trace_id/span_id injection from active span context (see `logger.traceHandler`).
- Propagation: W3C `traceparent` + `baggage`.
- Resource attributes: `service.name`, `service.version` (ldflags), `deployment.environment`.

**HTTP:** `otelgin.Middleware` runs FIRST so panic events recorded by `middleware.Recovery` attach to the span.

**Mongo:** custom `event.CommandMonitor` in `database/mongo_otel.go` (no official otel pkg for mongo-driver/v2). Spans keyed by `RequestID` with a janitor goroutine purging entries older than 2 minutes (leak protection).

**Kafka:** manual W3C trace inject (producer) + extract (consumer) via `record.Headers`. Retry/DLQ publishes call `ForceSample()` so failure paths always show up in SigNoz.

**Adding a span in service code:**
```go
ctx, span := otel.Tracer("internal/user").Start(ctx, "UserService.Register")
defer span.End()
```
<!-- feature:observability:end -->

## Health

`internal/platform/health.Registry` runs registered `Checker`s every `cacheTTL` (default 5s) in a background goroutine and serves cached results. `/healthz` returns 200 always (liveness). `/readyz` returns 200 unless a `SeverityCritical` checker has failed N consecutive cycles (default 3). `/internal/readyz` exposes per-check details — bind it on an internal network only.

**Adding a checker:** implement `Checker` (Name, Severity, Check) and call `registry.Register(c)` in `ProvideHealthRegistry`. Use `SeverityCritical` only for dependencies that genuinely block serving (e.g. primary database); use `SeverityDegraded` for things the service can run without (e.g. Kafka in the HTTP server).

<!-- feature:worker:start -->
## Message brokers

Two brokers ship side-by-side with **identical routing semantics**: handler returns nil/Canceled/ErrNonRetryable/other → commit/drop/DLQ/retry. Both inject W3C `traceparent` + `x-idempotency-key`, both run an in-process retry consumer that moves messages back to main, both use 3-attempt bounded republish.

### When to pick which

| Use case | Kafka | SQS |
|---|---|---|
| High throughput (>10k msg/s) | ✅ | ⚠️ batch + multiple workers |
| Strong ordering per key | ✅ partition | ❌ Standard (FIFO not shipped) |
| AWS-only deployment | ⚠️ MSK costs | ✅ native, cheap |
| Multi-cloud / portable | ✅ | ❌ AWS-only |
| Replay history | ✅ retention | ❌ delete on ACK |
| Low-volume notifications | ⚠️ overkill | ✅ |
| Backoff > 15 min | ✅ no limit | ❌ `MessageDelaySeconds` capped at 900s |

### Kafka

**Contract:** `kafka.MessageHandler.Handle(ctx, record) error`.
**Flow:** `topic` → handler fail → `topic.retry` → retry consumer (`<group>-retry`) sleeps `min(base*2^n, max)` → re-publish to `topic` → after `MaxRetries` → `topic.dlq`.
**Producer:** `kafka.Producer.Publish(ctx, topic, key, value, opts...)`.

### SQS (`internal/platform/sqs`)

**Contract:** `sqs.MessageHandler.Handle(ctx, msg *types.Message) error` — same return semantics as Kafka.
**Flow:** `events` → handler fail → `events-retry` (with `MessageDelaySeconds=backoff`) → retry consumer moves back to `events` → after `MaxRetries` → `events-dlq`.
**Producer:** `sqs.Producer.Publish(ctx, queueURL, body, opts...)` — auto-injects traceparent + idempotency, rejects with `ErrTooManyAttributes` when caller exceeds 7 custom `MessageAttributes` (3 reserved).

**Gotchas:**
- `MessageDelaySeconds` AWS cap = 900s → `RetryBackoffMax` silently clamps. For longer backoffs, use Step Functions / external scheduler.
- `MessageAttributes` limit = 10 keys; 3 reserved (`traceparent`, `tracestate`, `x-idempotency-key`), so callers get 7.
- Standard queue ≠ Kafka partition: NO ordering guarantee. FIFO queues not shipped.
- Publish-then-delete is non-atomic; rare crash window relies on SQS visibility timeout for redrive — consumer code MUST be idempotent via `GetIdempotencyKey(msg)`.
- App-level retry queue is the primary mechanism. Native SQS redrive policy (`maxReceiveCount=5`) wired by `make sqs-create-queues` acts as a safety net for stuck/crashed messages.
- Production: queues must be provisioned by Terraform/CDK. Worker only needs `sqs:Send/Receive/Delete/GetQueueUrl/ChangeMessageVisibility` IAM. Boilerplate does NOT auto-provision.

**Hygiene shared by both:** `SanitizeForRepublish` strips attacker-controlled `x-retry-count`/`x-error-reason`, drops malformed `traceparent`. `SanitizeError` truncates to 256 ASCII chars before stamping `x-error-reason` on DLQ.

### Adding an SQS-consuming feature

1. Create `internal/<feature>/` with `event_handler_sqs.go` implementing `sqs.MessageHandler`:
   ```go
   type SQSEventHandler struct { logger *slog.Logger /* + deps */ }
   func NewSQSEventHandler(logger *slog.Logger) *SQSEventHandler { return &SQSEventHandler{logger: logger} }
   func (h *SQSEventHandler) Handle(ctx context.Context, msg *types.Message) error { /* ... */ }
   ```
2. Update `cmd/worker/providers.go`: replace `ProvideSQSHandler` to return the feature's handler:
   ```go
   func ProvideSQSHandler(h *<feature>.SQSEventHandler) platformsqs.MessageHandler { return h }
   ```
3. Add `<feature>.NewSQSEventHandler` to `cmd/worker/wire.go` ProviderSet.
4. `make wire mocks test`.
5. Set `APP_SQS__CONSUMER__QUEUE_URL` to enable. With QueueURL empty OR handler nil, worker logs skip and only Kafka goroutines run.

### Local dev

```bash
make localstack-up        # start LocalStack on :4566
make sqs-create-queues    # provision events / events-retry / events-dlq (LocalStack only)
```

Then set `APP_SQS__ENDPOINT=http://localhost:4566` and the printed `QUEUE_URL` in `.env`.
<!-- feature:worker:end -->

## Resilience

- `resilience.Do[T](ctx, cfg, op)` — retry with jittered exponential backoff (cenkalti/backoff v5). Wrap non-retryable errors with `resilience.Permanent(err)`.
- `resilience.Breaker[T](registry, cfg)` — named circuit breakers (sony/gobreaker v2). Timeout is jittered ±20% to desync half-open windows across replicas.
- `httpclient.NewInternalClient(cfg)` — for service-to-service calls inside a trust boundary.
- `httpclient.NewExternalClient(cfg)` — for any URL that may originate from user input. Custom dialer blocks loopback / RFC1918 / link-local / metadata IPs (`169.254.169.254`). **Default to External for any non-hardcoded URL.**

## Middleware order

Set in `cmd/server/providers.go:ProvideRouter`:
```
otelgin → Recovery → RequestLogger → RateLimit → BodyLimit → Timeout → CORS → handler
```

Rationale: `otelgin` first so its span is on `ctx`; `Recovery` records panics on that span; `RateLimit`/`BodyLimit` reject load early; `Timeout` propagates cancellation to mongo/HTTP-client calls; `CORS` last (cheap header math).

`http.Server` itself enforces `ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout` — these CANNOT be replaced by handler-level timeouts because body read happens before the handler runs.

## Migrations

`migrations/*.up.json` / `*.down.json` use the **golang-migrate Mongo JSON format**, not SQL. The migrate runner injects the database name as the URI path before handing off to the driver — see `cmd/migrate/main.go:buildDSN`.

## Docker

Multi-stage distroless build (`Dockerfile`). Set `TARGET=server|worker|migrate` build arg. `VERSION` + `COMMIT` build args propagate to `main.version` / `main.commit` via ldflags. Healthcheck runs the binary itself with the `healthcheck` subcommand, which hits `GET /readyz` on `127.0.0.1:$PORT` — distroless has no curl/wget, so subcommand is the only option.

## Testing Strategy (go-kit style)

Unit tests only — demonstration-focused, not exhaustive. The sample features (`user`, `role`) ship with canonical examples for each pattern; copy them when adding real features.

**Layout** — `*_test.go` next to source, mirror production split:
```
internal/middleware/
  recovery.go        recovery_test.go
  timeout.go         timeout_test.go
  body_limit.go      body_limit_test.go
  ...                helpers_test.go    # shared test setup (gin.TestMode init, discardLogger)
```

**Refactor seams (kept testable without live infra):**
- `kafka.Producer` accepts a `kafkaWriter` interface — mock via fake struct, no broker needed.
- `kafka.consumer.decideRoute(err, retryCount, max) Route` — pure function; routing logic unit-tested exhaustively.
- `observability.exporterOptions(cfg)` — pure config builder, separate from `Init()` (which dials gRPC).

**Adding tests for a new feature** — copy from `internal/user/`:
- `service_test.go` — table-driven, mockery for repo
- `handler_test.go` — `gin.CreateTestContext` + service mock; assert HTTP status + DTO mapping
- `event_handler_test.go` (if async) — fake `*kgo.Record` + service mock

**Why no integration / E2E?** — boilerplate is template, not service. testcontainers + retry-flow scenarios + full-stack bootstrap belong in YOUR repo when you have a real feature to test, not pre-installed on speculation. See [go-kit](https://github.com/go-kit/kit) — same philosophy.

**Coverage** — no gate. `make test-cov` opens HTML for inspection only.

## Adding a feature

1. Create `internal/<feature>/` with the standard files (model, repository, service, handler, dto, routes, errors, wire).
2. Export `ProviderSet` from `wire.go`.
3. Register interfaces in `.mockery.yaml` → `packages`.
4. Reference `<feature>.ProviderSet` from `cmd/server/wire.go` (and worker if it has async handlers — see `user.NewEventHandler` usage).
5. Wire routes into `ProvideRouter` in `cmd/server/providers.go`.
6. Add tests per the Testing Strategy section above.
7. `make wire && make mocks && make test`.

## Module path note

The module is `github.com/rock288/go-mongo-boilerplate`. When used as a template for a new project, run `make init` (interactive) or `cd cmd/init && go run . --root=../.. --non-interactive --module=…` — that handles rename, feature strip, wire/mocks regen, and removes itself. Manual `go mod edit` + find/replace works too but skips the feature-trim step.

## Maintaining the bootstrap CLI

When you add a NEW opt-out feature to the template:

1. Wrap feature-specific code with `// feature:<name>:start` / `// feature:<name>:end` markers (use `#` for YAML/Make/env, `<!-- -->` for Markdown). Markers are line-based and substring-matched — comment style is irrelevant.
2. Add a `Feature{}` entry to `cmd/init/manifest.go` (`DeletePaths` + reuse `allMarkerFiles`). Use `DeletePaths` for files entirely feature-specific (whole-file removal sidesteps marker gymnastics — see `internal/platform/health/kafka_checker.go`).
3. Add a `huh.NewOption(...)` line in `cmd/init/prompt.go`; extend `computeStripList` accordingly.
4. Add a `--no-<name>` CLI flag in `cmd/init/main.go`.
5. Append a matrix combo to `.github/workflows/init-matrix.yml` AND `scripts/test-init-matrix.sh`.
6. Run `bash scripts/test-init-matrix.sh` locally — every combo must pass.

When two features couple (e.g. kafka's demo handler lives in the samples package), wrap with **nested markers** — outer = the surrounding feature, inner = the coupled feature. Stripping either tag removes the block; orphan outer markers are wiped by `stripAllResidualMarkers` during self-destruct.

### Runtime semantics

- Stripper is **substring + line-based**; comment style is irrelevant — `//`, `#`, `<!-- -->`, and anything else carrying `feature:<name>:start|end` on the line all work.
- **Generated files are NEVER marked** — `wire_gen.go` and `**/mocks/*.go` are deleted-then-regenerated by post-actions, so markers there would survive into the regen and be meaningless.
- Post-action order is **locked**: `wire → tidy → mockery → build`. Wire must precede tidy because tidy walks the import graph; missing `wire_gen.go` breaks it.
- `cmd/init/` is a **separate Go module** (its own `go.mod` carries the `huh` dep). The root module never imports from `cmd/init`. Self-destruct deletes the whole directory; do not add reverse imports.
- The `oldModule` constant in `cmd/init/rename.go` is the literal pre-rename path. The rename walk explicitly skips `cmd/init/rename.go` so the constant survives until self-destruct.

### Markers vs split-file

| Use markers when… | Split into a separate file when… |
|---|---|
| Feature-specific lines sit inside a shared file (config struct field, wire provider entry, single function body) | The whole file is feature-specific (`kafka_checker.go`, `mongo_otel.go`, `config_observability_test.go`) |
| The feature contributes a few imports that other features also use | The feature's imports cause unused-import errors when the feature is stripped (import-wrap gymnastics — split is cleaner) |
| The block is < ~20 lines | The block is the entire file or > ~50 lines |

When in doubt, prefer split-file: whole-file deletion via `DeletePaths` has zero risk of marker mismatch and produces cleaner stripped output.

### Anti-patterns (DO NOT)

- DO NOT mark `wire_gen.go` or any `**/mocks/*` file — they're regenerated, markers don't survive in a meaningful way.
- DO NOT change the post-action order (`wire → tidy → mockery → build`). Tidy before wire fails on the broken import graph.
- DO NOT add a feature without also adding a matrix combo to `.github/workflows/init-matrix.yml` AND `scripts/test-init-matrix.sh`. CI is the only catch for marker drift.
- DO NOT touch the `oldModule` constant in `cmd/init/rename.go` (it's the literal pre-rename module path; updating it post-fork breaks the find/replace).
- DO NOT make `cmd/init/` depend on anything in the main module — it would self-reference after rename, and the separate `go.mod` would fail to resolve.
- DO NOT wrap markers around the package declaration (`package x`) — Go requires it; the file becomes invalid post-strip.
