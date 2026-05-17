# Architecture & Project Structure

Tài liệu này mô tả cấu trúc thư mục của repo, tác dụng từng folder, và đánh giá mức độ tuân thủ các design pattern phổ biến cho ứng dụng Go production.

> Module path: `github.com/rock288/go-mongo-boilerplate`
> Go version: 1.25
> Phong cách: **package-by-feature + platform layer**, DI compile-time bằng `google/wire`.

---

## 1. Bird's-eye view

```
go-mongo-boilerplate/
├── cmd/                # Entry points (main packages)
│   ├── server/         # HTTP API server (Gin)
│   ├── worker/         # Kafka consumer worker
│   └── migrate/        # Standalone migration runner
├── internal/           # Private application code (Go compiler enforces)
│   ├── platform/       # Shared infrastructure (cross-feature)
│   ├── middleware/     # HTTP middleware (Gin)
│   ├── user/           # Feature: user (vertical slice)
│   └── role/           # Feature: role (vertical slice)
├── config/             # Default config (config.yaml)
├── migrations/         # MongoDB JSON migrations (golang-migrate)
├── deploy/             # Deployment manifests (SigNoz, otel-collector)
├── scripts/            # Maintenance scripts (scaffold.sh)
├── docs/               # Architecture, ADRs, journals, runbook, OpenAPI spec
├── plans/              # Planning docs (local, gitignored)
├── Dockerfile          # Multi-stage distroless build
├── docker-compose.yml  # Local dev stack (mongo, kafka, redis)
├── docker-compose.localstack.yml  # Optional SQS dev via LocalStack
├── Makefile            # Standard targets (run/build/test/lint/...)
├── lefthook.yml        # Optional pre-commit/pre-push hooks
├── .mockery.yaml       # Mock generation config
├── CLAUDE.md           # Agent-facing conventions (Claude Code)
└── AGENTS.md           # Multi-agent compat (Cursor/Aider/Windsurf) — points at CLAUDE.md
```

---

## 2. Folder-by-folder

### 2.1 `cmd/` — Entry points

Mỗi sub-folder là một `main` package độc lập, build ra một binary riêng. **Không chứa business logic**; chỉ làm bootstrap, gọi Wire, lắng nghe signal, gọi shutdown.

| Sub | Vai trò | File chính |
|---|---|---|
| `cmd/server/` | HTTP API server (Gin) | `main.go`, `wire.go`, `wire_gen.go`, `providers.go` |
| `cmd/worker/` | Kafka consumer (background processor) | tương tự server |
| `cmd/migrate/` | Standalone migration CLI; đọc `MONGO_URI` / `MONGO_DATABASE` trực tiếp (không qua koanf) | `main.go` |

**Quy ước:** `wire_gen.go` là code generated — không edit tay. Sau khi đổi provider set, chạy `make wire`.

### 2.2 `internal/` — Private code

Go compiler chặn import `internal/...` từ bên ngoài module. Đây là nơi chứa toàn bộ logic.

#### 2.2.1 `internal/platform/` — Hạ tầng dùng chung

Cross-feature infrastructure. Bất kỳ feature nào cũng được phép import; features **không** được import lẫn nhau qua đây.

| Package | Trách nhiệm |
|---|---|
| `config/` | Load YAML + env overrides (koanf), validate config tại boot |
| `logger/` | slog handler auto-inject `trace_id`/`span_id` từ active span |
| `database/` | Mongo client + custom `event.CommandMonitor` cho OTel spans |
| `httpserver/` | Gin server wrapper, timeouts (Read/Write/Idle), graceful shutdown |
| `httpclient/` | `NewInternalClient` (trusted) và `NewExternalClient` (SSRF-safe, chặn loopback/RFC1918/metadata IPs) |
| `kafka/` | Producer (auto-inject `traceparent`, idempotency key) + Consumer (retry/DLQ flow) + Header sanitizer |
| `sqs/` | AWS SQS Producer + Consumer mirroring the Kafka layout. App-level retry queue with `MessageDelaySeconds` backoff (cap 900s) + DLQ + W3C trace propagation via `MessageAttributes`. Mock SQS API for tests via `SQSAPI` interface |
| `health/` | Registry chạy nền cache kết quả `Checker`, expose `/healthz` (liveness) và `/readyz` (readiness, fail-after-N) |
| `observability/` | OTel SDK init: tracing + metrics + log correlation, OTLP gRPC tới SigNoz |
| `resilience/` | Retry với jittered exponential backoff (cenkalti/backoff v5) + named Circuit Breakers (sony/gobreaker v2) |
| `pagination/` | Cursor-based pagination: opaque base64(JSON) cursor (`Cursor`), generic envelope `Page[T]`, `ClampLimit` (default 20, max 100) |
| `httperror/` | Shared error envelope `{error:{code,message}}` + helpers `BadRequest`/`Conflict`/`NotFound`/`Internal` — handlers không tự lặp `gin.H{"error":...}` |

