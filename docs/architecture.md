# Cross-Border Payments Analytics — Architecture

## 1. Overview

Cross-Border Payments Analytics is an event-driven platform designed to
ingest, process, and analyze international payment transactions.

The system demonstrates a production-oriented architecture using:

- Go
- React + TypeScript
- Apache Kafka
- MariaDB
- ClickHouse
- Prometheus
- Grafana
- Docker
- Docker Compose

The entire platform is designed to run locally using Docker Compose.

A developer should only need Git and Docker to run the complete system.

---

## 2. Problem Statement

Companies operating cross-border payment platforms process transactions
across multiple countries, currencies, payment providers, and payment
corridors.

Operational databases are optimized for transactional workloads but are
not ideal for analytical queries over large volumes of historical payment
data.

This platform separates:

- Transaction ingestion
- Payment processing
- Event distribution
- Operational persistence
- Analytical persistence
- Analytics queries
- Observability

The system receives international payment transactions and produces
analytics across dimensions such as:

- Source country
- Destination country
- Source currency
- Destination currency
- Payment corridor
- Payment provider
- Transaction status
- Time period

---

## 3. Architecture Goals

The architecture is designed around the following principles:

- Event-driven communication
- Clear service boundaries
- Asynchronous payment processing
- Concurrent processing using Go
- Separation of OLTP and OLAP workloads
- Horizontal scalability
- Idempotent event processing
- Fault tolerance
- Observability by default
- Local reproducibility using Docker
- Graceful shutdown
- Automated testing

---

## 4. High-Level Architecture

```text
                           ┌─────────────────────┐
                           │       React         │
                           │    TypeScript       │
                           │      :5173          │
                           └─────────┬───────────┘
                                     │
                                     │ REST / JSON
                                     ▼
                           ┌─────────────────────┐
                           │  Payments Service   │
                           │        Go           │
                           │       :8080         │
                           └─────────┬───────────┘
                                     │
                          ┌──────────┴──────────┐
                          │                     │
                          ▼                     ▼
                     ┌─────────┐            ┌─────────┐
                     │ MariaDB │            │  Kafka  │
                     │  OLTP   │            │  KRaft  │
                     │  :3306  │            │  :9092  │
                     └─────────┘            └────┬────┘
                                                │
                                      payments.created
                                                │
                                                ▼
                                   ┌─────────────────────┐
                                   │ Payment Processor   │
                                   │        Go           │
                                   │                     │
                                   │    Worker Pool      │
                                   │    Goroutines       │
                                   └──────────┬──────────┘
                                              │
                                              │
                                  payments.processed
                                              │
                                              ▼
                                          ┌───────┐
                                          │ Kafka │
                                          └───┬───┘
                                              │
                                              ▼
                                   ┌─────────────────────┐
                                   │ Analytics Service   │
                                   │        Go           │
                                   │       :8082         │
                                   └──────────┬──────────┘
                                              │
                                              ▼
                                       ┌────────────┐
                                       │ ClickHouse │
                                       │    OLAP    │
                                       │   :8123    │
                                       └─────┬──────┘
                                             │
                                             │ analytics
                                             ▼
                                           React
```

Implementation status: `payments-service` (HTTP + MariaDB + Transactional Outbox), `payment-processor` (Kafka consumer/worker pool + producer), and `analytics-service` (Kafka consumer + ClickHouse + HTTP analytics API) are all implemented, per sections 5–7 below. `payments-service` now publishes to `payments.created` for real via its outbox relay (see section 7) — the whole documented critical path (POST → MariaDB → outbox → Kafka → payment-processor → Kafka → analytics-service → ClickHouse → analytics API) is verified end-to-end, both manually and by an automated test (`tests/e2e_test.go`). `frontend`, `prometheus`, and `grafana` all run via `docker compose up`; Kafka, MariaDB, and ClickHouse container health checks and Go-service `/health`+`/ready` endpoints gate startup ordering.

---

## 5. Payment Processor: concurrency & offset strategy

`payment-processor` consumes `payments.created`, validates and "processes"
each event through a **bounded worker pool**, and publishes the outcome to
`payments.processed` or, on failure, `payments.dlq`.

### Why bounded concurrency

A goroutine-per-message consumer has no ceiling: a burst of Kafka traffic
would spawn unbounded goroutines and unbounded memory/connection use
downstream. A bounded worker pool caps how much processing work can run
at once, regardless of how fast Kafka delivers messages — the classic
bulkhead pattern applied to a Kafka consumer.

