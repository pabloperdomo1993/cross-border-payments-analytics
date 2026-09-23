# cross-border-payments-analytics

A production-oriented, locally-runnable cross-border payments platform:
transaction ingestion (Go + MariaDB), asynchronous processing (Go +
Kafka, KRaft mode), and payment analytics (Go + ClickHouse).

See [docs/architecture.md](docs/architecture.md) for the full
architecture, including the reasoning behind each service's design.

## Services

| Service | Language | Role | Port |
|---|---|---|---|
| `payments-service` | Go | HTTP API, OLTP writes | 8080 (HTTP) |
| `payment-processor` | Go | Kafka consumer, bounded worker pool | 9091 (metrics) |
| `analytics-service` | Go | Kafka consumer, ClickHouse, analytics HTTP API | 8082 (HTTP) |
| `frontend` | React + TypeScript | (scaffold only, not yet wired up) | 5173 |
| `mariadb` | — | OLTP storage | 3306 |
| `clickhouse` | — | OLAP storage | 8123 / 9000 |
| `kafka` | — | Event backbone (KRaft, no Zookeeper) | 9092 |
| `prometheus` | — | Metrics | 9090 |

## Running locally

Requires Docker and Docker Compose. No local Go, MariaDB, ClickHouse, or
Kafka installation needed.

```bash
docker compose up --build
```

This starts every service above. `payments-service` and
`analytics-service` become available once their dependencies report
healthy; `payment-processor` and `analytics-service` join their Kafka
consumer groups once the `kafka-init` one-shot job has created the
required topics.

```bash
curl localhost:8080/health
curl localhost:8082/health
```

## Database & Analytics

This project treats SQL and data modeling as a first-class concern, not
an afterthought — two different databases are used deliberately, for
two different jobs:

- **MariaDB → OLTP.** `payments-service`'s `transactions` table is the
  operational source of truth: one row per payment, `DECIMAL` monetary
  columns (never `FLOAT`), a unique `idempotency_key` so retried
  creation requests can't produce duplicates, and an index shaped around
  actual query patterns rather than one per column. See
  [docs/architecture.md §6](docs/architecture.md) and the migrations
  under `backend/payments-service/migrations/` for the full schema
  history and reasoning.

- **ClickHouse → OLAP.** `analytics-service`'s `payments_analytics`
  table is a purpose-built analytical schema — `MergeTree`, monthly
  partitions, an `ORDER BY` key chosen for corridor-shaped queries,
  `LowCardinality` dimension columns — fed by a Kafka consumer that
  batches inserts rather than writing row-by-row. It is **not** a copy
  of the MariaDB schema; see the extensive comments in
  `backend/analytics-service/migrations/001_create_payments_analytics.sql`
  for why each choice was made.

### Supported analytics

All under `GET /api/v1/analytics/...` on `analytics-service` (port
8082), each accepting the shared filters `from`, `to` (dates,
`YYYY-MM-DD`), `source_country`, `destination_country`,
`source_currency`, `destination_currency`, `provider`, and `status`:

- **Volume by corridor** — `GET /corridors`
- **Volume by currency** — `GET /currencies`
- **Volume by country** — `GET /countries?by=source|destination`
- **Provider performance & failure rates** — `GET /providers`
- **Time-series payment volume** — `GET /timeseries?interval=day`

Example:

```bash
curl "localhost:8082/api/v1/analytics/corridors?from=2026-09-01&to=2026-09-30"
curl "localhost:8082/api/v1/analytics/providers?provider=provider_a"
```

### Generating a benchmark dataset

`scripts/seed` is a standalone tool that generates a large, deterministic
synthetic payments dataset and loads it directly into MariaDB and/or
ClickHouse (bypassing Kafka — see
[docs/architecture.md §6](docs/architecture.md) for why). The same
`-seed` value always reproduces the same dataset.

```bash
cd scripts/seed
go build -o seed .
./seed -target=both -count=1000000 -seed=42
```

Run `./seed -h` for every flag (batch sizes, date spread, connection
overrides).

### Performance experiments

Two documents contain **real, measured** results against a 1,000,000
-row dataset — no fabricated numbers:

- [docs/performance/sql-optimization.md](docs/performance/sql-optimization.md) —
  a MariaDB before/after index experiment, including an honest,
  unflattering result explaining exactly when this kind of index does
  and doesn't help.
- [docs/performance/mariadb-vs-clickhouse.md](docs/performance/mariadb-vs-clickhouse.md) —
  the same aggregation query run on equivalent datasets in both engines,
  framed as an OLTP-vs-OLAP illustration rather than a "which database
  wins" comparison.

## Tests

Each Go module has its own test suite (table-driven, hand-written fakes
— no mocking framework):

```bash
cd backend/payments-service && go test ./...
cd backend/payment-processor && go test ./...
cd backend/analytics-service && go test ./...
cd scripts/seed && go test ./...
```
