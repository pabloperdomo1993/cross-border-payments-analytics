# Testing Strategy

This document defines the testing philosophy for this distributed
system, and is explicit about the difference between what is
**implemented today** and what is a **planned improvement** — no test,
metric, or capability described here should be read as existing unless
it says so.

## Goals

- **Fast feedback.** Unit tests run with no external infrastructure and
  finish in seconds, so they run on every change, not just before a PR.
- **Confidence without slowness.** Integration and end-to-end tests use
  real infrastructure (real MariaDB, real Kafka, real ClickHouse) rather
  than mocks of them, so a passing suite means the actual wiring works —
  but they're gated behind an explicit build tag so they stay opt-in and
  don't slow down the default `go test ./...` loop.
- **Reproducibility.** Every test — unit, integration, or end-to-end —
  is deterministic: hand-written fakes instead of a mocking framework,
  fixed seeds where randomness is involved (see `scripts/seed`'s
  `-seed` flag), and no reliance on wall-clock timing beyond bounded
  `time.After` timeouts in tests that wait on a goroutine.
- **Protection against concurrency bugs.** This system's core
  correctness property — a bounded worker pool processing Kafka messages
  without racing on shutdown or offset marking — is exactly the kind of
  bug `go test` alone won't catch. `go test -race ./...` is a required
  check, not an optional one.
- **Protection against event-contract incompatibility.** `payments.created`,
  `payments.processed`, and `payments.dlq` are the only interface between
  independently deployable services. A change that silently breaks that
  JSON shape is a production incident waiting to happen, so the contract
  those events must satisfy is treated as testable surface — see
  **Contract Tests** below for what's actually covered today versus what
  isn't yet.

## Testing Levels

### Unit Tests

Run with plain `go test ./...` (or `npm run test` for the frontend) —
**no Docker, no network, no external infrastructure**. This is what runs
by default and what CI (if configured) would run on every push.

Examples that exist today:

- Payment validation and domain invariants:
  `backend/payments-service/internal/domain/transaction_test.go`,
  `outbox_test.go`.
- Payment creation/retrieval use cases against hand-written fakes:
  `backend/payments-service/internal/application/{create_transaction_test.go,get_transaction_test.go}`.
- HTTP handlers via `httptest`, including error-classification and
  metrics-increment assertions:
  `backend/payments-service/internal/handlers/http/{transaction_handler_test.go,health_handler_test.go,middleware_test.go}`.
- Worker pool behavior (bounded queue, graceful shutdown):
  `backend/payment-processor/internal/workerpool/pool_test.go`.
- Payment processing/classification logic:
  `backend/payment-processor/internal/processing/processor_test.go`.
- Kafka consumer handler logic against hand-written fakes for
  `sarama.ConsumerGroupSession`/`ConsumerGroupClaim` (offset-marking
  order, retry/DLQ routing, metrics increments):
  `backend/payment-processor/internal/kafka/consumer_test.go`,
  `backend/analytics-service/internal/kafka/consumer_test.go`.
- Analytics query filter validation:
  `backend/analytics-service/internal/domain/filter_test.go`.
- Config validation (fail-fast on invalid env vars):
  `backend/payment-processor/internal/config/config_test.go`,
  `backend/analytics-service/internal/config/config_test.go`.
- Frontend component/behavior tests (Vitest + React Testing Library):
  `frontend/src/components/MetricCard.test.tsx`,
  `frontend/src/features/payments/components/{PaymentFilters,PaymentsTable}.test.tsx`,
  `frontend/src/pages/DashboardPage.test.tsx`.

### Integration Tests

Run against **real** local infrastructure, gated behind the
`integration` build tag so they never run silently as part of a plain
`go test ./...` — bring the dependencies up first:

```bash
docker compose up -d mariadb kafka kafka-init clickhouse
make test-integration   # go test -tags=integration ./... per module
```

Tests that exist today:

- **payments-service → MariaDB**: `internal/repository/mariadb/transaction_repository_integration_test.go`
  (constraints: unique idempotency key, primary key, not-found).
- **payments-service → MariaDB + outbox**: `internal/outbox/relay_integration_test.go`,
  including a rollback test proving a payment row is never committed
  without its corresponding outbox row.
- **payment-processor → Kafka**: `internal/kafka/consumer_integration_test.go`
  — a real `payments.created` message reaching a real consumer and
  producing a real `payments.processed` message.
