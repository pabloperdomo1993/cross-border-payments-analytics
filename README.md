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

Only two URLs matter for actually *using* the platform. Everything else
below is infrastructure the app depends on, or tooling for developers —
not something you open and interact with directly.

| | URL | What it actually is |
|---|---|---|
| 🖥️ **The app** | **http://localhost:5173** | The product: React dashboard, payments table, analytics charts. This is `frontend` — open this one. |
| 📊 **Dashboards** | **http://localhost:3000**<br>user: `admin`<br>password: `admin` | This is `grafana`, a separate *monitoring* tool — not the app. It shows operational metrics about the *system itself* (request rates, latency, Kafka throughput, worker activity), already set up with no manual configuration. You'd open this to check the platform's health, not to use the platform. |

`:5173` and `:3000` are unrelated pipelines that happen to run side by
side: the frontend talks directly to `payments-service` (`:8080`) and
`analytics-service` (`:8082`) for its data. Grafana never talks to the
frontend, and the frontend never talks to Grafana — Grafana instead
reads from `prometheus` (`:9090`), which independently scrapes metrics
from the three Go services. Two parallel, independent systems sharing
one `docker compose up`, not one feeding into the other.

Everything else, for reference:

| Service | Language | Role | URL / port |
|---|---|---|---|
| `payments-service` | Go | HTTP API, OLTP writes, outbox relay | http://localhost:8080 |
| `payment-processor` | Go | Kafka consumer, bounded worker pool | http://localhost:9091 (`/health`, `/metrics`) |
| `analytics-service` | Go | Kafka consumer, ClickHouse, analytics HTTP API | http://localhost:8082 |
| `mariadb` | — | OLTP storage | localhost:3306 |
| `clickhouse` | — | OLAP storage | localhost:8123 (HTTP), localhost:9000 (native) |
| `kafka` | — | Event backbone (KRaft, no Zookeeper) | localhost:9092 (containers), localhost:29092 (host tools) |
| `prometheus` | — | Metrics storage Grafana reads from | http://localhost:9090 |

## Running locally

**Prerequisites**: just Docker and Docker Compose. Nothing else needs to
be installed on your machine to run the platform.

```bash
git clone <repository>
cd cross-border-payments-analytics
cp .env.example .env
docker compose up --build
```

That's the whole setup. The first run takes a few minutes (it builds the
frontend and all three Go services from source); after that, startup
takes seconds. Once the command settles and stops printing new logs:

- **Open the app** → http://localhost:5173
- **Open the dashboards** (optional) → http://localhost:3000, user `admin` / password `admin` — see [Services and local URLs](#services-and-local-urls) above for what this is

If you'd rather not type the full `docker compose` command, `make up`
does the same thing (see the [Makefile](Makefile)).

<details>
<summary>About <code>.env.example</code></summary>

There's also a `.env.example` file listing every environment variable
each service accepts (ports, credentials, tuning knobs). It's **not**
read automatically by Docker Compose or by any service today — the real
defaults live directly in `docker-compose.yml`. Copy it to `.env` only
if you want a single place to read what's configurable; editing `.env`
alone won't change anything unless you also update `docker-compose.yml`
to use it.

</details>

### Verifying the backend is healthy (optional)

Docker health checks — not just container start order — gate startup:
`payments-service` waits on MariaDB being healthy and Kafka's topics
existing; `payment-processor` and `analytics-service` wait on Kafka
(and, for analytics-service, ClickHouse) the same way. So by the time
`docker compose up` settles, the backend should already be ready — but
if you want to check directly, all three Go services expose `/health`
(process alive) and `/ready` (dependencies reachable):

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

> If a Prometheus target ever shows as down in Grafana, see
> "Troubleshooting a DOWN Prometheus target" in
> [docs/architecture.md](docs/architecture.md).

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

Running the app (above) only needs Docker. Running the Go test suites
directly on your machine (outside a container) additionally needs a Go
toolchain per module; the race detector further needs a C compiler
(cgo) — see [Race detector](#race-detector) below.

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
