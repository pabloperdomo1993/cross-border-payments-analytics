//go:build integration

// Integration tests against a REAL ClickHouse instance. Run with:
//
//	docker compose up -d clickhouse
//	go test -tags=integration ./internal/repository/clickhouse/...
//
// Fixture rows use a distinctive, randomly-suffixed provider name per
// test run so assertions are isolated from whatever else is in the
// table (e.g. this repo's own 1,000,000-row benchmark seed) without
// needing to truncate shared data.
package clickhouse_test

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"testing"
	"time"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/domain"
	chrepo "github.com/pabloperdomo1993/cross-border-payments-analytics/backend/analytics-service/internal/repository/clickhouse"
)

func testConn(t *testing.T) driver.Conn {
	t.Helper()

	addr := envOr("TEST_CLICKHOUSE_ADDR", "localhost:9000")
	database := envOr("TEST_CLICKHOUSE_DATABASE", "analytics")
	user := envOr("TEST_CLICKHOUSE_USER", "default")
	pass := envOr("TEST_CLICKHOUSE_PASSWORD", "analytics")

	conn, err := clickhousedriver.Open(&clickhousedriver.Options{
		Addr: []string{addr},
		Auth: clickhousedriver.Auth{Database: database, Username: user, Password: pass},
	})
	if err != nil {
		t.Fatalf("open clickhouse: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Ping(ctx); err != nil {
		t.Skipf("skipping: cannot reach test ClickHouse at %s (run `docker compose up -d clickhouse` first): %v", addr, err)
	}
	return conn
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("generate random suffix: %v", err)
	}
	return fmt.Sprintf("%x", b)
}

type fixtureRow struct {
	sourceCountry, destinationCountry   string
	sourceCurrency, destinationCurrency string
	amount                              string
	status                              string
}

// seedFixtures inserts rows tagged with a unique provider name (so
// assertions can filter to exactly this test's data) and returns that
// provider name.
func seedFixtures(t *testing.T, conn driver.Conn, rows []fixtureRow) string {
	t.Helper()
	provider := "integration_test_" + randomSuffix(t)
	ctx := context.Background()
	createdAt := time.Now().UTC()

	batch, err := conn.PrepareBatch(ctx, `
		INSERT INTO payments_analytics (
			transaction_id, source_country, destination_country,
			source_currency, destination_currency,
			source_amount, destination_amount, fx_rate,
			provider, status, created_at
		)
	`)
	if err != nil {
		t.Fatalf("prepare batch: %v", err)
	}

	for _, r := range rows {
		amount, err := decimal.NewFromString(r.amount)
		if err != nil {
			t.Fatalf("parse fixture amount: %v", err)
		}
		if err := batch.Append(
			uuid.New(), r.sourceCountry, r.destinationCountry,
			r.sourceCurrency, r.destinationCurrency,
			amount, amount, decimal.NewFromFloat(1.0),
			provider, r.status, createdAt,
		); err != nil {
			t.Fatalf("append fixture row: %v", err)
		}
	}

	if err := batch.Send(); err != nil {
		t.Fatalf("send fixture batch: %v", err)
	}

	return provider
}

func TestIntegration_Corridors_AggregatesByCorridor(t *testing.T) {
	conn := testConn(t)
	repo := chrepo.NewAnalyticsRepository(conn)

	// CO -> US : 100, CO -> US : 200, MX -> US : 500
	provider := seedFixtures(t, conn, []fixtureRow{
		{"CO", "US", "COP", "USD", "100.00", domain.StatusCompleted},
		{"CO", "US", "COP", "USD", "200.00", domain.StatusCompleted},
		{"MX", "US", "MXN", "USD", "500.00", domain.StatusCompleted},
	})

	results, err := repo.Corridors(context.Background(), domain.Filter{Provider: provider})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var coUS, mxUS *domain.CorridorVolume
	for i := range results {
		switch {
		case results[i].SourceCountry == "CO" && results[i].DestinationCountry == "US":
			coUS = &results[i]
		case results[i].SourceCountry == "MX" && results[i].DestinationCountry == "US":
			mxUS = &results[i]
		}
	}

	if coUS == nil {
		t.Fatal("expected a CO -> US corridor in the results")
	}
	if coUS.TransactionCount != 2 {
		t.Errorf("expected CO -> US transaction_count = 2, got %d", coUS.TransactionCount)
	}
	if coUS.TotalVolume != "300.00" {
		t.Errorf("expected CO -> US volume = 300.00, got %s", coUS.TotalVolume)
	}

	if mxUS == nil {
		t.Fatal("expected an MX -> US corridor in the results")
	}
	if mxUS.TransactionCount != 1 {
		t.Errorf("expected MX -> US transaction_count = 1, got %d", mxUS.TransactionCount)
	}
	if mxUS.TotalVolume != "500.00" {
		t.Errorf("expected MX -> US volume = 500.00, got %s", mxUS.TotalVolume)
	}
}