- **analytics-service → Kafka**: `internal/kafka/consumer_integration_test.go`
  — a real `payments.processed` message reaching a real consumer and
  landing in ClickHouse.
- **analytics-service → ClickHouse**: `internal/repository/clickhouse/analytics_repository_integration_test.go`
  (aggregation queries against deterministic fixture data).

Deliberately not present, and not planned as a large suite: broader
Kafka-broker-behavior tests (rebalances, network partitions, broker
restarts mid-consume). The two Kafka integration tests above cover the
two hops that matter for correctness; simulating broker failure modes
is a larger investment reserved for if/when a real incident motivates it.

### Contract Tests

Kafka event schemas are the contract between independently deployable
services — `payments-service` (producer of `payments.created`),
`payment-processor` (consumer of `payments.created`, producer of
`payments.processed`/`payments.dlq`), and `analytics-service` (consumer
of `payments.processed`/`payments.dlq`) can each be redeployed
independently, so a silent, incompatible change to any of these JSON
shapes is exactly the kind of bug unit tests within one service can't
catch.

**Current state, stated plainly**: there is no dedicated, separate
"contract test" suite today. The integration tests listed above do
exercise real JSON messages flowing between real services, which
provides *some* protection against gross incompatibility, but they are
not designed or maintained as schema-contract tests specifically (they
don't, for example, assert against a versioned schema file, and they
would not catch a backward-incompatible field removal that both sides
happen to still agree on in that test run).

