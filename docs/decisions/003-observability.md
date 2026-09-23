# ADR-003: Observability via Prometheus + Grafana

Status: Accepted

## Context

The system's critical path is asynchronous and multi-stage:
`payments-service` → Kafka → `payment-processor` → Kafka →
`analytics-service`. A failure or slowdown at any stage isn't visible
from any single service's own logs — a payment can be accepted by
`payments-service` and then silently stall in `payment-processor` or
never reach ClickHouse, with nothing in any one process's output making
that obvious. The system also must run entirely locally
(`docker compose up --build`), with no cloud account required.

## Options

### Option 1: Logs only

Rely on each service's structured logs (`log/slog`, JSON output).

Advantages:
- Already present in every service, zero additional infrastructure.
- Good for understanding a single request/event's specific journey
  after the fact, given a correlation ID.

Disadvantages:
- No aggregate view: answering "what's the current error rate" or "is
  the worker pool saturated right now" requires grepping/aggregating
  logs by hand, which doesn't scale as a monitoring practice.
- No time-series view of a numeric value (latency, queue depth) without
  building that aggregation yourself.
- No alerting or dashboarding without a separate log-aggregation stack
  layered on top — which then has to run somewhere too.

### Option 2: Cloud-specific monitoring (e.g. a managed SaaS APM)

Advantages:
- Little to no local infrastructure to run; polished dashboards and
  alerting out of the box.
- Often integrates tracing, logs, and metrics in one product.

Disadvantages:
- Directly conflicts with this project's explicit requirement to run
  completely locally with no cloud dependency — a contributor without
  an account for that specific vendor couldn't reproduce the observable
  behavior at all.
- Usually requires an agent, API key, or network egress that has no
  place in a `docker compose up --build`-only environment.
- Ties the project's observability story to a specific vendor's product
  decisions and pricing.

### Option 3: Prometheus + Grafana, self-hosted locally

Advantages:
- Runs entirely inside `docker compose`, no external account or network
  dependency — consistent with every other piece of this stack.
- Prometheus's pull-based scraping model is a natural fit for a fixed,
  known set of services (three Go services, all with static Compose
  service names).
- Grafana's file-based provisioning means both the datasource and
  dashboards can be committed to the repository and load automatically
  — no manual point-and-click setup a new contributor has to repeat.
- Widely-understood, standard tooling — not a bespoke in-house solution.

Disadvantages:
- More moving parts than "just logs": two additional containers, a
  scrape configuration file, and instrumentation code inside each
  service.
- Metrics answer "how much/how often/how long," not "why" — a spike in
  `payments_processing_failed_total` still requires logs (or a trace,
  which this project does not have) to diagnose the specific cause.

## Decision

Use Prometheus + Grafana, both running locally via `docker compose`
(`prometheus` on `:9090`, `grafana` on `:3000`) — this is already
implemented, not planned.

This is the only option consistent with the project's requirement to
run entirely locally with `docker compose up --build` and no cloud
dependency: Option 2 is disqualified by that requirement outright, and
Option 1 doesn't provide the aggregate, time-series view this
asynchronous, multi-stage system actually needs to reason about its own
health.

**Endpoints** — implemented and present on all three Go services
(`payments-service`, `payment-processor`, `analytics-service`):

- `GET /health` — liveness only; never touches a dependency (MariaDB,
  Kafka, ClickHouse), so it stays accurate even when a dependency is
  down.
- `GET /ready` — readiness; each service pings its actual dependency
  (MariaDB via `db.PingContext`, ClickHouse via a `Pinger` interface,
  Kafka via a short-lived `sarama.Client`) with a 2-second timeout,
  returning 503 when unreachable. Note: no `docker-compose.yml`
  healthcheck currently uses `/ready` (only `/health`) — see
  `docs/architecture.md §7` for why that's a deliberate choice, not an
  oversight.
- `GET /metrics` — Prometheus text-format metrics, scraped every 5
  seconds per `observability/prometheus/prometheus.yml`.

**Metrics that exist today** (the actual names in the codebase — not the
generic names a hypothetical spec might suggest):

- `http_requests_total{method,pattern,status}`,
  `http_request_duration_seconds{method,pattern}` — `payments-service`
  and `analytics-service` (labeled by route *pattern*, e.g.
  `/api/v1/transactions/{id}`, never a raw path with an ID in it, to
  keep label cardinality bounded).
- `payments_created_total`, `payments_creation_failed_total{reason}` —
  `payments-service`.
- `outbox_events_published_total`, `outbox_events_publish_errors_total`
  — `payments-service`.
- `payments_processing_total{status}`,
  `payments_processing_failed_total{retryable}`,
  `payment_processing_duration_seconds` — `payment-processor`.
- `payment_worker_active`, `payment_worker_queue_size`,
  `payment_worker_queue_capacity` — `payment-processor` (worker pool
  saturation/backpressure).
- `kafka_messages_consumed_total`,
  `clickhouse_insert_batches_total`/`clickhouse_insert_errors_total` —
  `analytics-service`.

**Explicitly not implemented today**: Kafka consumer lag is not exposed
as a metric by either consumer (`payment-processor` or
`analytics-service`) — there is no `kafka_consumer_lag` or equivalent
gauge in this codebase. A Grafana dashboard panel or ADR describing this
as available would be inaccurate; it's a natural candidate for future
work, not a current capability.

A Grafana dashboard ("Cross-Border Payments — Platform Overview") is
auto-provisioned from
`observability/grafana/provisioning/dashboards/platform-overview.json`
covering the metrics above — see `docs/architecture.md §7` for the full
panel list and the one intentionally-inexact metric mapping it documents
(Kafka processing errors → `clickhouse_insert_errors_total`, since no
generic Kafka-processing-error counter exists).

## Trade-offs

We accept increased instrumentation and infrastructure complexity —
metrics-recording code inside every HTTP handler and Kafka consumer,
two additional containers, a scrape config, and provisioning files to
maintain — in exchange for system behavior actually becoming measurable
rather than merely loggable. This is real, ongoing maintenance burden:
every new metric is a label-cardinality decision that has to be made
deliberately (this project explicitly avoids payment/event/correlation
IDs as label values, for exactly this reason), and every dashboard panel
has to be kept honest against what metrics actually exist — the moment
a panel or a document like this one claims a metric that isn't real, the
dashboard becomes actively misleading rather than merely incomplete.
