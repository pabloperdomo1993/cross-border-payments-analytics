-- Transactional Outbox: a row here is written in the SAME database
-- transaction as the `transactions` row it describes (see
-- internal/repository/mariadb/transaction_repository.go's Create),
-- which is what makes "payment persisted" and "event queued for
-- publish" atomic — either both happen or neither does.
--
-- id: the event's own identity (not the payment's) — a payment could
-- in principle have more than one outbox event over its lifetime
-- (created, completed, failed), though today only "payment.created"
-- is produced.
--
-- aggregate_id: the transactions.id this event describes. No FOREIGN
-- KEY: the two tables are written in the same transaction by
-- application code, so referential integrity is already guaranteed at
-- write time, and a FK would only add overhead without adding safety.
--
-- payload: the exact JSON published to Kafka, serialized once at write
-- time by the application layer (see internal/application) — the
-- relay republishes these bytes verbatim rather than reconstructing
-- them from the transactions row, so what's published is guaranteed
-- to match what was intended at creation time.
--
-- status + published_at: the relay's own bookkeeping. Only two states
-- (pending/published) — there is no "failed" terminal state; a publish
-- attempt that fails just leaves the row pending for the next poll,
-- deliberately simple (see internal/outbox/relay.go's doc comment for
-- the at-least-once trade-off this implies).
CREATE TABLE IF NOT EXISTS outbox_events (
    id           CHAR(36)     NOT NULL,
    aggregate_id CHAR(36)     NOT NULL,
    event_type   VARCHAR(64)  NOT NULL,
    payload      TEXT         NOT NULL,
    status       VARCHAR(16)  NOT NULL DEFAULT 'pending',
    created_at   DATETIME(3)  NOT NULL,
    published_at DATETIME(3)  NULL,
    PRIMARY KEY (id),
    -- Serves the relay's poll query: WHERE status = 'pending' ORDER BY
    -- created_at. No index on aggregate_id: nothing queries by it yet
    -- (it exists for traceability/debugging, not lookups).
    INDEX idx_outbox_events_status_created_at (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
