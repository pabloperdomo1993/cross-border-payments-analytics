-- Composite index for the provider + status + created_at access
-- pattern (e.g. "recent completed transactions for provider X"),
-- measured in docs/performance/sql-optimization.md.
--
-- Column order (provider, status, created_at) follows MariaDB's
-- leftmost-prefix rule: this single index also serves queries filtering
-- on provider alone, or provider+status alone, in addition to all three
-- together — so it isn't limited to only the exact three-column query
-- it was designed around.
--
-- No separate single-column indexes on provider/status/created_at:
-- they would be redundant given this composite already covers their
-- leftmost-prefix cases, and "do not add an index for every column"
-- applies here as much as it did in 001.
ALTER TABLE transactions
    ADD INDEX idx_transactions_provider_status_created_at (provider, status, created_at);