```text
Kafka Consumer (one goroutine per assigned partition)
      |
      v
+------------------+
| Bounded Channel  |   capacity = WORKER_QUEUE_SIZE
+--------+---------+
         |
         v
+---------------------------+
|      Worker Pool          |   size = WORKER_COUNT
|                           |
| W1 W2 W3 ... Wn           |
+-------------+-------------+
              |
              v
       Payment Processor (validate, classify)
              |
       +------+------+
       |             |
       v             v
    success        failure
  (payments.      (retry, then
   processed)      payments.dlq)
```

### Worker count & bounded queue

`WORKER_COUNT` (default 10) is the maximum number of payment jobs
executing at once, enforced by a fixed number of long-lived goroutines
draining a fixed-capacity Go channel (`internal/workerpool`) — not
one-goroutine-per-message. `WORKER_QUEUE_SIZE` (default 100) is that
channel's capacity. Both must be positive; `payment-processor` fails
fast at startup otherwise (see `internal/config`).

### Backpressure

Submitting a job (`Pool.Execute`) blocks on a channel send when the
queue is full. There is no unbounded queue and no dropped messages: a
full queue simply makes the Kafka consumer's per-partition read loop
wait, which is real backpressure propagating all the way back to Kafka
fetch pacing.

### Cancellation & per-job timeout

The worker pool takes and propagates `context.Context` throughout;
`context.Background()` is never used inside request/message processing.
Each job additionally gets its own `context.WithTimeout(parentCtx,
PAYMENT_PROCESSING_TIMEOUT)` (default 5s) so a stuck payment can't
occupy a worker indefinitely — `cancel()` is always deferred immediately
next to the `WithTimeout` call.

### Kafka offset / partition strategy (the core correctness question)

Concurrently processing multiple messages from the *same* partition and
committing out of order risks marking an offset "done" while an earlier
message from that partition is still failed or in-flight. The chosen
strategy is **concurrency across partitions, strict sequential
processing within each partition**:

- Sarama's consumer-group API calls `ConsumeClaim` once per assigned
  partition, each in its own goroutine, with messages delivered in
  order within that goroutine.
- For each message, that partition's goroutine submits one job to the
  shared bounded pool and **blocks until it completes** before reading
  the partition's next message or marking any offset.
- This guarantees at most one in-flight job per partition at any time,
  so offsets are always marked in order, per partition — there is no
  code path where a later message's success can mark an offset past an
  earlier message's unresolved one.
- Concurrency comes from *different* partitions each holding one job in
  the shared pool simultaneously. **True parallelism is therefore
  `min(WORKER_COUNT, partitions assigned to this consumer)`** — a
  fundamental property of safely-ordered Kafka processing, not a
  limitation of this implementation. Locally, `payments.created` (and
  the other two topics) are created with 6 partitions specifically so
  `WORKER_COUNT=10` has real concurrency to exercise.
- A message is only ever marked processed after its outcome has been
  successfully published (or dead-lettered) — never merely because it
  was enqueued into the worker pool.

### Graceful shutdown

`SIGINT`/`SIGTERM` → `signal.NotifyContext` cancels the consumer's
context, which stops `ConsumeClaim` from claiming new messages → the
worker pool's `Shutdown()` closes the job queue (exactly once, guarded
against a concurrent-close panic) and waits for already-enqueued and
in-flight jobs to finish, bounded by a 10s timeout → the metrics HTTP
server and Kafka producer are closed → the process exits. Any message
still in flight when the timeout is exceeded is simply never marked, so
it's safely redelivered on the next run (at-least-once).

### Error behavior: retryable vs non-retryable, minimal retry, DLQ

No retry/DLQ mechanism existed before this work, so a deliberately
**minimal** one was built rather than a general retry framework:
- **Non-retryable** (malformed JSON, invalid payment data, unsupported
  currency) → dead-lettered immediately, no retry attempted.
- **Retryable** (the `payments.processed` publish failing — a real
  transient-broker scenario — or the per-job timeout elapsing) →
  retried in-process up to `PAYMENT_MAX_RETRIES` additional times
  (default 3) with a fixed `PAYMENT_RETRY_BACKOFF` (default 200ms)
  between attempts, all still bounded by the one overall per-job
  timeout. No retry topic, no external scheduler.
- After retries are exhausted (or immediately, for a non-retryable
  error), the event is published to `payments.dlq` with the failure
  reason, and the offset is marked — the partition is never stuck
  behind a permanently-failing message.
