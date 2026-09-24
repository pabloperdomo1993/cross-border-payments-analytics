# Code Quality Review — Clean Architecture, SOLID, Clean Code

This is a manual, evidence-based review of how well this repository
applies Clean Architecture, SOLID, and clean-code practices across the
backend, frontend, and infrastructure. Every finding below cites a real
file (and, where useful, a real symbol name) — nothing here is a generic
checklist item disconnected from this codebase. See **Scope and
limitations** at the end for what this review does and doesn't cover.

## Executive summary

| Area | Verdict |
|---|---|
| **Backend** | Genuinely strong Clean Architecture and SOLID adherence — proper dependency direction, rich domain value objects, narrow consumer-defined interfaces. Two real gaps found, one of them functional (not just style): transaction status is never updated after creation, and one repository interface is too broad for one of its two consumers. |
| **Frontend** | Solid feature-oriented architecture with a clean data-access boundary. Two moderate findings: duplicated filter/pagination state logic across two pages, and no error boundary anywhere in the app. |
| **Infra** | Docker practice is already good (multi-stage builds, non-root users, pinned image versions, no baked-in credentials). Findings here are optional hardening, not defects. |

Nothing found in this review contradicts the architecture already
documented in [`docs/architecture.md`](architecture.md) or the decisions
recorded in [`docs/decisions/`](decisions/) — see **What this review
doesn't re-litigate** below.

---

## 1. Backend: Clean Architecture & SOLID

### Layering — assessed against `payments-service` in depth

The dependency direction is respected throughout:

```
handlers/http  →  application  →  repository (interface)
                        ↓                ↑
                     domain      repository/mariadb (implementation)
```

- **`internal/domain`** (`transaction.go`, `errors.go`, `outbox.go`) has
  zero imports of any database driver, HTTP framework, or messaging
  library. It's pure business logic: value objects (`Money`, `FXRate`,
  `CountryCode`, `CurrencyCode`, `TransactionID`, `IdempotencyKey`) each
  own their own validation, monetary arithmetic goes through `math/big`
  specifically to avoid `float64` precision loss (`Money.Multiply`,
  `transaction.go:116-144`), and status transitions are governed by an
  explicit `allowedTransitions` adjacency map (`transaction.go:301-305`)
  rather than scattered `if` checks — an invalid transition like
  `completed → failed` is structurally impossible to perform through the
  `Transaction` API, not just discouraged by convention.
- **`internal/application`** (`create_transaction.go`, `get_transaction.go`)
  depends only on `repository.TransactionRepository`, an interface — it
  has no idea MariaDB exists. This is the Dependency Inversion Principle
  applied correctly: the high-level policy (use cases) doesn't depend on
  the low-level detail (a SQL driver); both depend on an abstraction.
- **`internal/handlers/http`** goes a step further than depending on the
  application layer's concrete types: `transaction_handler.go:19-27`
  declares its *own* narrow `CreateTransactionUseCase`/
  `GetTransactionUseCase` interfaces (each a single `Execute` method)
  rather than importing `*application.CreateTransaction` directly. This
  is genuine Interface Segregation *and* Dependency Inversion — the HTTP
  layer defines the interface it needs, and the application layer
  happens to satisfy it, rather than the application layer's full public
  surface leaking into the transport layer.
- `payment-processor` and `analytics-service` follow the same shape
  (`cmd/{api,processor}` + `internal/{config,domain,repository,handlers}`)
  and were reviewed in depth during this session's earlier observability
  work — their Kafka/worker-pool and Kafka/ClickHouse layers are
  well-designed with consumer-defined interfaces (`Publisher`, `Pool`,
  `Inserter`) for the same reason. No new findings there beyond what's
  already recorded in [`docs/decisions/001-bounded-worker-pool.md`](decisions/001-bounded-worker-pool.md).

This is a strong foundation. The findings below are real, but they're
gaps in an otherwise well-structured codebase, not evidence of a
different overall problem.

### Finding 1 — Interface Segregation: `TransactionRepository` is too broad for one of its two consumers

**File**: `backend/payments-service/internal/repository/transaction_repository.go:20-46`

```go
type TransactionRepository interface {
	Create(ctx context.Context, transaction *domain.Transaction, event *domain.OutboxEvent) error
	FindByID(ctx context.Context, id string) (*domain.Transaction, error)
	UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, updatedAt time.Time) error
	FetchPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error)
	MarkOutboxEventPublished(ctx context.Context, id string, publishedAt time.Time) error
}
```

