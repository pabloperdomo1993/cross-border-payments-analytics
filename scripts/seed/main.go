// Command seed generates a large, deterministic synthetic payments
// dataset and loads it directly into MariaDB and/or ClickHouse, in
// batches, without ever holding the full dataset in memory. See the
// package comment in generator.go for why this bypasses the normal
// application/Kafka path.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	clickhousedriver "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	var (
		target          = flag.String("target", "both", "where to load data: mariadb, clickhouse, or both")
		count           = flag.Int("count", 1_000_000, "number of payment records to generate")
		seed            = flag.Int64("seed", 42, "PRNG seed; the same seed always produces the same dataset")
		days            = flag.Int("days", 90, "spread created_at over this many trailing days")
		mariadbBatch    = flag.Int("mariadb-batch-size", 1000, "rows per MariaDB INSERT statement")
		clickhouseBatch = flag.Int("clickhouse-batch-size", 20000, "rows per ClickHouse batch insert")

		mariadbDSN = flag.String("mariadb-dsn", envOr("SEED_MARIADB_DSN", "payments:payments@tcp(localhost:3306)/payments?parseTime=true"), "MariaDB DSN")
		chAddr     = flag.String("clickhouse-addr", envOr("SEED_CLICKHOUSE_ADDR", "localhost:9000"), "ClickHouse native address")
		chDatabase = flag.String("clickhouse-database", envOr("SEED_CLICKHOUSE_DATABASE", "analytics"), "ClickHouse database")
		chUser     = flag.String("clickhouse-user", envOr("SEED_CLICKHOUSE_USER", "default"), "ClickHouse user")
		chPassword = flag.String("clickhouse-password", envOr("SEED_CLICKHOUSE_PASSWORD", "analytics"), "ClickHouse password")
	)
	flag.Parse()

	if *target != "mariadb" && *target != "clickhouse" && *target != "both" {
		log.Fatalf("invalid -target %q: must be mariadb, clickhouse, or both", *target)
	}

	baseTime := time.Now().UTC()
	spread := time.Duration(*days) * 24 * time.Hour

	if *target == "mariadb" || *target == "both" {
		start := time.Now()
		if err := seedMariaDB(*mariadbDSN, *count, *seed, *mariadbBatch, baseTime, spread); err != nil {
			log.Fatalf("seed mariadb: %v", err)
		}
		log.Printf("mariadb: inserted %d rows in %s", *count, time.Since(start))
	}

	if *target == "clickhouse" || *target == "both" {
		start := time.Now()
		if err := seedClickHouse(*chAddr, *chDatabase, *chUser, *chPassword, *count, *seed, *clickhouseBatch, baseTime, spread); err != nil {
			log.Fatalf("seed clickhouse: %v", err)
		}
		log.Printf("clickhouse: inserted %d rows in %s", *count, time.Since(start))
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

const insertTransactionsSQL = `
INSERT INTO transactions (
	id, idempotency_key, source_country, destination_country,
	source_currency, destination_currency,
	source_amount, destination_amount, fx_rate, provider, status,
	created_at, updated_at
) VALUES `

func seedMariaDB(dsn string, count int, seed int64, batchSize int, baseTime time.Time, spread time.Duration) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	ctx := context.Background()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	rng := rand.New(rand.NewSource(seed))

	const columnsPerRow = 13
	rowPlaceholder := "(?,?,?,?,?,?,?,?,?,?,?,?,?)"

	inserted := 0
	for inserted < count {
		n := batchSize
		if remaining := count - inserted; remaining < n {
			n = remaining
		}

		args := make([]any, 0, n*columnsPerRow)
		query := insertTransactionsSQL
		for i := 0; i < n; i++ {
			if i > 0 {
				query += ","
			}
			query += rowPlaceholder

			r := generateRecord(rng, baseTime, spread)
			args = append(args,
				r.ID, r.IdempotencyKey, r.SourceCountry, r.DestinationCountry,
				r.SourceCurrency, r.DestinationCurrency,
				r.SourceAmount, r.DestinationAmount, r.FXRate, r.Provider, r.Status,
				r.CreatedAt, r.CreatedAt,
			)
		}

		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("insert batch at offset %d: %w", inserted, err)
		}

		inserted += n
		if inserted%100000 == 0 || inserted == count {
			log.Printf("mariadb: %d/%d rows inserted", inserted, count)
		}
	}

	return nil
}

func seedClickHouse(addr, database, user, password string, count int, seed int64, batchSize int, baseTime time.Time, spread time.Duration) error {
	conn, err := clickhousedriver.Open(&clickhousedriver.Options{
		Addr: []string{addr},
		Auth: clickhousedriver.Auth{
			Database: database,
			Username: user,
			Password: password,
		},
	})
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer conn.Close()

	ctx := context.Background()
	if err := conn.Ping(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	rng := rand.New(rand.NewSource(seed))

	inserted := 0
	for inserted < count {
		n := batchSize
		if remaining := count - inserted; remaining < n {
			n = remaining
		}

		if err := insertClickHouseBatch(ctx, conn, rng, n, baseTime, spread); err != nil {
			return fmt.Errorf("insert batch at offset %d: %w", inserted, err)
		}

		inserted += n
		if inserted%100000 == 0 || inserted == count {
			log.Printf("clickhouse: %d/%d rows inserted", inserted, count)
		}
	}

	return nil
}

func insertClickHouseBatch(ctx context.Context, conn driver.Conn, rng *rand.Rand, n int, baseTime time.Time, spread time.Duration) error {
	batch, err := conn.PrepareBatch(ctx, `
		INSERT INTO payments_analytics (
			transaction_id, source_country, destination_country,
			source_currency, destination_currency,
			source_amount, destination_amount, fx_rate,
			provider, status, created_at
		)
	`)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for i := 0; i < n; i++ {
		r := generateRecord(rng, baseTime, spread)
		id, err := parseUUID(r.ID)
		if err != nil {
			return fmt.Errorf("parse uuid: %w", err)
		}
		sourceAmount, err := parseDecimal(r.SourceAmount)
		if err != nil {
			return fmt.Errorf("parse source_amount: %w", err)
		}
		destinationAmount, err := parseDecimal(r.DestinationAmount)
		if err != nil {
			return fmt.Errorf("parse destination_amount: %w", err)
		}
		fxRate, err := parseDecimal(r.FXRate)
		if err != nil {
			return fmt.Errorf("parse fx_rate: %w", err)
		}

		if err := batch.Append(
			id, r.SourceCountry, r.DestinationCountry,
			r.SourceCurrency, r.DestinationCurrency,
			sourceAmount, destinationAmount, fxRate,
			r.Provider, r.Status, r.CreatedAt,
		); err != nil {
			return fmt.Errorf("append row: %w", err)
		}
	}

	return batch.Send()
}
