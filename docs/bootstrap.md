# Bootstrap CLI (`cmd/init`)

A one-shot interactive CLI that turns this template into a tailored starter project: renames the module path, strips features you don't want, regenerates Wire + mocks, and removes itself. Self-destructs after a clean run — the resulting repo has no trace of the bootstrap tool.

## Quickstart

```bash
git clone https://github.com/<you>/go-mongo-boilerplate my-service
cd my-service
make init                                        # interactive (huh prompts)

# or scriptable:
cd cmd/init && go run . --root=../.. \
  --non-interactive --module=github.com/you/svc \
  --no-kafka --no-observability
```

**Prereqs in `PATH`:** `wire`, `mockery v3`. If missing:

```bash
go install github.com/google/wire/cmd/wire@latest
go install github.com/vektra/mockery/v3@latest
echo 'export PATH="$(go env GOPATH)/bin:$PATH"' >> ~/.zshrc
```

## Toggle reference

| Toggle | Removes | Keep when |
|---|---|---|
| _(none)_ | nothing | want the full template |
| `--no-kafka` | `internal/platform/kafka/`, `internal/user/event_handler*.go`, `kafka_checker.go`, kafka config + env + Make targets, kafka entries in `.mockery.yaml` / `docker-compose.yml` / README / CLAUDE.md | no message broker, OR only using SQS |
| `--no-sqs` | `internal/platform/sqs/` (+ its mocks), `sqs_checker.go`, sqs config + env, `make localstack-up` / `sqs-create-queues`, `docker-compose.localstack.yml`, `scripts/sqs-create-dev-queues.sh` | not using AWS SQS |
| `--no-observability` | `internal/platform/observability/`, `internal/platform/database/mongo_otel.go`, `config_observability_test.go`, `otelgin.Middleware` call, observability config + env, `make signoz-*`, `docker-compose.signoz.yml`, `deploy/signoz/` | not running OTel / SigNoz |
| `--no-samples` | `internal/user/`, `internal/role/`, `scripts/scaffold.sh`, samples entries in wire / mockery / README | bringing your own features — keep platform infra only |
| `--no-kafka` + `--no-sqs` _(cascade)_ | also removes `cmd/worker/`, worker config block, `make worker` target, worker entries in Dockerfile / docker-compose / docs | no async workload at all |

**Compatibility note:** `--no-samples` while keeping `--no-kafka=false` leaves the kafka platform code intact but strips the demo `user.EventHandler` — you'll need to wire your own `kafka.MessageHandler`. The nested markers handle this automatically; see CLAUDE.md "Maintaining the bootstrap CLI" for the pattern.

## Interactive walkthrough

```text
$ make init
cd cmd/init && go run . --root=../..

┌ New module path ────────────────────────────────────────┐
│ > github.com/yourorg/your-service                       │
│                                                         │
│ e.g. github.com/yourorg/your-service                    │
└─────────────────────────────────────────────────────────┘

┌ Features to keep ───────────────────────────────────────┐
│ Toggle to KEEP; unchecked items are stripped            │
│   [✓] Kafka (message broker)                            │
│   [✓] AWS SQS (second message broker)                   │
│   [✓] Observability (OTel + SigNoz)                     │
│   [✓] Sample features (user, role)                      │
└─────────────────────────────────────────────────────────┘

wire: github.com/yourorg/your-service/cmd/server: wrote …/cmd/server/wire_gen.go
wire: github.com/yourorg/your-service/cmd/worker: wrote …/cmd/worker/wire_gen.go
mockery v3.7.0 — wrote 5 mocks

✓ Bootstrap complete
  Module: github.com/yourorg/your-service
  Features kept:    kafka, sqs, observability, samples
  Features removed:

Next steps:
  make migrate-up   # apply Mongo migrations
  make run          # start the HTTP server
  make worker       # start the message consumer
```

## Scriptable form

```bash
cd cmd/init && go run . --root=../.. \
  --non-interactive --force \
  --module=github.com/acme/orderservice \
  --no-observability --no-samples
```

