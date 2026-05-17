# Operator Runbook

Quick lookups for common failures.

## Symptom → Check → Remediation

### `/readyz` returns 503

1. `curl /internal/readyz` (internal network) → see which checker failed.
2. If `mongo`: check the Mongo container/cluster, network policy, credentials.
3. If 3+ consecutive failures: process will stop accepting traffic for a load-balancer until a check succeeds (cache TTL 5s, threshold 3).

### Traces missing in SigNoz

1. `APP_OBSERVABILITY__ENABLED=true`?
2. `APP_OBSERVABILITY__ENDPOINT` reachable from the pod (try `nc -vz <host> 4317`).
3. For SigNoz Cloud, `APP_OBSERVABILITY__INSECURE=false` AND `APP_OBSERVABILITY__INGESTION_KEY` set?
4. `APP_OBSERVABILITY__SAMPLE_RATIO > 0`? (Default 1.0 local, expected 0.1 prod.)
5. Retry/DLQ paths force-sample → if those are missing too, exporter is broken end-to-end.

### Consumer lag growing

1. `kafka-consumer-groups --describe --group <base>` → see lag per partition.
2. Check retry topic: `<base>.retry` — is the retry consumer goroutine alive? It uses group `<base>-retry`.
3. DLQ topic `<base>.dlq` should accumulate poison-pill records; investigate `x-error-reason` header.
4. If handler is slow: scale workers (each pod gets one main + one retry consumer).

### High HTTP latency

1. `APP_SERVER__HANDLER_TIMEOUT` — handlers exceeding this see `context.Canceled` from `c.Request.Context()`.
2. Mongo span on a request trace: check `db.operation` and duration.
3. Rate limiter map size: if `APP_RATE_LIMIT__PER_IP__RPS` low and traffic spikes from many IPs, the LRU evicts on each request — bump `map_max_entries`.

### Slowloris / stuck connections

`http.Server.ReadHeaderTimeout = 5s` by default. Any client that takes longer to send headers is dropped. Adjust `APP_SERVER__READ_HEADER_TIMEOUT` if legitimate slow clients (rare).

### Kafka rebalance loop

1. Worker SIGTERM during in-flight handler? Two-phase shutdown drains up to `APP_WORKER__SHUTDOWN_TIMEOUT`.
2. Handler too slow per record? franz-go sends heartbeats automatically; check broker logs for session-timeout drops.
3. Consumer group session timeout vs handler latency mismatch.

### Out of memory

1. Trace SDK `BatchSpanProcessor` queue: `WithMaxQueueSize(2048)` — bursts above that drop spans (logged by SDK).
2. Mongo CommandMonitor map: should self-purge entries older than 2 minutes. If size grows unbounded, driver may not be firing `Succeeded/Failed` — file a bug; janitor will GC stale entries.
3. Rate limiter map: capped at `APP_RATE_LIMIT__MAP_MAX_ENTRIES`.

### Panic in a handler

Logged with stack via `middleware.Recovery`. On SigNoz the span shows status=ERROR with the panic recorded as an exception event. `/readyz` does not flip — only repeated panics affect a checker.

### SQS

**Dashboards (CloudWatch):**
- `ApproximateNumberOfMessagesVisible` — backlog depth
- `ApproximateAgeOfOldestMessage` — alert at 5 min
- DLQ depth — should be 0 in steady state; alert >0

**Required IAM** on the worker role:
- `sqs:SendMessage`, `:ReceiveMessage`, `:DeleteMessage`
- `:GetQueueUrl`, `:ChangeMessageVisibility`
- **NOT** `:CreateQueue` — queues provisioned by Terraform/CDK; boilerplate does not auto-provision

**Common errors:**
- `AWS.SimpleQueueService.NonExistentQueue` — `queue_url` wrong or queue not provisioned
- Throttling: 300 ops/s per Standard queue; on throttle worker logs + backs off
- `MessageDelaySeconds` capped at 900s — `RetryBackoffMax` clamps silently

## On-call quick links

- SigNoz UI: `http://signoz.<env>.example.com` or `localhost:3301` for local
- Mongo Atlas: `https://cloud.mongodb.com`
- Kafka tooling: `docker exec -it <kafka> kafka-consumer-groups.sh --bootstrap-server localhost:9092`
- SQS CLI (LocalStack): `aws --endpoint-url=http://localhost:4566 sqs list-queues`
- Source: `https://github.com/rock288/go-mongo-boilerplate`
