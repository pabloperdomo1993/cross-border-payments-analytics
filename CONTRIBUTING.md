# Contributing

## Development Requirements

This project is **local-first**: the entire platform — frontend, three Go
services, Kafka (KRaft), MariaDB, ClickHouse, Prometheus, and Grafana —
runs via Docker Compose, with no cloud account, no Kubernetes cluster,
and no external service required to get a working environment.

To run the platform, you need:

- Git
- Docker
- Docker Compose
- Make (optional — every `make` target is a thin wrapper around a
  `docker compose` or `go test` command; see the [Makefile](Makefile) if
  you'd rather run the underlying command directly)

You do **not** need a local Go toolchain, Node.js, MariaDB, Kafka,
ClickHouse, Prometheus, or Grafana installation to run the platform.
Running the Go test suites *outside* Docker (e.g. from your editor) does
need a Go toolchain per module, and the race detector additionally needs
a C compiler — see [README.md — Race detector](README.md#race-detector).

## Getting Started

```bash
git clone <repository>
cd cross-border-payments-analytics
cp .env.example .env   # not read automatically by Docker Compose or the
                        # services — it exists as living documentation of
                        # every environment variable each service accepts
make up                 # equivalent to: docker compose up --build
```

Once the stack is up, see [README.md](README.md) for the full list of
service ports, `/health`/`/ready` curl examples, and an example payment
you can POST to watch the whole pipeline run.

```bash
make down    # stop containers, keep volumes (data survives)
make clean   # docker compose down -v — also removes MariaDB/ClickHouse/Kafka/Grafana volumes
make logs    # docker compose logs -f
```

## Repository Structure

```
backend/
  payments-service/     Go — HTTP API, MariaDB writes, transactional outbox relay
  payment-processor/    Go — Kafka consumer, bounded worker pool, Kafka producer
  analytics-service/    Go — Kafka consumer, ClickHouse writes, analytics HTTP API
frontend/                React + TypeScript + Vite — dashboard, payments table, analytics charts
observability/
  prometheus/            Prometheus scrape configuration
  grafana/provisioning/  Auto-provisioned Grafana datasource + dashboards
infrastructure/           Reserved for infrastructure-as-code (currently empty — no
                          Terraform or other IaC exists in this repository yet)
scripts/seed/             Standalone Go module: generates a deterministic synthetic
                          dataset directly into MariaDB/ClickHouse for benchmarking
tests/                    Root-level Go module: one automated end-to-end test that
                          exercises the full HTTP → Kafka → ClickHouse pipeline
docs/
  architecture.md         The system's architecture, in detail — read this first
  testing-strategy.md     What is and isn't tested, and how
  decisions/              Architecture Decision Records (see below)
  performance/            Real, measured performance experiments (no fabricated numbers)
```

`backend/payments-service`, `backend/payment-processor`, `backend/analytics-service`,
`scripts/seed`, and `tests/` are five **independent Go modules** (each has
its own `go.mod`) — there is no shared Go module or package between the
three services. See [docs/decisions/004-monorepo.md](docs/decisions/004-monorepo.md)
for why the repository is nonetheless organized as one monorepo, and what
that deliberately does *not* mean for how the services are coupled.

### Service boundaries

- **payments-service** owns payment *ingestion*: validates and persists
  a transaction to MariaDB, and — in the same database transaction —
  writes an outbox row that a background relay later publishes to Kafka
  (`payments.created`). It never talks to Kafka synchronously on the
  request path.
- **payment-processor** owns payment *processing*: consumes
  `payments.created`, runs each event through a bounded worker pool, and
  publishes the outcome to `payments.processed` or `payments.dlq`. It
  has no HTTP API beyond `/health`, `/ready`, `/metrics`.
- **analytics-service** owns *analytics*: consumes `payments.processed`/
  `payments.dlq`, batches inserts into ClickHouse, and serves the
  read-side analytics HTTP API (corridors, currencies, countries,
  providers, time series).

See [docs/architecture.md](docs/architecture.md) for the full reasoning
behind each boundary, including the Kafka offset/concurrency strategy
and the outbox pattern's delivery semantics.

## Development Workflow

1. Branch from `main`.
2. Implement a focused change — one logical concern per branch.
3. Add or update tests for the change (see
   [docs/testing-strategy.md](docs/testing-strategy.md) for what test
   level fits what kind of change).
4. Run the relevant tests locally (see Code Quality below).
5. Run linting/formatting for whichever part of the stack you touched.
6. Update documentation when the change affects behavior described in
   `README.md`, `docs/architecture.md`, or an existing ADR.
7. Open a Pull Request.

## Branch Naming

```
feature/payment-ingestion
feature/corridor-analytics
fix/kafka-consumer-retry
test/payment-idempotency
docs/architecture-update
chore/dependency-update
```

## Commit Convention

This project uses [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(payments): add payment creation endpoint
feat(analytics): add corridor aggregation
fix(processor): prevent duplicate event processing
test(payments): add idempotency tests
docs(architecture): document Kafka processing
chore(docker): update local infrastructure
```

## Pull Request Requirements

A PR should:

- Explain **what** changed and **why**.
- Include appropriate tests for the change.
- Pass CI when CI exists for this repository (no CI pipeline is
  currently configured — see [docs/testing-strategy.md](docs/testing-strategy.md)).
- Avoid unrelated changes — keep the diff focused on the stated purpose.
- Update documentation when the change affects it.
- Document significant architecture decisions as an ADR (see below).

## Code Quality

### Go

Each Go module (`backend/payments-service`, `backend/payment-processor`,
`backend/analytics-service`, `scripts/seed`, `tests`) is checked
independently — run these from inside the module directory, or use the
equivalent `make` target:

```bash
go fmt ./...
go vet ./...
go test ./...          # or: make test — runs this for every module, no Docker required
go test -race ./...    # or: make test-race — requires a C compiler (cgo)
```

Integration tests (real MariaDB/Kafka/ClickHouse) are gated behind a
build tag so they never run silently:

```bash
docker compose up -d mariadb kafka kafka-init clickhouse
make test-integration   # go test -tags=integration ./... per module
```

### Frontend

From `frontend/`, using the scripts actually defined in
[`frontend/package.json`](frontend/package.json):

```bash
npm run lint
npm run test         # vitest run
npm run test:watch   # vitest, watch mode
npm run build        # tsc -b && vite build
```

## Architecture Changes

Significant technical decisions — ones with real trade-offs, not routine
implementation details — should be documented as an Architecture
Decision Record under [`docs/decisions/`](docs/decisions/). See the
existing ADRs there for the expected structure (Context, Options with
advantages/disadvantages, Decision, Trade-offs). An ADR is warranted
when a reasonable engineer could have chosen differently and future
readers would benefit from knowing why this repository didn't.