func TestIntegration_Currencies_AggregatesBySourceCurrency(t *testing.T) {
	conn := testConn(t)
	repo := chrepo.NewAnalyticsRepository(conn)

	provider := seedFixtures(t, conn, []fixtureRow{
		{"CO", "US", "COP", "USD", "100.00", domain.StatusCompleted},
		{"CO", "BR", "COP", "BRL", "50.00", domain.StatusCompleted},
		{"MX", "US", "MXN", "USD", "500.00", domain.StatusCompleted},
	})

	results, err := repo.Currencies(context.Background(), domain.Filter{Provider: provider})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byCurrency := map[string]domain.CurrencyVolume{}
	for _, r := range results {
		byCurrency[r.Currency] = r
	}

	if got := byCurrency["COP"]; got.TransactionCount != 2 || got.TotalVolume != "150.00" {
		t.Errorf("expected COP: count=2 volume=150.00, got count=%d volume=%s", got.TransactionCount, got.TotalVolume)
	}
	if got := byCurrency["MXN"]; got.TransactionCount != 1 || got.TotalVolume != "500.00" {
		t.Errorf("expected MXN: count=1 volume=500.00, got count=%d volume=%s", got.TransactionCount, got.TotalVolume)
	}
}

func TestIntegration_Countries_BySourceAndDestination(t *testing.T) {
	conn := testConn(t)
	repo := chrepo.NewAnalyticsRepository(conn)

	provider := seedFixtures(t, conn, []fixtureRow{
		{"CO", "US", "COP", "USD", "100.00", domain.StatusCompleted},
		{"CO", "BR", "COP", "BRL", "50.00", domain.StatusCompleted},
		{"MX", "US", "MXN", "USD", "500.00", domain.StatusCompleted},
	})

	ctx := context.Background()

	bySource, err := repo.Countries(ctx, domain.Filter{Provider: provider}, domain.CountryDirectionSource)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sourceByCountry := map[string]domain.CountryVolume{}
	for _, r := range bySource {
		sourceByCountry[r.Country] = r
	}
	if got := sourceByCountry["CO"]; got.TransactionCount != 2 || got.TotalVolume != "150.00" {
		t.Errorf("expected source CO: count=2 volume=150.00, got count=%d volume=%s", got.TransactionCount, got.TotalVolume)
	}

	byDestination, err := repo.Countries(ctx, domain.Filter{Provider: provider}, domain.CountryDirectionDestination)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	destByCountry := map[string]domain.CountryVolume{}
	for _, r := range byDestination {
		destByCountry[r.Country] = r
	}
	if got := destByCountry["US"]; got.TransactionCount != 2 || got.TotalVolume != "600.00" {
		t.Errorf("expected destination US: count=2 volume=600.00, got count=%d volume=%s", got.TransactionCount, got.TotalVolume)
	}
}

func TestIntegration_Providers_CountsAndFailureRate(t *testing.T) {
	conn := testConn(t)
	repo := chrepo.NewAnalyticsRepository(conn)

	provider := seedFixtures(t, conn, []fixtureRow{
		{"CO", "US", "COP", "USD", "100.00", domain.StatusCompleted},
		{"CO", "US", "COP", "USD", "200.00", domain.StatusCompleted},
		{"CO", "US", "COP", "USD", "300.00", domain.StatusFailed},
		{"CO", "US", "COP", "USD", "400.00", domain.StatusFailed},
	})

	results, err := repo.Providers(context.Background(), domain.Filter{Provider: provider})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 provider row for this isolated fixture, got %d", len(results))
	}

	got := results[0]
	if got.TotalTransactions != 4 {
		t.Errorf("expected total_transactions = 4, got %d", got.TotalTransactions)
	}
	if got.CompletedTransactions != 2 {
		t.Errorf("expected completed_transactions = 2, got %d", got.CompletedTransactions)
	}
	if got.FailedTransactions != 2 {
		t.Errorf("expected failed_transactions = 2, got %d", got.FailedTransactions)
	}
	if got.FailureRate != 0.5 {
		t.Errorf("expected failure_rate = 0.5, got %v", got.FailureRate)
	}
}

func TestIntegration_TimeSeries_AggregatesByDay(t *testing.T) {
	conn := testConn(t)
	repo := chrepo.NewAnalyticsRepository(conn)

	provider := seedFixtures(t, conn, []fixtureRow{
		{"CO", "US", "COP", "USD", "100.00", domain.StatusCompleted},
		{"CO", "US", "COP", "USD", "200.00", domain.StatusCompleted},
	})

	results, err := repo.TimeSeries(context.Background(), domain.Filter{Provider: provider}, domain.TimeIntervalDay)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 day bucket for same-day fixtures, got %d: %+v", len(results), results)
	}
	if results[0].TransactionCount != 2 {
		t.Errorf("expected transaction_count = 2, got %d", results[0].TransactionCount)
	}
	if results[0].TotalVolume != "300.00" {
		t.Errorf("expected total_volume = 300.00, got %s", results[0].TotalVolume)
	}
}

func TestIntegration_Corridors_EmptyDataset(t *testing.T) {
	conn := testConn(t)
	repo := chrepo.NewAnalyticsRepository(conn)

	// A provider name that (by construction) matches no rows.
	results, err := repo.Corridors(context.Background(), domain.Filter{Provider: "no_such_provider_" + randomSuffix(t)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results for a provider with no data, got %d rows", len(results))
	}
}

func TestIntegration_Ping(t *testing.T) {
	conn := testConn(t)
	repo := chrepo.NewAnalyticsRepository(conn)

	if err := repo.Ping(context.Background()); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
