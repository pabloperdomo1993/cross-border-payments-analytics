# cross-border-payments-analytics

A production-oriented, locally-runnable cross-border payments platform:
transaction ingestion (Go + MariaDB), asynchronous processing (Go +
Kafka, KRaft mode), and payment analytics (Go + ClickHouse).

See [docs/architecture.md](docs/architecture.md) for the full
architecture, including the reasoning behind each service's design.

## Architecture

```
React → payments-service (Go) → MariaDB
                              → Transactional Outbox → Kafka → payment-processor (Go, bounded worker pool)
                                                                        → Kafka → analytics-service (Go) → ClickHouse
payments-service / payment-processor / analytics-service → Prometheus → Grafana
```

See [docs/architecture.md](docs/architecture.md) for the full
architecture, including the reasoning behind each service's design, the
Transactional Outbox's delivery semantics, and the testing strategy.

## Services and local URLs

| Service | Language | Role | URL / port |
|---|---|---|---|
| `frontend` | React + TypeScript | Web UI — dashboard, payments table, analytics charts | http://localhost:5173 |
| `payments-service` | Go | HTTP API, OLTP writes, outbox relay | http://localhost:8080 |
| `payment-processor` | Go | Kafka consumer, bounded worker pool | http://localhost:9091 (`/health`, `/metrics`) |
| `analytics-service` | Go | Kafka consumer, ClickHouse, analytics HTTP API | http://localhost:8082 |
| `mariadb` | — | OLTP storage | localhost:3306 |
| `clickhouse` | — | OLAP storage | localhost:8123 (HTTP), localhost:9000 (native) |
| `kafka` | — | Event backbone (KRaft, no Zookeeper) | localhost:9092 (containers), localhost:29092 (host tools) |
| `prometheus` | — | Metrics | http://localhost:9090 |
| `grafana` | — | Dashboards (Prometheus datasource pre-provisioned) | http://localhost:3000 (admin/admin) |

## Requirements

Docker and Docker Compose only. No local Go, Node.js, MariaDB, Kafka,
ClickHouse, Prometheus, or Grafana installation needed to **run** the
platform. Running the Go test suites locally (outside Docker) needs a
Go toolchain per module; the race detector additionally needs a C
compiler (cgo) — see [Race detector](#race-detector) below.

## Running locally

```bash
git clone <repository>
cd cross-border-payments-analytics
cp .env.example .env
docker compose up --build
```

(`.env` isn't read automatically by Docker Compose or the services
today — see its comments; it exists as living documentation of every
environment variable each service accepts.)

`make up` / `make down` / `make clean` are equivalent shortcuts for
`docker compose up --build` / `docker compose down` / `docker compose
down -v` (the latter also removes the MariaDB/ClickHouse/Kafka/Grafana
volumes — a full local reset).

Startup order is enforced via Docker health checks, not just container
start order: `payments-service` waits on MariaDB healthy + the
`kafka-init` topic-creation job completing; `payment-processor` and
`analytics-service` wait on `kafka-init` (and, for analytics-service,
ClickHouse healthy) the same way. All three Go services expose
`/health` (process alive) and `/ready` (dependencies reachable):

```bash
curl localhost:8080/health   # payments-service
curl localhost:8080/ready
curl localhost:9091/health   # payment-processor
curl localhost:9091/ready
curl localhost:8082/health   # analytics-service
curl localhost:8082/ready
```

### Example: create a payment and watch it flow through

```bash
curl -X POST localhost:8080/api/v1/transactions \
  -H 'Content-Type: application/json' \
  -d '{"idempotency_key":"demo-1","source_country":"CO","destination_country":"US","source_currency":"COP","destination_currency":"USD","source_amount":"500.00","fx_rate":"0.00025","provider":"provider_a"}'

# A few seconds later, it's aggregated in analytics-service:
curl "localhost:8082/api/v1/analytics/corridors?source_country=CO&destination_country=US"
```

### Dashboards

Open http://localhost:3000 (admin/admin) — the **"Cross-Border Payments — Platform Overview"** dashboard is provisioned automatically on startup, no manual import needed. If a Prometheus target ever shows as down, see "Troubleshooting a DOWN Prometheus target" in `docs/architecture.md`.

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
make seed   # equivalent to: cd scripts/seed && go run . -target=both -count=1000000 -seed=42
```

Run `cd scripts/seed && go run . -h` for every flag (batch sizes, date
spread, connection overrides, smaller `-count` for a quick smoke test).

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

Each Go module (`backend/payments-service`, `backend/payment-processor`,
`backend/analytics-service`, `scripts/seed`) has its own test suite
(table-driven, hand-written fakes — no mocking framework), plus one
cross-service test module (`tests/`) for the end-to-end critical path.

```bash
make test          # plain `go test ./...` per module — no Docker required
```

or per module:

```bash
cd backend/payments-service && go test ./...
```

### Integration tests (real MariaDB / ClickHouse / Kafka)

Gated behind a build tag so they never silently run (or silently get
skipped) as part of a normal `go test ./...`. Bring up the real
dependencies first:

```bash
docker compose up -d mariadb kafka kafka-init clickhouse
make test-integration
```

This exercises: Transactional Outbox atomicity (including a real
rollback-on-outbox-failure test), MariaDB constraints, ClickHouse
aggregation queries against deterministic fixture data, two focused
Kafka flows (`payments.created` → `payment-processor`,
`payments.processed` → `analytics-service`/ClickHouse), and the
automated end-to-end test in `tests/` (POST a payment, poll
analytics-service until it's visible) — which needs the full stack:
`docker compose up -d`.

### Race detector

```bash
make test-race
```

Requires a C compiler (cgo). If this fails with a `cgo` or missing-gcc
error, install one — e.g. on Windows,
[WinLibs](https://winlibs.com/) via `winget install
BrechtSanders.WinLibs.POSIX.UCRT` (add its `mingw64/bin` to `PATH`); on
macOS, Xcode Command Line Tools (`xcode-select --install`); on Linux,
`gcc`/`build-essential` from your package manager. All four modules —
including the bounded worker pool, both outbox/Kafka consumer goroutine
lifecycles, and shutdown paths — pass `-race` cleanly.

## Stopping / resetting the environment

```bash
docker compose down      # or: make down  — stops containers, keeps volumes (data survives)
docker compose down -v   # or: make clean — also removes MariaDB/ClickHouse/Kafka/Grafana volumes
```
