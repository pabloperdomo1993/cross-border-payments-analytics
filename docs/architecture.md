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

Implementation status: `payments-service` (HTTP + MariaDB) and `payment-processor` (Kafka consumer/worker pool + producer) are implemented, per section 5 below. Analytics Service and ClickHouse are not yet implemented. `payments-service` does not yet publish to `payments.created` — see section 5's limitations.

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