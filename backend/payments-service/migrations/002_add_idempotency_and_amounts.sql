-- idempotency_key lets a client safely retry a transaction creation
-- request without producing a duplicate row: a second INSERT with the
-- same key fails the UNIQUE constraint below (surfaced by the
-- application as a 409 Conflict), rather than silently creating a
-- second, distinct transaction for what was really one retried request.
--
-- amount -> source_amount / destination_amount: the original schema
-- only stored the source-currency amount. Cross-border analytics needs
-- both sides of the conversion (how much the sender pays vs. how much
-- the recipient receives), so amount is renamed to source_amount and a
-- new destination_amount column is added alongside it — both DECIMAL,
-- matching the same precision reasoning as the original amount column
-- (see 001_create_transactions.sql).
--
-- updated_at is added for standard row-freshness tracking (e.g. status
-- transitions); it starts equal to created_at and moves forward from
-- there.
-- Split into separate statements (rather than one multi-clause ALTER)
-- so the rename is unambiguous before anything references the new
-- column name.
ALTER TABLE transactions
    CHANGE COLUMN amount source_amount DECIMAL(18,2) NOT NULL;

ALTER TABLE transactions
    ADD COLUMN idempotency_key VARCHAR(128) NOT NULL,
    ADD COLUMN destination_amount DECIMAL(18,2) NOT NULL,
    ADD COLUMN updated_at DATETIME(3) NOT NULL,
    ADD UNIQUE INDEX idx_transactions_idempotency_key (idempotency_key);