#### 2.2.2 `internal/<feature>/` — Vertical slice

Mỗi feature là một package độc lập, đầy đủ một concern. Hiện có `user` và `role`. Layout chuẩn:

```
internal/user/
├── model.go         # Domain model (struct, không có transport tags)
├── dto.go           # Request/response DTOs (JSON/validator tags)
├── repository.go    # Mongo persistence (interface + impl)
├── service.go       # Business logic (interface + impl)
├── handler.go       # HTTP handlers (Gin)
├── event_handler.go # Kafka message handler (chỉ feature nào có async work)
├── routes.go        # Đăng ký route lên router
├── errors.go        # Sentinel errors (ErrXxx) — feature-local
├── wire.go          # ProviderSet cho Wire
└── mocks/           # Generated mocks (mockery v3) — KHÔNG edit tay
```

**Quy tắc cross-feature:** chỉ gọi nhau qua interface đã export ở `service.go`. Không được reach vào repository hoặc concrete struct của feature khác.

#### 2.2.3 `internal/middleware/` — HTTP middleware

Tách riêng khỏi từng feature vì đây là cross-cutting concern toàn server. Thứ tự apply (set ở `cmd/server/providers.go:ProvideRouter`):

```
otelgin → Recovery → RequestID → RequestLogger → RateLimit → BodyLimit → Timeout → CORS → handler
```

`RequestID` đọc header `X-Request-ID` từ client hoặc generate UUID, lưu vào `c.Request.Context()` (downstream Mongo/Kafka/HTTP calls thừa kế), echo lại trong response header, và `RequestLogger` tự pick up vào field `request_id`.

Files: `recovery.go`, `request_id.go`, `request_logger.go`, `rate_limit.go`, `body_limit.go`, `timeout.go`, `cors.go`.

### 2.3 `config/` — Default config

- `config.yaml`: defaults checked-in.
- Override bằng env vars theo pattern `APP_<SECTION>__<KEY>` (double underscore là nesting separator của koanf).
- Validation chạy tại boot (`Config.Validate()`): từ chối observability `enabled+insecure=false` mà không có ingestion key, CORS `allow_credentials=true` với `"*"`, v.v.

### 2.4 `migrations/` — DB migrations

- Format **JSON của golang-migrate Mongo driver**, không phải SQL.
- Convention: `NNNNNN_<name>.up.json` / `NNNNNN_<name>.down.json`.
- Hiện có: `000001_init_users`, `000002_init_roles`.
- Chạy qua `cmd/migrate` hoặc `make migrate-up`.

### 2.5 `deploy/` — Deployment artifacts

- `deploy/signoz/otel-collector-config.yaml`: cấu hình OpenTelemetry Collector cho SigNoz local.
- Mở rộng tương lai: helm charts, kustomize overlays, terraform... bỏ vào đây.

### 2.6 `docs/` — Documentation

| Sub/file | Mục đích |
|---|---|
| `architecture.md` | (file này) Mô tả cấu trúc + design patterns |
| `api.openapi.yaml` | OpenAPI 3.1 spec cho các endpoint mẫu — hand-written, sync khi thêm endpoint |
| `runbook.md` | Operational playbook (incident response, oncall) |
| `ci-template.md` | CI pipeline template |
| `adr/` | Architecture Decision Records — gitignored, ghi nội bộ |
| `journals/` | Engineering journal entries — gitignored |

### 2.6a `scripts/` — Maintenance scripts

| File | Mục đích |
|---|---|
| `scaffold.sh` | Clone `internal/user/` → `internal/<name>/` với perl rename. Gọi qua `make scaffold name=Order` |

### 2.7 `plans/` — Local planning

Gitignored. Chứa kế hoạch sprint, refactor, reports do agents tạo ra. Không phải artifact ship lên production.

### 2.8 Root files

| File | Vai trò |
|---|---|
| `Dockerfile` | Multi-stage distroless, build arg `TARGET=server\|worker\|migrate` |
| `docker-compose.yml` | Local stack: mongo + kafka + redis |
| `docker-compose.signoz.yml` | SigNoz stack riêng (UI :3301, OTLP :4317) |
| `Makefile` | Targets: `run`, `worker`, `build`, `migrate-up`, `wire`, `mocks`, `test`, `lint`, `vuln`, `secrets`, `ci`, `hooks`, `scaffold`, `docker-build`, `docker-scan` |
| `.mockery.yaml` | Mockery v3 config — đăng ký interfaces cần mock |
| `.golangci.yml` | Lint config v2 (pinned `v2.1.0`, auto-installed via Makefile) |
| `.gitleaks.toml` | Secret scanning rules |
| `.env.example` | Mẫu env vars cho dev local |
| `lefthook.yml` | Optional pre-commit (gofmt/lint/gitleaks) + pre-push (test); install bằng `make hooks` |
| `CLAUDE.md` | Conventions cho Claude Code khi làm việc trên repo |
| `AGENTS.md` | Stub redirect cho Cursor / Aider / Windsurf / Copilot — trỏ về `CLAUDE.md` |
| `LICENSE` | MIT |
| `CONTRIBUTING.md` | PR flow, conventional commits, test requirements |
| `CODE_OF_CONDUCT.md` | Contributor Covenant v2.1 |