- A failure is never silently discarded: if even the DLQ publish itself
  fails, the offset is *not* marked, and the message is redelivered
  later rather than lost.

### Observability

`payment-processor` exposes Prometheus metrics on `METRICS_PORT`
(default 9091, scraped by the `prometheus` compose service):
`payment_worker_active`, `payment_worker_queue_size`,
`payment_worker_queue_capacity` (pool saturation/backpressure),
`payments_processing_total` and `payments_processing_failed_total`
(low-cardinality `status`/`retryable` labels only — no
transaction/correlation IDs as labels), and
`payment_processing_duration_seconds`.

### Known limitations / follow-ups

- `payments-service` does not yet publish to `payments.created` — that
  wiring is a natural next step. Locally, test events can be produced
  by hand (see the payment-processor README section / final report for
  the exact `kafka-console-producer` command).
- No integration test runs against a real Kafka broker; `internal/kafka`
  is instead tested with hand-written fakes for
  `sarama.ConsumerGroupSession`/`ConsumerGroupClaim`, which is enough to
  prove the offset-ordering property but doesn't exercise real broker
  behavior (rebalances, network partitions, etc).

---

## 6. SQL & data modeling: MariaDB, ClickHouse, and analytics-service

### MariaDB (OLTP): schema evolution

`payments-service`'s `transactions` table grew across three versioned
migrations rather than being redesigned in place:

- **001** — the original table: `id`, `source_country`,
  `destination_country`, `source_currency`, `destination_currency`,
  `amount`, `fx_rate`, `provider`, `status`, `created_at`. Only
  `PRIMARY KEY (id)`.
- **002** — added `idempotency_key` (`VARCHAR(128)`, unique index —
  lets a client safely retry a creation request without producing a
  duplicate row), renamed `amount` → `source_amount`, added
  `destination_amount` (both sides of the currency conversion, not just
  the sending side), and `updated_at`.
- **003** — added the composite index `(provider, status, created_at)`,
  measured in `docs/performance/sql-optimization.md`. No blind
  single-column indexes: this one composite serves provider-only,
  provider+status, and provider+status+created_at queries via leftmost
  -prefix matching.

All monetary columns are `DECIMAL`, never `FLOAT`/`DOUBLE`; the Go
domain layer mirrors this with fixed-point integer types and a
`Money.Multiply(FXRate)` helper implemented via `math/big` (never
`float64`) to compute `destination_amount` from `source_amount` and
`fx_rate` without rounding error or overflow.

### ClickHouse (OLAP): `payments_analytics`

A deliberately different schema from MariaDB's — see the extensive
comments in `backend/analytics-service/migrations/001_create_payments_analytics.sql`
for the full reasoning. Summary: `MergeTree`, `PARTITION BY
toYYYYMM(created_at)`, `ORDER BY (source_country, destination_country,
created_at)` (optimized for the corridor-volume query this system leads
with — a documented, explicit trade-off against currency/provider-first
queries), `LowCardinality(String)` for every low-cardinality dimension
column, native `Decimal`/`DateTime64`/`UUID` types.

### analytics-service

New third Go service, following the same conventions as
`payments-service`/`payment-processor` (`cmd/api`, `internal/{config,
domain, repository, handlers/http}`, env-var config with fail-fast
validation, `signal.NotifyContext` graceful shutdown). Two ingestion
paths into ClickHouse:

- **Real-time**: a Kafka consumer batches `payments.processed` and
  `payments.dlq` messages (by count or a flush-interval timer — never
  row-by-row, which is bad practice for ClickHouse) and bulk-inserts via
  `clickhouse-go/v2`'s native `PrepareBatch`. Offsets for a batch are
  only marked after that batch's insert succeeds.
- **Bulk/benchmark**: `scripts/seed`, a standalone tool, loads a large
  deterministic dataset directly into both MariaDB and ClickHouse for
  the performance experiments below — intentionally bypassing Kafka,
  since pushing a million individual messages through the real pipeline
  would test Kafka throughput, not SQL performance.

This required extending `payment-processor`'s `Outcome` Kafka contract
(previously just `{id, status, reason}`) to carry the full payment
dimensions, since that topic is `analytics-service`'s only real input.

HTTP API (`/api/v1/analytics/{corridors,currencies,countries,providers,
timeseries}`, plus `/health`/`/ready`) uses only parameterized
ClickHouse queries — filter *values* are never string-concatenated into
SQL, though the countries endpoint's grouping *column* and the
time-series endpoint's bucketing *function* are switched via
code-controlled (never user-supplied) identifiers.

