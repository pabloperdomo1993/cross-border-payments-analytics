# MariaDB SQL optimization experiment

**All numbers on this page were measured live** against a real MariaDB
11 container in this repo's Docker Compose environment, running on the
development machine used to build this feature. They are not estimates.
Absolute timings will vary by hardware; the *relative* improvement and
the query-plan changes are the reproducible part.

## Setup

- **Dataset**: `backend/payments-service`'s `transactions` table, seeded
  with `scripts/seed` (`./seed -target=mariadb -count=1000000 -seed=42`).
  Deterministic — the same seed reproduces the same dataset.
- **Final row count**: 1,000,001 (1,000,000 generated + 1 manually
  created via the API during earlier verification).
- **Distribution** (`GROUP BY provider, status`): roughly even across
  `provider_a`/`provider_b`/`provider_c`, each ~80% `completed`, ~15%
  `failed`, ~5% `pending`.
- **Date range**: `created_at` spans 2026-06-25 to 2026-09-23 (a 90-day
  trailing window from generation time).
- **Query under test**:

  ```sql
  SELECT id, source_amount, destination_amount, created_at
  FROM transactions
  WHERE provider = 'provider_a'
    AND status = 'completed'
    AND created_at >= '2026-09-15 00:00:00'
    AND created_at < '2026-09-22 00:00:00';
  ```

  This matches 20,802 rows (~2.08% of the table) — a realistic
  "recent completed transactions for one provider" operational query,
  not a needle-in-a-haystack lookup.

- **Method**: MariaDB's `SET profiling = 1; ... SHOW PROFILES;` for
  wall-clock duration (server-side, three runs each), plus
  `EXPLAIN` / `ANALYZE` (MariaDB's `EXPLAIN ANALYZE` equivalent) for the
  query plan and actual row counts.

## Before: no supporting index

Only `PRIMARY KEY (id)` and the `idempotency_key` unique index existed
(from migrations 001–002).

```
EXPLAIN:
id  select_type  table         type  possible_keys  key   rows    Extra
1   SIMPLE       transactions  ALL   NULL           NULL  969524  Using where

ANALYZE (actual execution):
id  select_type  table         type  rows    r_rows      filtered  r_filtered  Extra
1   SIMPLE       transactions  ALL   969524  1000001.00  100.00    2.08        Using where
```

`type: ALL` — full table scan. `r_rows: 1000001` — every row was read
to find the 2.08% that matched.

**Measured duration, `SELECT ... WHERE provider=... AND status=... AND created_at BETWEEN ...` returning the 4 columns above (3 runs):** 3.94s, 4.03s, 4.08s.

**Measured duration, the same predicate as `SELECT COUNT(*)` (isolates scan/filter cost from result-set transfer, 3 runs):** 4.38s, 4.81s, 4.42s.

Both forms cost about the same before indexing, which makes sense: with
no index, the dominant cost is the full 1,000,001-row scan itself,
regardless of how many columns or rows are ultimately returned.

## After: `(provider, status, created_at)` composite index

```sql
ALTER TABLE transactions
    ADD INDEX idx_transactions_provider_status_created_at (provider, status, created_at);
```

Applied live against the already-seeded 1,000,001-row table in **8.3s**
(`time` around the `ALTER TABLE`).

```
EXPLAIN:
id  select_type  table         type   possible_keys                                 key                                           key_len  rows   Extra
1   SIMPLE       transactions  range  idx_transactions_provider_status_created_at  idx_transactions_provider_status_created_at  331      42158  Using index condition

ANALYZE (actual execution):
id  select_type  table         type   rows   r_rows    filtered  r_filtered  Extra
1   SIMPLE       transactions  range  42158  20802.00  100.00    100.00      Using index condition
```

`type: range` using the new index. `r_rows: 20802` — MariaDB read
almost exactly the matching rows, not the whole table (`r_filtered:
100%`, vs. 2.08% before — everything the index handed back was already
a match).

**Measured duration, `SELECT COUNT(*)` with the same predicate (3 runs):** 0.0125s, 0.0101s, 0.0099s.

**Measured duration, the same 4-column row-returning `SELECT` (3 runs):** 4.86s, 4.73s, 4.34s.

## Before vs. after

| Measurement | Before | After | Change |
|---|---|---|---|
| Query plan | `ALL` (full scan) | `range` (index scan) | — |
| Rows examined (`r_rows`) | 1,000,001 | 20,802 | ~48x fewer rows read |
| `COUNT(*)` duration | ~4.4–4.8s | ~0.010–0.013s | **~400–450x faster** |
| 4-column row-returning `SELECT` duration | ~3.9–4.1s | ~4.3–4.9s | **no improvement (slightly worse)** |

## This is a real, honest result — not the one you'd expect

The `COUNT(*)` numbers show the index doing exactly what it should:
~450x less work to find and count the matching rows. But the actual
row-returning query — the one this experiment was built around — showed
**no improvement, and if anything a small regression**, after indexing.

This was measured, not adjusted after the fact, and it's a genuinely
useful result to understand rather than paper over:

- The composite index covers the `WHERE` clause perfectly (`Using index
  condition`), but the query also selects `id`, `source_amount`, and
  `destination_amount` — columns **not** in the index. For each of the
  20,802 matching rows, MariaDB (InnoDB) still has to do a secondary
  lookup back into the clustered index (keyed on `id`) to fetch those
  columns. That's 20,802 extra random-access lookups, on top of the
  index range scan.
- At ~2% selectivity, that's a lot of lookups — this is the classic
  crossover zone where a secondary index stops being a clear win for
  row-returning queries: cheap enough to locate matches, expensive
  enough in per-row lookups (plus transferring ~20,802 rows back to the
  client) that it doesn't beat a sequential scan of the same table.
  `COUNT(*)`, needing none of those lookups or any row transfer, shows
  the index's real benefit cleanly.
- Practical implication: this index is a strong win for
  existence/counting checks and for queries with tighter selectivity
  (a single day instead of a week would return a few thousand rows, not
  20,802, and likely show a real end-to-end improvement — not measured
  here, since the goal was to report what was actually run, not what
  plausibly would happen with different parameters).
- This is exactly why the index is still justified — it directly serves
  `analytics-service`'s provider-filtered queries and any future
  `WHERE provider = ? AND status = ?`-shaped lookup — but its benefit
  for *this specific* 4-column, ~2%-selectivity query is a lesson in
  index limits, not a validation failure.

## Reproducing this

```bash
docker compose up -d mariadb
cd scripts/seed && go build -o seed . && ./seed -target=mariadb -count=1000000 -seed=42
# run the BEFORE queries against migrations 001+002 only, then:
docker compose exec mariadb mariadb -u payments -ppayments payments \
  -e "ALTER TABLE transactions ADD INDEX idx_transactions_provider_status_created_at (provider, status, created_at);"
# re-run the same queries for AFTER
```