| Flag | Meaning |
|---|---|
| `--root` | project root, relative to `cwd` (default `../..` — assumes `cd cmd/init`) |
| `--non-interactive` | skip `huh` prompts; rely on `--no-*` flags + `--module` |
| `--force` | proceed even if working tree is dirty (default: refuse) |
| `--module` | new module path (`go mod edit -module=`) |
| `--no-kafka`, `--no-sqs`, `--no-observability`, `--no-samples` | strip the named feature |
| `--version` | print version, exit |

## What it actually does

1. **Pre-flight** — abort if `git status` is dirty, unless `--force`.
2. **Module rename** — `go mod edit -module=<new>`, then find/replace every literal `github.com/rock288/go-mongo-boilerplate` in `.go`, `.md`, `.yaml`, `.yml`, `.sh`, `.example`, `Makefile`, `Dockerfile`, `go.mod`. Walk skips `vendor/`, `.git/`, `node_modules/`, `bin/`.
3. **Per-feature strip** — for each `--no-*` toggle: delete `DeletePaths` (whole files / dirs), then run the line-based marker stripper over `MarkerFiles` to remove `feature:<name>:start` … `feature:<name>:end` blocks.
4. **Worker cascade** — if both `--no-kafka` and `--no-sqs`: remove `cmd/worker/` and strip `feature:worker:*` blocks.
5. **Post-actions** — in this fixed order: `wire ./...` → `go mod tidy` → `mockery` → `go build ./...`. Wire MUST run before tidy, otherwise the missing `wire_gen.go` breaks the import graph.
6. **Self-destruct** — strip any residual `feature:*` lines, `rm -rf cmd/init/`, run a final `go mod tidy`.

## Post-init checklist

```bash
git status                                                      # 19+ modified, cmd/init/ gone
grep -rE "feature:[a-z]+:(start|end)" --include='*.go' .         # → empty
grep charmbracelet/huh go.mod                                    # → empty (root stays clean)
make ci                                                          # lint + test + race + vuln + secrets
git add -A && git commit -m "chore: bootstrap from go-mongo-boilerplate"
```

## Troubleshooting

| Symptom | Fix |
|---|---|
| `wire not found in PATH` | `go install github.com/google/wire/cmd/wire@latest` then ensure `$(go env GOPATH)/bin` is on `PATH` |
| `mockery not found in PATH` | `go install github.com/vektra/mockery/v3@latest` |
| `dirty git tree at <root>; commit/stash first or pass --force` | `git status` → commit or `git stash`, or re-run with `--force` |
| `prompt: module path required` | provide `--module=<path>`, or fill the field in interactive mode |
| `module path must contain /, e.g. github.com/you/repo` | use a full path, not a bare word |
| `wire: generate failed` after strip | regression — file an issue with the combo + Go version. Recovery: `git restore .` |
| Residual `feature:*` markers after init | unexpected — file an issue. Manual cleanup: `grep -rlE "feature:[a-z]+:(start|end)" \| xargs perl -ni -e 'print unless /feature:[a-z]+:(start\|end)/'` |
| Build fails post-strip with an unused-import error | a marker pair is mis-paired or a needed import wasn't wrapped. `git restore .` and report the combo |

## Re-adding a stripped feature

**This bootstrap is one-shot.** There is no rollback. To get a feature back, copy it from the upstream template:

```
https://github.com/rock288/go-mongo-boilerplate
```

Look for files under `internal/platform/<feature>/` and the corresponding `feature:<name>` blocks in `cmd/server/wire.go`, `cmd/worker/`, `internal/platform/config/config.go`, `Makefile`, `docker-compose.yml`. After copying, run `make wire mocks test`.

## See also

- [`CLAUDE.md`](../CLAUDE.md) — section "Maintaining the bootstrap CLI" (for template maintainers adding new opt-out features)
- [`scripts/test-init-matrix.sh`](../scripts/test-init-matrix.sh) — run all 7 combos locally before pushing
- [`.github/workflows/init-matrix.yml`](../.github/workflows/init-matrix.yml) — CI exercise on every PR