One interface covers two distinct responsibilities — transaction
persistence (`Create`, `FindByID`, `UpdateStatus`) and outbox-queue
management (`FetchPendingOutboxEvents`, `MarkOutboxEventPublished`).
`GetTransaction` (`internal/application/get_transaction.go:12-19`) takes
a `repository.TransactionRepository` but calls only `FindByID` — it is
forced to depend on four methods it never uses, including two that
belong to a completely different concern (the outbox relay's polling
loop, not transaction retrieval).

**Why this matters, concretely**: a future change to the outbox methods'
signatures (e.g. adding a claim-token parameter for multi-instance
polling, which `FetchPendingOutboxEvents`'s own doc comment already
anticipates) would force `GetTransaction`'s fake-repository test double
(`fake_repository_test.go`) to implement methods it has nothing to do
with, purely to satisfy the interface. That's the concrete cost of an
Interface Segregation Principle violation, not an abstract one.

**Proposed fix** (not implemented — described): split the interface
along its two real responsibilities, e.g.:

```go
type TransactionStore interface {
	Create(ctx context.Context, transaction *domain.Transaction, event *domain.OutboxEvent) error
	FindByID(ctx context.Context, id string) (*domain.Transaction, error)
	UpdateStatus(ctx context.Context, id string, status domain.TransactionStatus, updatedAt time.Time) error
}

type OutboxStore interface {
	FetchPendingOutboxEvents(ctx context.Context, limit int) ([]*domain.OutboxEvent, error)
	MarkOutboxEventPublished(ctx context.Context, id string, publishedAt time.Time) error
}
```

`GetTransaction` would then depend on a `TransactionReader`-shaped
interface (or just `TransactionStore` if `UpdateStatus`/`Create` are
kept together), and the outbox relay (`internal/outbox/relay.go`) would
depend on `OutboxStore` alone. The single `mariadb.TransactionRepository`
struct can still implement both interfaces — this is a consumer-side
interface split, not a storage-layer redesign.

### Finding 2 — Dead code and an incomplete flow: transaction status is never updated after creation

**Files**: `backend/payments-service/internal/repository/transaction_repository.go:31` (`UpdateStatus`),
`backend/payments-service/internal/domain/transaction.go:429-437` (`MarkCompleted`/`MarkFailed`)

This is the most important finding in this review — a functional gap,
not a style preference.

`UpdateStatus` (repository interface + MariaDB implementation) and
`Transaction.MarkCompleted`/`MarkFailed` (domain) are fully implemented
and covered by unit/integration tests. A repository-wide search confirms
they are called **only from test files** — no production code path
(`internal/application`, `internal/handlers/http`, `internal/outbox`)
ever calls any of them.

The practical consequence: once `payments-service` creates a transaction,
it is written to MariaDB with `status = "pending"` and **stays
`"pending"` forever**, no matter what `payment-processor` later
determines. `payment-processor` publishes the real outcome
(`completed`/`failed`) to the `payments.processed` topic, and
`analytics-service` consumes it into ClickHouse correctly — but nothing
in `payments-service` consumes `payments.processed` to reflect that
outcome back into its own `transactions` table. `GET
/api/v1/transactions/{id}` will report `"pending"` for a payment that
completed successfully an hour ago.

This is also a clean-code issue independent of the functional gap:
`UpdateStatus`/`MarkCompleted`/`MarkFailed` are dead production code —
implemented and tested for a flow that was never wired end-to-end. Code
that exists only to be unit-tested, with no caller, is a maintenance
liability (it can silently rot, and its tests give false confidence that
the *feature* works, when only the *unit* does).

**Proposed fix** (not implemented — described, two options depending on
how much scope is acceptable):

1. **Minimal**: if updating MariaDB from the processing outcome is out
   of scope for now, remove the unused `UpdateStatus`/
   `MarkCompleted`/`MarkFailed` code paths (or explicitly mark them as
   intentionally-unused scaffolding for a documented future phase) so
   the codebase doesn't imply a capability that isn't wired up.
2. **Complete the flow**: add a small Kafka consumer to
   `payments-service` (or a dedicated responsibility, to avoid growing
   the HTTP service into also being a consumer) that subscribes to
   `payments.processed`/`payments.dlq` and calls
   `UpdateStatus`/`MarkCompleted`/`MarkFailed` to bring the OLTP record
   in line with the actual outcome. This would make `GET
   /api/v1/transactions/{id}` reflect reality and would be a natural,
   already-supported extension point — the domain and repository layers
   are already built for it; only the wiring is missing.

### Finding 3 — Consistency: two different configuration philosophies for the same problem

**Files**: `backend/payments-service/cmd/api/main.go:39-91` vs.
`backend/payment-processor/internal/config/config.go` /
`backend/analytics-service/internal/config/config.go`

