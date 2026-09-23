// Package repository defines the persistence abstraction analytics
// queries and Kafka ingestion depend on. HTTP handlers and the Kafka
// consumer never talk to ClickHouse directly — only through this
// interface, implemented by internal/repository/clickhouse.
package repository

import (
	"context"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
)

// AnalyticsRepository serves the analytics read queries and the write
// path that ingests processed payment outcomes.
type AnalyticsRepository interface {
	Corridors(ctx context.Context, filter domain.Filter) ([]domain.CorridorVolume, error)
	Currencies(ctx context.Context, filter domain.Filter) ([]domain.CurrencyVolume, error)
	Countries(ctx context.Context, filter domain.Filter, direction domain.CountryDirection) ([]domain.CountryVolume, error)
	Providers(ctx context.Context, filter domain.Filter) ([]domain.ProviderStats, error)
	TimeSeries(ctx context.Context, filter domain.Filter, interval domain.TimeInterval) ([]domain.TimeSeriesPoint, error)

	// InsertBatch persists a batch of processed payment outcomes. Used
	// by the Kafka consumer (see internal/kafka), which accumulates
	// messages before calling this rather than inserting one row at a
	// time — batched inserts are how ClickHouse is meant to be written
	// to.
	InsertBatch(ctx context.Context, outcomes []domain.PaymentOutcome) error

	// Ping checks connectivity, used by the /ready endpoint.
	Ping(ctx context.Context) error
}