---

## 3. Đánh giá Design Patterns

> **TL;DR:** Codebase đạt chuẩn ở mức **production-grade Go service**. Tuân thủ tốt các pattern phổ biến cho microservice (DI, repository, service layer, circuit breaker, ports & adapters lite). Một số nơi có thể siết thêm (xem mục §4).

### 3.1 Patterns đã áp dụng tốt

#### Package-by-feature (Vertical Slice) ✅
- Mỗi feature (`user`, `role`) self-contained, không phụ thuộc concrete type của feature khác.
- Đối lập với "package-by-layer" (`models/`, `controllers/`, `services/`) — phong cách hiện tại scale tốt hơn khi số feature tăng.

#### Clean / Hexagonal Architecture (nhẹ) ✅
- `service.go` định nghĩa interface (port); `repository.go`, `httpclient`, `kafka` là adapter.
- Service không biết đến HTTP hay Mongo — handler map error domain → HTTP ở "biên".
- Repository trả về domain error, không leak `mongo.ErrNoDocuments` lên trên.

#### Dependency Injection (compile-time) ✅
- `google/wire` thay vì runtime container (như Uber fx) → lỗi DI phát hiện tại compile-time, không có reflection overhead.
- Mọi constructor `NewXxx` trả về **interface** (không phải concrete) → giúp mock dễ dàng và đảo ngược phụ thuộc.

#### Repository Pattern ✅
- `UserRepository` / `RoleRepository` là interface; impl `userRepository` nằm cùng file, ẩn driver Mongo khỏi service layer.

#### Service Layer Pattern ✅
- Business rule tập trung tại `service.go`, không leak xuống handler hay lên repository.

#### Strategy Pattern ✅
- `health.Checker` interface — mỗi dependency (Mongo, Kafka, Redis) là một strategy.
- `kafka.MessageHandler` — tách topic-specific logic khỏi consumer infra.

#### Circuit Breaker ✅
- `resilience.Breaker[T]` (sony/gobreaker v2), timeout jittered ±20% để desync half-open windows giữa các replica → tránh thundering herd khi recovery.

#### Retry with Backoff ✅
- `resilience.Do[T]` — jittered exponential. Opt-out bằng `resilience.Permanent(err)`.

#### Observer Pattern ✅
- Mongo `event.CommandMonitor` (vì mongo-driver/v2 chưa có official otel pkg) → custom monitor publish spans, janitor goroutine purge entries > 2 phút (chống leak).

#### Decorator / Middleware Pipeline ✅
- Thứ tự middleware được document rõ ràng kèm lý do (otelgin trước Recovery để panic span gắn vào trace).

#### Producer-Consumer + Retry/DLQ ✅
- Kafka flow: `events` → handler fail → `events.retry` (backoff) → re-publish → sau N lần → `events.dlq`.
- Header sanitizer (`SanitizeForRepublish`, `SanitizeError`) chống attacker-controlled `x-retry-count` và truncate error string 256 chars.
- Trace context propagation thủ công (W3C `traceparent` trong `record.Headers`) — `ForceSample()` ở failure path để đảm bảo failure visible trên SigNoz.

#### Defense-in-depth tại HTTP boundary ✅
- `http.Server` enforce `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout` (handler-level timeout không thay thế được vì body read xảy ra trước handler).
- `NewExternalClient` chặn SSRF (loopback, RFC1918, link-local, metadata IPs như `169.254.169.254`).

#### Graceful Shutdown ✅
- Server và worker đều capture SIGTERM/SIGINT, drain in-flight requests, gọi `shutdown()` của OTel để flush span/metric.

#### Health Pattern (Liveness vs Readiness) ✅
- `/healthz` (liveness) luôn 200.
- `/readyz` (readiness) fail sau N lần consecutive failure của critical checker → tránh flap.
- `/internal/readyz` per-check details, chỉ bind internal network.

#### Configuration Validation at Boot ✅
- Fail-fast với combination nguy hiểm (CORS `*` + credentials, OTel insecure + key thật...).

### 3.2 Điểm tích cực khác

