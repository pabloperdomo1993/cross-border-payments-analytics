# MariaDB vs. ClickHouse: an architectural comparison, not a contest

**All numbers on this page were measured live** against real MariaDB 11
and ClickHouse 24.8 containers in this repo's Docker Compose
environment. They are not estimates.

**The point of this document is not "ClickHouse wins."** It's to show
*why* this system uses both engines for different jobs, with real
numbers backing up the architectural reasoning in `docs/architecture.md`
— not just asserting it.

- **MariaDB** (`payments-service`): OLTP / operational state. Its job is
  safe, correct, single-row and small-range operations — create a
  payment, look one up by id, enforce a unique `idempotency_key` — under
  concurrent writes, with real transactional guarantees. It is
  deliberately **not** tuned or expected to be fast at scanning and
  aggregating the entire table.
- **ClickHouse** (`analytics-service`): OLAP / analytical workloads. Its
  job is exactly the opposite: scan and aggregate millions of rows
  across a handful of dimensions (corridor, currency, provider, time) as
  fast as possible. It has no concept of a unique-constraint-enforced
  primary key the way MariaDB does, and isn't used for point lookups by
  id in this system.

Below is one query that exercises exactly the workload each engine is
*not* specialized for versus the one it is — run against equivalent
datasets on both.

## Setup

- **Dataset**: the same deterministic seed (`seed=42`) used for
  `docs/performance/sql-optimization.md`, loaded into both engines via
  `scripts/seed` — `-target=mariadb` and `-target=clickhouse` — each
  producing 1,000,000 rows from an identical PRNG sequence.
- **Row counts**: MariaDB 1,000,001; ClickHouse 1,000,001 (each has one
  extra manually-created row from earlier end-to-end verification, in
  different corridors — see below).
- **Query** (logically identical on both engines): volume and
  transaction count grouped by corridor.

  ```sql
  -- MariaDB
  SELECT source_country, destination_country,
         COUNT(*) AS transaction_count, SUM(source_amount) AS total_volume
  FROM transactions
  GROUP BY source_country, destination_country
  ORDER BY total_volume DESC;

  -- ClickHouse
  SELECT source_country, destination_country,
         count() AS transaction_count, sum(source_amount) AS total_volume
  FROM analytics.payments_analytics
  GROUP BY source_country, destination_country
  ORDER BY total_volume DESC;
  ```

  Neither table has an index/sort key on both `source_country` and
  `destination_country` in a way that avoids scanning the bulk of the
  table for this query — MariaDB has no index covering this GROUP BY at
  all (a genuine full table scan); ClickHouse's `ORDER BY
  (source_country, destination_country, created_at)` primary key is
  exactly aligned with this query's GROUP BY, which is the intended
  effect of that schema choice (see the migration's comments).

- **Method**: MariaDB via `SET profiling=1; ... SHOW PROFILES;`
  (3 runs); ClickHouse via `clickhouse-client --time ... FORMAT Null`
  (suppresses result printing so only server-side execution time is
  measured, 3 runs).

## Results

**MariaDB** (3 runs): 5.769s, 5.751s, 5.660s.

**ClickHouse** (3 runs): 0.032s, 0.032s, 0.050s.

| Engine | Avg. duration | Rows scanned |
|---|---|---|
| MariaDB 11 | ~5.73s | 1,000,001 (full table scan, row-by-row) |
| ClickHouse 24.8 | ~0.038s | 1,000,001 (columnar scan, only `source_country`/`destination_country`/`source_amount` columns read) |

**~150x faster on ClickHouse for this query**, on equivalent data.

## Correctness check

The two result sets agree almost exactly — the small, fully-explained
difference confirms the datasets really are equivalent, not just
similarly sized:

| Corridor | MariaDB count | ClickHouse count | MariaDB volume | ClickHouse volume |
|---|---|---|---|---|
| CO→US | 33,627 | 33,626 | 68,130,414,459.77 | 68,126,414,459.77 |
| CO→BR | 33,537 | 33,537 | 67,936,987,323.98 | 67,936,987,323.98 |
| CO→AR | 33,457 | 33,457 | 67,569,143,564.11 | 67,569,143,564.11 |
| CO→CL | 33,180 | 33,180 | 66,846,185,237.54 | 66,846,185,237.54 |
| CO→MX | 33,069 | 33,069 | 66,657,837,212.46 | 66,657,837,212.46 |

Every corridor matches exactly except CO→US, where MariaDB has exactly
one more transaction and exactly 4,000,000.00 more volume — precisely
the one payment created manually through `payments-service`'s HTTP API
during earlier verification (a CO→US, 4,000,000.00 COP transaction),
which only exists in MariaDB. ClickHouse's dataset instead has a
different single manual test row (MX→US, from the Kafka end-to-end
verification), which is why its total row count also matches MariaDB's
(1,000,001) despite this per-corridor difference. Both are explained,
accounted-for artifacts of the manual verification steps, not
data-loading bugs.

## Why the gap is this large

This isn't a tuning failure on MariaDB's side — it's the two engines
doing exactly what they're built for:

- MariaDB (InnoDB) stores rows together on disk/in the buffer pool. To
  compute `SUM(source_amount)` grouped by two columns, it has to read
  every row in full — every column, including ones this query doesn't
  need — because that's how row-oriented storage works.
- ClickHouse stores each column separately and contiguously. This query
  only touches three columns (`source_country`, `destination_country`,
  `source_amount`), so ClickHouse reads only those, in compressed,
  contiguous blocks. `source_country`/`destination_country` are also
  `LowCardinality(String)` (a handful of distinct values, dictionary
  -encoded), making both the scan and the GROUP BY itself cheap. On top
  of that, the table's `ORDER BY (source_country, destination_country,
  created_at)` primary key means rows are already physically grouped by
  corridor, which is exactly what this query groups by.

## What this does *not* show

This is one aggregation query on one dataset size. It does not show:
- MariaDB is "bad" — for `payments-service`'s actual job (create one
  payment, look one up by id/idempotency_key), MariaDB's numbers in
  `sql-optimization.md` are excellent (single-digit milliseconds with
  the right index).
- ClickHouse is a general-purpose replacement for MariaDB — it has no
  role in this system for the transactional, single-row,
  uniqueness-enforced writes `payments-service` needs.
- How either engine behaves at 10x or 100x this row count, with
  concurrent load, or with a query shape that doesn't align with
  ClickHouse's `ORDER BY` key (see `sql-optimization.md`'s own honest
  result for a case where an index didn't help as expected).

## Reproducing this

```bash
docker compose up -d mariadb clickhouse
cd scripts/seed && go build -o seed .
./seed -target=both -count=1000000 -seed=42
```
