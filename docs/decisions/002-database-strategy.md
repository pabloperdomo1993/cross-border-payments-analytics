# ADR-002: Database Strategy — MariaDB for OLTP, ClickHouse for OLAP

Status: Accepted

## Context

The system has two workloads with genuinely different access patterns:
recording individual payments reliably and correctly (transactional),
and answering aggregate questions over potentially millions of payments
— by corridor, currency, provider, and time — quickly (analytical). A
single database engine optimizes for one of these at the expense of the
other.

## Options

### Option 1: MariaDB only

Use MariaDB for both operational storage and analytics queries.

Advantages:
- One database to run, back up, and reason about.
- Strong transactional guarantees are available everywhere, including
  for the analytics queries.

Disadvantages:
- Row-store engines like InnoDB (MariaDB's default) scan and aggregate
  large historical ranges far slower than a column-store built for that
  purpose — `docs/performance/mariadb-vs-clickhouse.md` measures a
  ~150x difference on the same corridor-aggregation query against
  equivalent datasets.
- Analytical query load (full-table scans, large `GROUP BY`s) competes
  for the same resources as the operational write path, risking
  operational latency regressions under analytical load.

### Option 2: ClickHouse only

Use ClickHouse for both operational writes and analytics.

Advantages:
- Excellent aggregate query performance across the board.
- One database to run for the read side.

Disadvantages:
- ClickHouse is not designed for the operational write pattern this
  system needs: frequent small single-row inserts/updates with strong
  per-row transactional guarantees (e.g. a unique idempotency-key
  constraint enforced atomically on insert). It's built for bulk,
  append-mostly ingestion.
- No general-purpose transactions in the sense payment state changes
  need (e.g. atomically writing a payment row and an outbox row in the
  same commit — see the Transactional Outbox in `docs/architecture.md §7`).

### Option 3: MariaDB + ClickHouse (one for each workload)

Advantages:
- Each engine is used for what it's actually good at: MariaDB for
  correctness-critical, low-latency, per-row operational writes;
  ClickHouse for high-volume aggregate reads.
- Each can be scaled, tuned, and reasoned about independently.

Disadvantages:
- Two databases to operate instead of one.
- Data has to move between them (here, via Kafka), which introduces
  a real trade-off — see below.

## Decision

Use MariaDB for OLTP and ClickHouse for OLAP — this is already
implemented, not a future plan.

**MariaDB** (`payments-service`, database `payments`) is the source of
truth for **payment state**:
- The `transactions` table is the single authoritative record of a
  payment, with `DECIMAL` monetary columns (never `FLOAT`), a unique
  `idempotency_key` index enforcing that a retried creation request
  can't produce a duplicate row, and an index shaped around actual
  query patterns (see `docs/architecture.md §6`).
- **Transactional consistency and idempotency** are enforced here, not
  downstream: the unique constraint is the actual mechanism preventing
  duplicate payments, not an application-level check that could race.
- **Outbox records**: the `outbox_events` table is written in the exact
  same SQL transaction as the `transactions` row it describes, so a
  payment can never exist without its corresponding queued event (or
  vice versa) — see `docs/architecture.md §7`.
- Operational queries (look up a payment by id, check its current
  status) are served here, against a small, hot, well-indexed table —
  not against the analytical store.

**ClickHouse** (`analytics-service`, database `analytics`) holds
**historical, analytical facts**:
- `payments_analytics`, a `MergeTree` table purpose-built for the
  queries this system actually runs: volume by corridor, by currency, by
  country, by provider, and time-series volume — see the schema
  reasoning in `backend/analytics-service/migrations/001_create_payments_analytics.sql`
  and `docs/architecture.md §6`.
- It is fed by a Kafka consumer that batches inserts (never row-by-row —
  ClickHouse is explicitly not designed for that write pattern).
- It is **not** a copy of the MariaDB schema — it's shaped for
  aggregation, with `LowCardinality` dimension columns and an `ORDER BY`
  chosen for the corridor-volume query this system leads with.

## Trade-offs

The trade-off is explicit and unavoidable given this split: **eventual
consistency** between the two stores. A payment can be `completed` in
MariaDB — visible immediately via `payments-service`'s API — before that
same payment's outcome has propagated through Kafka
(`payments.created` → `payment-processor` → `payments.processed`) and
been batch-inserted into ClickHouse. The delay is bounded by the outbox
relay's poll interval (`OUTBOX_POLL_INTERVAL`, default 500ms),
`payment-processor`'s processing time, and `analytics-service`'s batch
flush interval (`ANALYTICS_BATCH_MAX_INTERVAL`, default 2s) — typically
low seconds end-to-end, but never instantaneous, and never guaranteed by
any distributed transaction across the two stores. We accept this
because the alternative — a single store trying to serve both workloads
well, or a synchronous write to both stores in the request path — either
degrades one workload to serve the other, or introduces a distributed
transaction across two different database engines, which is
significantly more complex and fragile than accepting a small, bounded,
well-understood replication lag. This mirrors the same at-least-once,
not-exactly-once trade-off already documented for the Transactional
Outbox itself (`docs/architecture.md §7`).