`payment-processor` and `analytics-service` both load configuration
through a dedicated `internal/config` package with `Load() (Config,
error)` — every environment variable is parsed and validated (positive
counts, positive durations), and all validation errors are collected and
returned together so a misconfigured deployment fails fast with a
complete list of what's wrong, before the service does anything else.

`payments-service` does not have an `internal/config` package at all —
`loadConfig()` lives inline in `cmd/api/main.go:53-66`, returns a bare
`config` struct with no error, and every `getEnv`/`getDuration`/`getInt`
helper (`main.go:68-91`) silently falls back to a default on a parse
failure rather than failing fast. A malformed `OUTBOX_POLL_INTERVAL`
(e.g. a typo like `"500m"` instead of `"500ms"`) would silently fall
back to the default instead of refusing to start — the opposite of the
other two services' behavior for the identical class of problem.

**Why this matters**: this isn't wrong in isolation (it works), but it's
an inconsistency an evaluator — or a new contributor moving between
services — will notice immediately, and it means `payments-service` is
the one service in the system that can start successfully with a
configuration mistake it should have refused.

**Proposed fix** (not implemented — described): extract
`payments-service`'s config loading into its own `internal/config`
package mirroring the other two services' `Load() (Config, error)`
shape, validating at minimum that durations/counts parse successfully
and that required values (`DB_PASSWORD`, `KAFKA_BROKERS`) aren't empty.

---

## 2. Frontend: architecture & clean code

The frontend is organized as a feature-oriented architecture:
`features/{analytics,dashboard,payments}/{api,hooks,components,types.ts}`,
a single centralized typed API client (`lib/apiClient.ts`) that
components never bypass, and TanStack Query owning all server state (no
`useEffect`+`fetch` anywhere). This is a clean, conventional structure
with a real data-access boundary — components depend on hooks, hooks
depend on feature `api/` modules, feature `api/` modules depend on the
one shared `apiClient` — matching the dependency direction described
when this was built.

### Finding 4 — DRY: identical filter/pagination state duplicated across two pages

**Files**: `frontend/src/pages/DashboardPage.tsx:16-26`,
`frontend/src/pages/PaymentsPage.tsx:13-23`

Both files independently declare the same three pieces of state and the
same handler:

```tsx
const [filters, setFilters] = useState<AnalyticsFilters>(EMPTY_FILTERS)
const [page, setPage] = useState(1)
const [pageSize, setPageSize] = useState(PAGE_SIZE)

function handleFiltersChange(next: AnalyticsFilters) {
  setFilters(next)
  setPage(1)
}
```