### Performance experiments

Two documents under `docs/performance/` contain **real, measured**
results (never fabricated) from a 1,000,000-row deterministic dataset:

- `sql-optimization.md` — a MariaDB before/after index experiment.
  Honestly reports a nuanced result: the composite index made
  `COUNT(*)`-shaped queries ~400–450x faster, but the actual
  row-returning query it was built around showed no wall-clock
  improvement, because per-row lookups back into the clustered index for
  non-indexed columns (plus transferring ~20K rows) dominate at ~2%
  selectivity — a real lesson about index limits, not a validation
  failure.
- `mariadb-vs-clickhouse.md` — the same corridor-volume aggregation on
  equivalent datasets in both engines (~150x faster on ClickHouse),
  framed explicitly as an OLTP-vs-OLAP architectural illustration, with
  a correctness cross-check between the two result sets.

---

## 7. Transactional Outbox, testing strategy, and full local stack

### Transactional Outbox (`payments-service`)

Previously deferred (see earlier revisions of this document); now
implemented. `outbox_events` (migration 004) is written in the exact
same SQL transaction as the `transactions` row it describes —
`TransactionRepository.Create` does `BEGIN; INSERT transactions; INSERT
outbox_events; COMMIT`, with a `defer sqlTx.Rollback()` that only
actually fires (rolling back the payment insert too) if the outbox
insert or the commit itself fails. A payment can never exist without a
corresponding queued event — verified directly by
`TestIntegration_Create_RollsBackPaymentWhenOutboxInsertFails` against a
real MariaDB.

A small in-process relay (`internal/outbox/relay.go`, started as a
goroutine from `main.go`) polls `outbox_events` on a ticker
(`OUTBOX_POLL_INTERVAL`, default 500ms), publishes each pending event's
already-serialized JSON payload to `payments.created` via a Kafka
producer, and marks it published only after a successful publish.

**Delivery semantics — stated plainly**: at-least-once, not
exactly-once. If the relay crashes between a successful Kafka publish
and the local "mark published" write, that event is republished on
restart. Building true exactly-once (e.g. a distributed transaction
across MariaDB and Kafka) is out of scope — this is a documented,
accepted trade-off, not an oversight, and it's the same trade-off every
outbox-pattern implementation without XA/2PC makes.

### Testing strategy

Each Go module keeps two independent test surfaces:

- **Plain `go test ./...`** — no Docker required. Domain unit tests,
  application/use-case tests against hand-written fakes (never a real
  database or broker), and HTTP handler tests via `httptest`. This is
  what runs by default and stays fast.
- **`go test -tags=integration ./...`** — real MariaDB / ClickHouse /
  Kafka, gated behind a build tag so it's opt-in and never silently
  skipped-but-passing. Covers: outbox atomicity (rollback, pending
  -event retrieval, publish-marking), MariaDB constraints (unique
  idempotency key, primary key, not-found), ClickHouse aggregation
  queries against deterministic fixture data (the exact
  CO→US/CO→US/MX→US example from this project's own test plan), and two
  focused Kafka flows — `payments.created` reaching a real
  `payment-processor` consumer and producing a real `payments.processed`
  message, and that message reaching a real `analytics-service` consumer
  and landing in ClickHouse. Deliberately not "dozens" of Kafka
  tests — two, covering the two hops that matter.
- **`tests/` (repo root)** — one automated end-to-end test
  (`TestE2E_PaymentCreation_BecomesVisibleInAnalytics`) that exercises
  the whole system as a black-box HTTP client: POST a payment, poll
  analytics-service until it's visible. This is what actually proves the
  full chain works together, not just each hop in isolation.
- **`go test -race ./...`** — run for all four Go modules. The worker
  pool's `RWMutex`-guarded close (to avoid a send-on-closed-channel
  race) and every Kafka consumer/outbox-relay goroutine were verified
  clean under the race detector, not just reasoned about.

### Docker Compose: the complete local stack

`frontend` (React/Vite, built via a Node multi-stage Dockerfile and
served by nginx — static files only, no Node.js in the runtime image),
`grafana` (with Prometheus auto-provisioned as its datasource — no
manual UI setup needed), and Docker-level `healthcheck:` entries on
`payments-service`/`payment-processor`/`analytics-service` (hitting
their `/health` endpoints; `payment-processor` gained `/health` and
`/ready` HTTP endpoints alongside its existing `/metrics`, since it
previously had neither) round out the compose file — the architecture's
originally intended service list is now fully present and wired.