The task's suggested envelope fields — `event_id`, `event_type`,
`event_version`, `occurred_at`, `correlation_id`, `payload` — describe a
reasonable **target shape** for a self-describing, evolvable event, but
they do not match what's actually on the wire today. The current schemas
(`backend/payment-processor/internal/domain/event.go`'s `PaymentEvent`/
`Outcome`, `backend/analytics-service/internal/domain/event.go`'s
`PaymentOutcome`) are flat payment-shaped payloads (`id`, country/currency/
amount/provider/status fields) with **no envelope**: no distinct
`event_id` (only the payment's own `id`), no `event_type`, no
`event_version`, no `occurred_at` (only `created_at`, the payment's
business timestamp, not the event's emission time), and no
`correlation_id`. This is a genuine, honestly-stated gap, not an
oversight to paper over.

**Backward compatibility and schema evolution** (target practice for
whenever this gap is closed): additive changes (a new optional field)
should never break an older consumer; removing or renaming a field is a
breaking change and requires either a new topic, an `event_version`
bump with dual-write/dual-read support during migration, or a
coordinated deploy — the classic constraints of any asynchronous,
independently-deployable-consumer system. Introducing `event_version`
into the actual schemas would be the natural first step toward making
this enforceable by a test rather than by convention alone.

### End-to-End Tests

The intended full-system flow:

```
HTTP request
    ↓
Payments Service
    ↓
MariaDB
    ↓
Kafka (payments.created)
    ↓
Payment Processor
    ↓
Kafka (payments.processed)
    ↓
Analytics Service
    ↓
ClickHouse
    ↓
Analytics API
```

**This test exists today**: `tests/e2e_test.go`
(`TestE2E_PaymentCreation_BecomesVisibleInAnalytics`), a standalone Go
module at the repo root. It exercises the system as a black-box HTTP
client — POST a payment to `payments-service`, then poll
`analytics-service`'s HTTP API until it's visible — proving the whole
chain works together, not just each hop in isolation. It requires the
full stack running (`docker compose up -d`) and is run via
`make test-integration` alongside the other integration tests.

### Concurrency Tests

`payment-processor`'s core value proposition is safe concurrent
processing of Kafka messages via a bounded worker pool
(`backend/payment-processor/internal/workerpool`), which makes the race
detector a required check, not an optional one:

```bash
go test -race ./...   # or: make test-race (requires a C compiler / cgo)
```

Run across all four Go modules (`payments-service`, `payment-processor`,
`analytics-service`, `scripts/seed`), this is what actually validates:

- **Data races** — particularly the worker pool's `sync.Once`/`RWMutex`
  -guarded queue close (avoiding a send-on-closed-channel panic under
  concurrent `Execute`/`Shutdown` calls) and every Kafka consumer/outbox
  -relay goroutine.
- **Bounded concurrency** — `pool_test.go` asserts the pool never runs
  more than `WORKER_COUNT` jobs simultaneously regardless of how many
  jobs are submitted at once.
- **Graceful shutdown** — `Shutdown()` drains in-flight and already
  -enqueued jobs before returning, bounded by a timeout at the call
  site (`main.go`), rather than dropping them.
- **Context cancellation** — `Pool.Execute` respects `ctx.Done()` while
  blocked waiting for queue capacity, rather than blocking forever.
- **Worker completion** — `wg.Wait()` inside `Shutdown()` guarantees
  every spawned worker goroutine has actually exited before the process
  continues tearing down (closing the metrics server, the Kafka
  producer).
- **Backpressure** — a full job queue blocks `Execute` (and therefore
  the Kafka consumer's per-partition read loop) rather than growing
  unboundedly or dropping messages; `pool_test.go` covers this directly.

### Failure / Resilience Tests

Covered today, with the specific test named:

- **Kafka publish failure during outbox relay** —
  `internal/outbox/relay_integration_test.go` and unit-level fakes in
  `relay_test.go` cover a publish failure leaving the event pending
  (retried on the next poll) rather than marked published.
- **DLQ publish itself failing** —
  `backend/payment-processor/internal/kafka/consumer_test.go`
  (`TestConsumerHandler_NeverMarksOffsetWhenDLQPublishFails`) proves the
  offset is never marked if even the dead-letter publish fails, so the
  message is redelivered rather than silently lost.
- **Malformed events** — both `payment-processor` and `analytics-service`
  consumer tests cover a message that fails to unmarshal (dead-lettered
  immediately for `payment-processor`; skipped-but-offset-marked for
  `analytics-service`, since a malformed *analytics* fact isn't worth
  redelivering forever).
- **ClickHouse insert failure** —
  `backend/analytics-service/internal/kafka/consumer_test.go`
  (`TestConsumerHandler_DoesNotMarkOffsetsWhenInsertFails` /
  `..._IncrementsInsertErrorsTotal`) proves no offset in a failed batch
  is marked, so the whole batch is redelivered.
- **Processing timeout / retry exhaustion** — `payment-processor`'s
  bounded in-process retry (`PAYMENT_MAX_RETRIES`,
  `PAYMENT_RETRY_BACKOFF`) and per-job timeout
  (`PAYMENT_PROCESSING_TIMEOUT`) are covered by
  `internal/kafka/consumer_test.go` and `internal/processing/processor_test.go`.
- **MariaDB constraint violations** (duplicate idempotency key,
  not-found) — `internal/repository/mariadb/transaction_repository_integration_test.go`.

**Not covered today** (planned, not implemented): Kafka broker
unavailability at consumer startup, MariaDB/ClickHouse unavailability
mid-request beyond what `/ready` reports, duplicate Kafka message
delivery (at-least-once redelivery producing a duplicate analytics row
is a known, accepted trade-off per `docs/architecture.md §7`, but there
is no test asserting *how much* duplication that produces or that
downstream aggregation tolerates it gracefully), and consumer-group
restart/rebalance behavior under real broker conditions.

### Load Tests

**No load testing tooling or results exist in this repository today.**
When load testing is performed, it must be reproducible and the numbers
must come from an actual run — the same discipline already applied in
[`docs/performance/sql-optimization.md`](performance/sql-optimization.md)
and [`docs/performance/mariadb-vs-clickhouse.md`](performance/mariadb-vs-clickhouse.md),
which report real, measured results (including an unflattering one)
against a real 1,000,000-row dataset rather than estimates.

Metrics a load test against this system should measure, once one exists:

- Requests/sec against `payments-service`'s `POST /api/v1/transactions`.
- Transactions/sec actually completing end-to-end (visible in
  `analytics-service`).
- P50 / P95 / P99 latency, both for the HTTP request itself and for
  `payment_processing_duration_seconds` (already exposed — see
  [docs/decisions/003-observability.md](decisions/003-observability.md)).
- Error rate (`http_requests_total{status=~"5.."}`,
  `payments_processing_failed_total`).
- Kafka consumer lag — **not currently exposed as a metric** by either
  consumer (`payment-processor` or `analytics-service`); this would need
  to be added before it could be measured under load.
- Worker pool utilization (`payment_worker_active` /
  `payment_worker_queue_size` versus `payment_worker_queue_capacity` —
  already exposed).

Never invent performance numbers in this document or elsewhere in the
repository — a claim without a reproducible run behind it doesn't belong
here.