This is byte-for-byte the same logic in two places (differing only in
`PAGE_SIZE`'s value, 10 vs. 20). It works today, but any future change
to this behavior (e.g. also resetting `pageSize` on filter change, or
persisting filters to the URL) has to be made twice and will drift if
one call site is missed.

**Proposed fix** (not implemented — described): extract a small shared
hook, e.g. `usePaymentsListState(defaultPageSize: number)` returning
`{ filters, page, pageSize, setPageSize, handleFiltersChange, setPage }`,
used by both `DashboardPage` and `PaymentsPage`. `AnalyticsPage` (which
only holds `filters`, no pagination) would not need this hook, since it
doesn't share the duplicated shape.

### Finding 5 — No error boundary anywhere in the app

**Scope**: `frontend/src` (confirmed by absence — no `componentDidCatch`,
no `react-error-boundary` usage, no `<ErrorBoundary>` component exists).

Every async state (loading/error/empty) is handled carefully at the
query level (`LoadingState`/`ErrorState`/`EmptyState`, per the
dashboard's own design), but there is no top-level (or per-route)
boundary catching a genuine *render* error — a bug in a component's
render logic (not a failed fetch) would currently take down the entire
React tree to a blank white page, with no fallback UI.

**Proposed fix** (not implemented — described): a single
`<ErrorBoundary>` wrapping `<RouterProvider>` in `app/App.tsx`, rendering
a minimal "something went wrong" fallback with a reload action. This is
a small, self-contained addition that doesn't touch any existing
component.

---

## 3. Infra: Docker & Compose practices

What's already good, confirmed directly from the files:

- **No `latest` tags anywhere** in `docker-compose.yml` — every image is
  version-pinned (`mariadb:11`, `apache/kafka:3.8.0`,
  `prom/prometheus:v2.55.1`, `clickhouse/clickhouse-server:24.8`,
  `grafana/grafana:11.3.0`), which makes the local stack reproducible
  rather than silently drifting on rebuild.
- **Multi-stage Dockerfiles** for all three Go services (build stage on
  `golang:1.25-alpine`, runtime on bare `alpine:3.20`) — no Go toolchain
  ships in the runtime image.
- **Non-root runtime user** in every Go service's Dockerfile
  (`addgroup -S app && adduser -S app -G app` followed by `USER app`).
- **No credentials baked into any image** — every service is configured
  entirely through environment variables at runtime, documented in
  `.env.example`.
- **`CGO_ENABLED=0`** static builds, consistent `restart: unless-stopped`
  across every service, and healthchecks gating startup ordering.

### Finding 6 (optional hardening) — base images pinned by tag, not by digest

Tags like `golang:1.25-alpine` and `alpine:3.20` can point to a
different actual image over time (a tag is mutable; a digest is not).
For a project explicitly valuing reproducibility (per
`docs/decisions/004-monorepo.md`'s "one local environment" rationale),
pinning to a digest (`golang:1.25-alpine@sha256:...`) would close that
gap. This is a nice-to-have for supply-chain hardening, not a defect —
tag-pinning already puts this repository ahead of an unpinned or
`latest`-tagged baseline.

### Finding 7 (limitation, not a flaw) — no network segmentation in `docker-compose.yml`

Every service shares Compose's default network — there's no tiering
(e.g. a database-only network `payments-service`/`mariadb` share that
`grafana` isn't on). For a project whose explicit, stated goal is a
single-command local developer environment (not a production
deployment target), this is a reasonable simplification, not an
oversight — but it's worth naming honestly, since an evaluator
comparing this to a production-shaped Compose file might otherwise
wonder if it was overlooked. It would need to be addressed before this
Compose file was ever used as a template for anything beyond local
development.

---

## Prioritized improvement list

| # | Finding | Area | Severity | Effort |
|---|---|---|---|---|
| 2 | Transaction status never updated after creation; `UpdateStatus`/`MarkCompleted`/`MarkFailed` are dead code | Backend | **High** — functional gap, not style | Medium (requires a new consumer, or a deliberate decision to remove the dead code) |
| 3 | `payments-service` config isn't fail-fast, unlike the other two services | Backend | Medium — consistency/robustness | Small |
| 1 | `TransactionRepository` interface too broad for `GetTransaction` | Backend | Medium — ISP violation | Small |
| 4 | Duplicated filter/pagination state across `DashboardPage`/`PaymentsPage` | Frontend | Low-Medium — DRY debt | Small |
| 5 | No React error boundary anywhere | Frontend | Medium — resilience gap | Small |
| 6 | Base images pinned by tag, not digest | Infra | Low — optional hardening | Small |
| 7 | No Docker network segmentation | Infra | Low — acceptable for local-only, named for completeness | N/A (deliberate) |

---

## What this review doesn't re-litigate

Architecture decisions that are already made and explicitly documented
are not re-argued here — see:

- [`docs/decisions/001-bounded-worker-pool.md`](decisions/001-bounded-worker-pool.md) —
  why a bounded worker pool over sequential or unbounded-goroutine
  processing.
- [`docs/decisions/002-database-strategy.md`](decisions/002-database-strategy.md) —
  why MariaDB (OLTP) + ClickHouse (OLAP), and the accepted
  eventual-consistency trade-off.
- [`docs/decisions/003-observability.md`](decisions/003-observability.md) —
  why Prometheus + Grafana, and exactly which metrics exist today.
- [`docs/decisions/004-monorepo.md`](decisions/004-monorepo.md) — why one
  repository, and why that doesn't mean shared business logic.

This review's findings are about **implementation-level** SOLID/clean-code
details within those already-decided architectural choices, not about
the choices themselves.

## Scope and limitations

This was a manual, evidence-based review of representative code — it
read `payments-service` in full depth (all of `internal/domain`,
`internal/application`, `internal/repository`, `internal/handlers/http`,
and `cmd/api/main.go`), and relied on this session's own prior deep work
on `payment-processor`'s worker pool/Kafka layer and `analytics-service`'s
Kafka/ClickHouse layer for those two services' shape, rather than
re-reading every file in all three services line by line. It is **not**:

- An automated static-analysis or linter pass (no `golangci-lint`,
  `staticcheck`, or equivalent was run as part of this review).
- A security audit (dependency vulnerability scanning, secrets
  scanning, or threat modeling are out of scope here).
- Exhaustive coverage of the frontend's every component or the full
  `docker-compose.yml` line by line.

Every finding above is backed by an actual file and, where applicable,
an actual grep confirming a claim (e.g. "never called from production
code" for Finding 2) rather than an assumption — consistent with this
project's own established convention of never stating something works,
or doesn't, without checking (see `docs/performance/*.md` and
`docs/testing-strategy.md` for the same discipline applied to
performance and test-coverage claims).
