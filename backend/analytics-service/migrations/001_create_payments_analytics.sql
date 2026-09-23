-- This table is deliberately NOT a copy of MariaDB's `transactions`
-- table. MariaDB stores one normalized OLTP row per payment, optimized
-- for single-row lookups by id/idempotency_key. This table stores
-- denormalized analytical facts, optimized for scanning and
-- aggregating millions of rows across a handful of dimensions
-- (corridor, currency, provider, status, time) — a different workload
-- needs a different physical layout, not just a different SQL dialect.
--
-- Column choices:
--
-- source_country / destination_country / source_currency /
-- destination_currency / provider / status are all LowCardinality(String).
-- Every one of these has a tiny, effectively-fixed vocabulary (a
-- handful of countries/currencies/providers, three statuses).
-- LowCardinality stores a dictionary of the distinct values once per
-- part and represents each row as a small integer index into it, which
-- shrinks storage and speeds up exactly the kind of GROUP BY these
-- columns are used for (corridor/currency/provider/status breakdowns).
--
-- source_amount / destination_amount / fx_rate use ClickHouse's native
-- Decimal type (never Float64) for the same reason as MariaDB's
-- DECIMAL: monetary values must not go through binary floating point.
-- Decimal(18,2) mirrors the MariaDB column precision; Decimal(18,8)
-- mirrors fx_rate's precision.
--
-- created_at is DateTime64(3) (millisecond precision), matching
-- MariaDB's DATETIME(3), so timestamps round-trip exactly.
--
-- transaction_id is ClickHouse's native UUID type rather than a plain
-- String — the values are always UUID-shaped, so storing them as UUID
-- is both smaller and self-documenting.
--
-- corridor is a MATERIALIZED column (computed at insert time, stored,
-- queryable) rather than being recomputed via string concatenation in
-- every query — a small ClickHouse-specific convenience for the
-- corridor-volume analytic.
--
-- No idempotency_key: write-time deduplication is an OLTP concern
-- (MariaDB's job); by the time a row reaches this analytical table it
-- has already been accepted once by payments-service. No updated_at:
-- an analytical fact about a processed payment is treated as immutable
-- once ingested.
--
-- ENGINE / PARTITION BY / ORDER BY:
--
-- MergeTree is ClickHouse's standard engine for exactly this kind of
-- append-mostly analytical table.
--
-- PARTITION BY toYYYYMM(created_at): monthly partitions. This keeps the
-- partition count reasonable for a dataset spanning a few months to a
-- few years (unlike daily partitioning, which would create thousands of
-- tiny parts over a long-running system) while still letting queries
-- with a `created_at` range (e.g. `from`/`to` filters) skip whole
-- partitions outside that range.
--
-- ORDER BY (source_country, destination_country, created_at): this is
-- ClickHouse's primary key AND physical sort order, so it's chosen for
-- the query this task leads with — volume by corridor. A query filtered
-- or grouped by (source_country, destination_country) can use the
-- sparse primary index directly; created_at as the third key component
-- keeps rows for a given corridor time-ordered, which also helps
-- corridor + time-range queries and the time-series query once a
-- corridor filter is applied.
--
-- Trade-off, stated explicitly rather than hidden: queries that group
-- primarily by currency, provider, or status (not corridor) do NOT
-- benefit from this primary key and fall back to a fuller scan within
-- whichever partitions the time-range filter selects. For this
-- dataset's size that's still fast in ClickHouse (columnar, compressed,
-- LowCardinality-encoded), but it's a real, deliberate trade-off: one
-- ORDER BY key cannot be optimal for every dimension simultaneously,
-- and corridor was chosen as the priority per this task's own framing.
-- Fully qualified with the `analytics` database (set via CLICKHOUSE_DB):
-- the official image's docker-entrypoint-initdb.d scripts don't
-- reliably run with that database as the client's default context, so
-- an unqualified table name can silently land in `default` instead.
CREATE TABLE IF NOT EXISTS analytics.payments_analytics
(
    transaction_id        UUID,
    source_country        LowCardinality(String),
    destination_country   LowCardinality(String),
    source_currency       LowCardinality(String),
    destination_currency  LowCardinality(String),
    source_amount         Decimal(18, 2),
    destination_amount    Decimal(18, 2),
    fx_rate               Decimal(18, 8),
    provider              LowCardinality(String),
    status                LowCardinality(String),
    created_at            DateTime64(3),
    corridor              String MATERIALIZED concat(source_country, '-', destination_country)
)
ENGINE = MergeTree
PARTITION BY toYYYYMM(created_at)
ORDER BY (source_country, destination_country, created_at);
