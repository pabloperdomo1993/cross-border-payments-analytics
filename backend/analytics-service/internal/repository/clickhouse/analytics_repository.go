// Package clickhouse implements repository.AnalyticsRepository against
// ClickHouse using its native Go driver. All queries are parameterized
// (never string-concatenated with filter values); only static SQL
// fragments this code itself controls (column/function names) are ever
// built dynamically.
package clickhouse

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
)

// parseTime parses the RFC3339 timestamp carried on the wire (e.g. from
// payment-processor's Outcome.CreatedAt) into a time.Time suitable for
// ClickHouse's DateTime64 column.
func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// AnalyticsRepository is a ClickHouse-backed implementation of
// repository.AnalyticsRepository.
type AnalyticsRepository struct {
	conn driver.Conn
}

// NewAnalyticsRepository builds an AnalyticsRepository backed by conn.
func NewAnalyticsRepository(conn driver.Conn) *AnalyticsRepository {
	return &AnalyticsRepository{conn: conn}
}

// Ping checks connectivity to ClickHouse.
func (r *AnalyticsRepository) Ping(ctx context.Context) error {
	if err := r.conn.Ping(ctx); err != nil {
		return fmt.Errorf("%w: ping: %v", domain.ErrRepository, err)
	}
	return nil
}

// filterConditions translates filter into a slice of parameterized SQL
// conditions and their corresponding args, in matching order. Every
// condition uses a `?` placeholder — filter values are never
// interpolated into the query string.
func filterConditions(f domain.Filter) ([]string, []any) {
	var clauses []string
	var args []any

	if !f.From.IsZero() {
		clauses = append(clauses, "created_at >= ?")
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		clauses = append(clauses, "created_at < ?")
		args = append(args, f.To)
	}
	if f.SourceCountry != "" {
		clauses = append(clauses, "source_country = ?")
		args = append(args, f.SourceCountry)
	}
	if f.DestinationCountry != "" {
		clauses = append(clauses, "destination_country = ?")
		args = append(args, f.DestinationCountry)
	}
	if f.SourceCurrency != "" {
		clauses = append(clauses, "source_currency = ?")
		args = append(args, f.SourceCurrency)
	}
	if f.DestinationCurrency != "" {
		clauses = append(clauses, "destination_currency = ?")
		args = append(args, f.DestinationCurrency)
	}
	if f.Provider != "" {
		clauses = append(clauses, "provider = ?")
		args = append(args, f.Provider)
	}
	if f.Status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, f.Status)
	}
	return clauses, args
}

func whereSQL(clauses []string) string {
	if len(clauses) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(clauses, " AND ")
}

const corridorsQuery = `
SELECT source_country, destination_country, count() AS transaction_count, sum(source_amount) AS total_volume
FROM payments_analytics
%s
GROUP BY source_country, destination_country
ORDER BY total_volume DESC
`