**A real networking issue found and fixed along the way**: Kafka's
single listener originally advertised itself only as `kafka:9092` —
correct for other containers, but unusable from the host (a locally-run
`go test -tags=integration`, or any developer tool), since a client
first fetches broker metadata from any reachable address and then
*reconnects using the advertised name*, which `kafka:9092` only
resolves to inside the Docker network. Fixed with a second `EXTERNAL`
listener advertised as `localhost:29092`, published as an additional
compose port — the kind of subtlety `docker compose up` alone won't
surface until something outside the Docker network actually tries to
produce/consume.

### Observability: metrics now cover every Go service

`payments-service` and `analytics-service` previously exposed no
Prometheus metrics at all; only `payment-processor` did. All three now
expose `/metrics` (HTTP request count/duration by method + **route
pattern**, never a raw path — a transaction ID must never become a
label value) plus service-specific counters: `payments_created_total`
and `outbox_events_published_total`/`outbox_events_publish_errors_total`
on `payments-service`; `kafka_messages_consumed_total` and
`clickhouse_insert_batches_total`/`clickhouse_insert_errors_total` on
`analytics-service`. `observability/prometheus/prometheus.yml` scrapes
all three; all three show as `health: up` in Prometheus's own
`/api/v1/targets`, verified live.

`payments-service` also now emits `payments_creation_failed_total{reason}`
(`validation_error`/`conflict`/`internal_error`) alongside the
previously-dead `payments_created_total`, which is incremented on every
successful `POST /api/v1/transactions` — both counters are visible from
the first request onward, not just after a prior session wired the
collector without ever calling `.Inc()`.

A Grafana dashboard, **"Cross-Border Payments — Platform Overview"**, is
provisioned automatically from
`observability/grafana/provisioning/dashboards/platform-overview.json`
(no manual import needed) with 13 panels: request rate, HTTP P95
latency, HTTP error rate, payments created, payments processed,
completed/failed payments, payment processing rate, processing
P50/P95/P99 latency, active workers, worker queue depth, Kafka messages
consumed, and Kafka processing errors. One mapping is intentionally
inexact and documented in the panel's own description: there is no
generic `kafka_processing_errors_total` metric anywhere in the system,
so "Kafka processing errors" is served by `clickhouse_insert_errors_total`
on `analytics-service` — since batch-inserting into ClickHouse is the
*only* per-message work that service does, an insert failure already
*is* the Kafka-processing failure; adding a second, parallel counter for
the same event would just be duplication.

**Why Docker healthchecks stay on `/health`, not `/ready`.** The only
compose-level consumer of any Go service's health status is
`prometheus`'s `depends_on: payment-processor: condition: service_healthy`.
Prometheus's job is to *observe* target health over time, so it should
start as soon as payment-processor's process is alive — not block on
`/ready`'s Kafka-connectivity check, which would make a slow Kafka
startup take Prometheus down with it, the one tool meant to help
diagnose exactly that kind of problem. Real dependency ordering onto
MariaDB/ClickHouse/Kafka is already handled correctly one level down, by
each infrastructure container's own healthcheck combined with
`depends_on: condition: service_healthy` on the Go services. `/ready`
therefore remains available for manual/external checks and future use,
but deliberately isn't wired into any `docker-compose.yml` healthcheck
today.

**Why `prometheus`/`grafana` have no Docker healthcheck of their own.**
Nothing in `docker-compose.yml` gates on either container's health
(`grafana`'s `depends_on: prometheus` only waits for the container to
*start*), so a healthcheck here would add complexity — and an assumption
that `wget`/`curl` exist in the upstream `prom/prometheus`/`grafana/grafana`
images — for no actual startup-ordering benefit.

**Troubleshooting a DOWN Prometheus target:**
1. `docker compose ps <service>` — confirm the container is actually running and its own healthcheck is passing.
2. `curl http://localhost:<port>/health` and `/ready` directly from the host, using the port `docker-compose.yml` publishes for that service.
3. `docker compose logs <service>` — look for a startup error (e.g. failed to bind, dependency not yet reachable).
4. Open `http://localhost:9090/targets` and read the `Error` column: DNS resolution failure usually means the container isn't running or was renamed; connection refused usually means the process hasn't started listening yet; a scrape timeout usually means the process is up but slow/stuck.
5. Confirm `observability/prometheus/prometheus.yml`'s target hostname matches the compose service name exactly — Docker's internal DNS resolves by service name, not `localhost`, and a typo here silently produces a permanent DNS-failure target.