- **Testability:** mocks generate sẵn ở `internal/<feature>/mocks/`, table-driven tests, `t.Parallel()` đúng chỗ.
- **Build determinism:** golangci-lint version pinned và auto-installed; mockery v3 pinned trong `.mockery.yaml`.
- **Supply-chain hygiene:** `make ci` chạy `govulncheck` + `gitleaks` + `trivy` (HIGH/CRITICAL fail).
- **Observability-first:** từ slog handler tự inject trace_id đến Mongo command monitor → trace coverage gần đầy đủ.
- **Docker:** distroless image, healthcheck dùng binary subcommand `healthcheck` (vì distroless không có curl/wget).
- **Vibe-coding friendly:** `CLAUDE.md` + `AGENTS.md` cho AI agent, `make scaffold` clone feature template, `httperror` envelope thống nhất → AI thêm endpoint mới không lặp boilerplate.
- **Request correlation:** `X-Request-ID` middleware propagate qua `ctx` + response header + log field, làm cầu nối debug giữa client log và server trace.

---

## 4. Khoảng trống & gợi ý cải thiện

| # | Hiện trạng | Gợi ý |
|---|---|---|
| 1 | Chỉ có 2 feature (`user`, `role`) → chưa stress-test ranh giới module | Khi thêm feature thứ 3+ có thể cross-call, cân nhắc dùng **mediator / event bus in-process** thay vì service-to-service interface để tránh n×m dependency |
| 2 | Domain model = persistence model (cùng `model.go`) | Khi domain phức tạp hơn, tách `domain.User` (rich domain) khỏi `dao.User` (Mongo doc); hiện tại OK vì model còn anemic |
| 3 | `repository.go` trả về domain model trực tiếp | Đủ dùng cho boilerplate; nếu cần tối ưu read (CQRS) thì tách `Query`/`Command` repository |
| 4 | Chưa có **outbox pattern** cho transactional event publishing | Khi business yêu cầu "DB write + Kafka publish must be atomic" → cần outbox; hiện tại không có vì chưa có use case |
| 5 | `internal/middleware/` đặt cạnh features chứ không trong `platform/` | Defensible (middleware là HTTP-specific, không phải infra), nhưng có thể chuyển vào `internal/platform/httpserver/middleware/` cho consistency |
| 6 | Không có **API versioning** layout (`v1/`, `v2/`) | Boilerplate, chưa cần; lúc thêm v2 thì nhân đôi route file thay vì file mới — cân nhắc package `internal/user/v1/`, `v2/` |
| 7 | Wire generated file checked-in | Đúng convention Go (giúp build không cần wire binary), nhưng nhớ chạy `make wire` mỗi lần thay provider |
| 8 | Chưa có **integration test** với build tag `//go:build integration` (đã mention trong CLAUDE.md nhưng chưa thấy file) | Nên có 1 file mẫu `repository_integration_test.go` để team biết cách viết |
| 9 | ~~Error mapping HTTP nằm rải rác ở handler~~ | ✅ **Closed (2026-05-17):** `internal/platform/httperror` cung cấp envelope chung + helpers; user + role handler đã migrate. Khi feature scale, gọi `httperror.BadRequest(c, err)` thay vì lặp `c.JSON(400, gin.H{...})` |
| 10 | Kafka consumer group, topic name hardcode trong feature | Khi thêm nhiều topic, gom vào `internal/<feature>/topics.go` constant block |

---

## 5. Kết luận

| Tiêu chí | Đánh giá |
|---|---|
| Package layout | ⭐⭐⭐⭐⭐ — chuẩn package-by-feature, có platform layer rõ ràng |
| Dependency Injection | ⭐⭐⭐⭐⭐ — Wire compile-time, interface-driven |
| Separation of concerns | ⭐⭐⭐⭐½ — domain ↔ infra tách tốt; có thể CQRS/outbox khi cần |
| Resilience patterns | ⭐⭐⭐⭐⭐ — retry, circuit breaker, jittered, SSRF-safe |
| Observability | ⭐⭐⭐⭐⭐ — OTel full-stack (trace+metric+log correlation), Mongo monitor |
| Testability | ⭐⭐⭐⭐ — mocks generated, table-driven; thiếu integration test mẫu |
| Operational readiness | ⭐⭐⭐⭐⭐ — health split, graceful shutdown, distroless, supply-chain scan |
| Documentation | ⭐⭐⭐⭐ — CLAUDE.md đầy đủ; runbook và ADR cần fill nội dung thực |

**Tóm lại:** Repo đạt chuẩn production-grade Go service. Các pattern áp dụng đúng chỗ, không over-engineer. Những gì còn thiếu (outbox, CQRS, API versioning) đều là **demand-driven** — chỉ nên thêm khi có yêu cầu kinh doanh cụ thể, đúng tinh thần YAGNI.
