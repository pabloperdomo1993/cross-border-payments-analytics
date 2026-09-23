-- Amount and fx_rate are DECIMAL, never FLOAT/DOUBLE, so exact monetary
-- values survive the round trip between the Go domain layer (fixed-point
-- integers) and storage without any binary floating-point rounding.
--
-- DECIMAL(18,2) for amount: 2 fractional digits matches the minor unit
-- (cents/centavos) assumed by the Go domain's Money type; 16 integer
-- digits comfortably covers any realistic transaction amount.
--
-- DECIMAL(18,8) for fx_rate: 8 fractional digits matches the Go domain's
-- FXRate fixed-point scale (FXRateScale = 1e8), enough precision for
-- rates like 0.00026260; 10 integer digits covers rates in either
-- direction of a currency pair.
--
-- Only a PRIMARY KEY index is added. The service currently only inserts
-- transactions and looks them up by id (GET /api/v1/transactions/{id});
-- there is no list/filter endpoint yet, so indexes on created_at,
-- provider, or status would have no query to serve and are deliberately
-- deferred until such access patterns exist.
CREATE TABLE IF NOT EXISTS transactions (
    id                   CHAR(36)      NOT NULL,
    source_country       CHAR(2)       NOT NULL,
    destination_country  CHAR(2)       NOT NULL,
    source_currency      CHAR(3)       NOT NULL,
    destination_currency CHAR(3)       NOT NULL,
    amount               DECIMAL(18,2) NOT NULL,
    fx_rate              DECIMAL(18,8) NOT NULL,
    provider             VARCHAR(64)   NOT NULL,
    status               VARCHAR(16)   NOT NULL,
    created_at           DATETIME(3)   NOT NULL,
    PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