// Corridors returns transaction volume grouped by (source_country,
// destination_country) — the query this ORDER BY key was chosen for.
func (r *AnalyticsRepository) Corridors(ctx context.Context, filter domain.Filter) ([]domain.CorridorVolume, error) {
	clauses, args := filterConditions(filter)
	query := fmt.Sprintf(corridorsQuery, whereSQL(clauses))

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: corridors query: %v", domain.ErrRepository, err)
	}
	defer rows.Close()

	results := []domain.CorridorVolume{}
	for rows.Next() {
		var (
			sourceCountry, destinationCountry string
			count                             uint64
			totalVolume                       decimal.Decimal
		)
		if err := rows.Scan(&sourceCountry, &destinationCountry, &count, &totalVolume); err != nil {
			return nil, fmt.Errorf("%w: scan corridor row: %v", domain.ErrRepository, err)
		}
		results = append(results, domain.CorridorVolume{
			SourceCountry:      sourceCountry,
			DestinationCountry: destinationCountry,
			TransactionCount:   count,
			TotalVolume:        totalVolume.StringFixed(2),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: corridors rows: %v", domain.ErrRepository, err)
	}
	return results, nil
}

const currenciesQuery = `
SELECT source_currency, count() AS transaction_count, sum(source_amount) AS total_volume
FROM payments_analytics
%s
GROUP BY source_currency
ORDER BY total_volume DESC
`

// Currencies returns transaction volume grouped by source currency —
// i.e. the currency the payment originates in.
func (r *AnalyticsRepository) Currencies(ctx context.Context, filter domain.Filter) ([]domain.CurrencyVolume, error) {
	clauses, args := filterConditions(filter)
	query := fmt.Sprintf(currenciesQuery, whereSQL(clauses))

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: currencies query: %v", domain.ErrRepository, err)
	}
	defer rows.Close()

	results := []domain.CurrencyVolume{}
	for rows.Next() {
		var (
			currency    string
			count       uint64
			totalVolume decimal.Decimal
		)
		if err := rows.Scan(&currency, &count, &totalVolume); err != nil {
			return nil, fmt.Errorf("%w: scan currency row: %v", domain.ErrRepository, err)
		}
		results = append(results, domain.CurrencyVolume{
			Currency:         currency,
			TransactionCount: count,
			TotalVolume:      totalVolume.StringFixed(2),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: currencies rows: %v", domain.ErrRepository, err)
	}
	return results, nil
}

const countriesQueryTemplate = `
SELECT %s AS country, count() AS transaction_count, sum(source_amount) AS total_volume
FROM payments_analytics
%s
GROUP BY country
ORDER BY total_volume DESC
`

// Countries returns transaction volume grouped by source or destination
// country, per direction. direction selects only the grouping COLUMN
// (a fixed, code-controlled identifier — never a filter value), so this
// stays safe from injection despite being built with fmt.Sprintf.
func (r *AnalyticsRepository) Countries(ctx context.Context, filter domain.Filter, direction domain.CountryDirection) ([]domain.CountryVolume, error) {
	column := "source_country"
	if direction == domain.CountryDirectionDestination {
		column = "destination_country"
	}

	clauses, args := filterConditions(filter)
	query := fmt.Sprintf(countriesQueryTemplate, column, whereSQL(clauses))

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: countries query: %v", domain.ErrRepository, err)
	}
	defer rows.Close()

	results := []domain.CountryVolume{}
	for rows.Next() {
		var (
			country     string
			count       uint64
			totalVolume decimal.Decimal
		)
		if err := rows.Scan(&country, &count, &totalVolume); err != nil {
			return nil, fmt.Errorf("%w: scan country row: %v", domain.ErrRepository, err)
		}
		results = append(results, domain.CountryVolume{
			Country:          country,
			TransactionCount: count,
			TotalVolume:      totalVolume.StringFixed(2),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: countries rows: %v", domain.ErrRepository, err)
	}
	return results, nil
}

const providersQuery = `
SELECT
    provider,
    count() AS total_transactions,
    countIf(status = 'completed') AS completed_transactions,
    countIf(status = 'failed') AS failed_transactions
FROM payments_analytics
%s
GROUP BY provider
ORDER BY total_transactions DESC
`

// Providers returns per-provider transaction counts and failure rate.
// failure_rate is computed in Go from the returned integer counts
// (never summed/divided in SQL as a float), avoiding any floating-point
// aggregation over the dataset itself.
func (r *AnalyticsRepository) Providers(ctx context.Context, filter domain.Filter) ([]domain.ProviderStats, error) {
	clauses, args := filterConditions(filter)
	query := fmt.Sprintf(providersQuery, whereSQL(clauses))

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: providers query: %v", domain.ErrRepository, err)
	}
	defer rows.Close()

	results := []domain.ProviderStats{}
	for rows.Next() {
		var (
			provider                 string
			total, completed, failed uint64
		)
		if err := rows.Scan(&provider, &total, &completed, &failed); err != nil {
			return nil, fmt.Errorf("%w: scan provider row: %v", domain.ErrRepository, err)
		}
		var failureRate float64
		if total > 0 {
			failureRate = float64(failed) / float64(total)
		}
		results = append(results, domain.ProviderStats{
			Provider:              provider,
			TotalTransactions:     total,
			CompletedTransactions: completed,
			FailedTransactions:    failed,
			FailureRate:           failureRate,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: providers rows: %v", domain.ErrRepository, err)
	}
	return results, nil
}

// intervalFunc maps a validated TimeInterval to the ClickHouse bucketing
// function to use. Only "day" is supported today (see
// domain.ParseTimeInterval); adding "hour"/"month" later is one more
// case here, not a rewrite.
func intervalFunc(interval domain.TimeInterval) (string, error) {
	switch interval {
	case domain.TimeIntervalDay:
		return "toStartOfDay", nil
	default:
		return "", fmt.Errorf("%w: unsupported time interval %q", domain.ErrRepository, interval)
	}
}

const timeSeriesQueryTemplate = `
SELECT %s(created_at) AS period_start, count() AS transaction_count, sum(source_amount) AS total_volume
FROM payments_analytics
%s
GROUP BY period_start
ORDER BY period_start
`

// TimeSeries returns transaction volume and count bucketed by interval.
func (r *AnalyticsRepository) TimeSeries(ctx context.Context, filter domain.Filter, interval domain.TimeInterval) ([]domain.TimeSeriesPoint, error) {
	bucketFn, err := intervalFunc(interval)
	if err != nil {
		return nil, err
	}

	clauses, args := filterConditions(filter)
	query := fmt.Sprintf(timeSeriesQueryTemplate, bucketFn, whereSQL(clauses))

	rows, err := r.conn.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: time series query: %v", domain.ErrRepository, err)
	}
	defer rows.Close()

	const outputLayout = "2006-01-02"
	results := []domain.TimeSeriesPoint{}
	for rows.Next() {
		var (
			periodStart time.Time
			count       uint64
			totalVolume decimal.Decimal
		)
		if err := rows.Scan(&periodStart, &count, &totalVolume); err != nil {
			return nil, fmt.Errorf("%w: scan time series row: %v", domain.ErrRepository, err)
		}
		results = append(results, domain.TimeSeriesPoint{
			PeriodStart:      periodStart.Format(outputLayout),
			TransactionCount: count,
			TotalVolume:      totalVolume.StringFixed(2),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: time series rows: %v", domain.ErrRepository, err)
	}
	return results, nil
}

const insertQuery = `
INSERT INTO payments_analytics (
	transaction_id, source_country, destination_country,
	source_currency, destination_currency,
	source_amount, destination_amount, fx_rate,
	provider, status, created_at
)
`

// InsertBatch writes outcomes to ClickHouse in a single batch, per
// ClickHouse's own recommended ingestion pattern (many small individual
// inserts create excessive parts and hurt performance; one batch per
// call is the idiomatic shape). Malformed individual outcomes (bad
// UUID/decimal/timestamp) are skipped with their error collected rather
// than failing the whole batch — one bad message shouldn't block every
// other message in the same batch from landing.
func (r *AnalyticsRepository) InsertBatch(ctx context.Context, outcomes []domain.PaymentOutcome) error {
	if len(outcomes) == 0 {
		return nil
	}

	batch, err := r.conn.PrepareBatch(ctx, insertQuery)
	if err != nil {
		return fmt.Errorf("%w: prepare batch: %v", domain.ErrRepository, err)
	}

	var skipped []error
	for _, o := range outcomes {
		row, err := toRow(o)
		if err != nil {
			skipped = append(skipped, err)
			continue
		}
		if err := batch.Append(
			row.transactionID, row.sourceCountry, row.destinationCountry,
			row.sourceCurrency, row.destinationCurrency,
			row.sourceAmount, row.destinationAmount, row.fxRate,
			row.provider, row.status, row.createdAt,
		); err != nil {
			skipped = append(skipped, fmt.Errorf("append row %s: %w", o.TransactionID, err))
		}
	}

	if err := batch.Send(); err != nil {
		return fmt.Errorf("%w: send batch: %v", domain.ErrRepository, err)
	}

	if len(skipped) > 0 {
		return fmt.Errorf("%w: %d of %d rows skipped: %v", domain.ErrRepository, len(skipped), len(outcomes), skipped[0])
	}
	return nil
}

type analyticsRow struct {
	transactionID                           uuid.UUID
	sourceCountry, destinationCountry       string
	sourceCurrency, destinationCurrency     string
	sourceAmount, destinationAmount, fxRate decimal.Decimal
	provider, status                        string
	createdAt                               time.Time
}

func toRow(o domain.PaymentOutcome) (analyticsRow, error) {
	id, err := uuid.Parse(o.TransactionID)
	if err != nil {
		return analyticsRow{}, fmt.Errorf("invalid transaction id %q: %w", o.TransactionID, err)
	}
	sourceAmount, err := decimal.NewFromString(o.SourceAmount)
	if err != nil {
		return analyticsRow{}, fmt.Errorf("invalid source_amount %q: %w", o.SourceAmount, err)
	}
	destinationAmount, err := decimal.NewFromString(o.DestinationAmount)
	if err != nil {
		return analyticsRow{}, fmt.Errorf("invalid destination_amount %q: %w", o.DestinationAmount, err)
	}
	fxRate, err := decimal.NewFromString(o.FXRate)
	if err != nil {
		return analyticsRow{}, fmt.Errorf("invalid fx_rate %q: %w", o.FXRate, err)
	}
	createdAt, err := parseTime(o.CreatedAt)
	if err != nil {
		return analyticsRow{}, fmt.Errorf("invalid created_at %q: %w", o.CreatedAt, err)
	}

	return analyticsRow{
		transactionID:       id,
		sourceCountry:       o.SourceCountry,
		destinationCountry:  o.DestinationCountry,
		sourceCurrency:      o.SourceCurrency,
		destinationCurrency: o.DestinationCurrency,
		sourceAmount:        sourceAmount,
		destinationAmount:   destinationAmount,
		fxRate:              fxRate,
		provider:            o.Provider,
		status:              o.Status,
		createdAt:           createdAt,
	}, nil
}
