//go:build integration

// The minimal end-to-end critical path test: a real HTTP POST against
// the running payments-service must eventually become visible through
// analytics-service's HTTP API, proving the whole chain works as a
// system — POST -> MariaDB -> outbox -> Kafka -> payment-processor ->
// Kafka -> analytics-service -> ClickHouse -> analytics API — not just
// that each hop works in isolation (the other integration tests in
// backend/*/internal/... already cover the individual hops).
//
// This test is a plain black-box HTTP client: it deliberately does not
// import any backend service's internal packages, since an end-to-end
// test should exercise the system exactly as an external caller would.
//
// Run with the full stack up:
//
//	docker compose up -d
//	go test -tags=integration ./...
package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestE2E_PaymentCreation_BecomesVisibleInAnalytics(t *testing.T) {
	paymentsURL := envOr("TEST_PAYMENTS_SERVICE_URL", "http://localhost:8080")
	analyticsURL := envOr("TEST_ANALYTICS_SERVICE_URL", "http://localhost:8082")

	client := &http.Client{Timeout: 10 * time.Second}

	if resp, err := client.Get(paymentsURL + "/health"); err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf("skipping: payments-service not reachable at %s (run `docker compose up -d` first): %v", paymentsURL, err)
	}
	if resp, err := client.Get(analyticsURL + "/health"); err != nil || resp.StatusCode != http.StatusOK {
		t.Skipf("skipping: analytics-service not reachable at %s (run `docker compose up -d` first): %v", analyticsURL, err)
	}

	// A provider name unique to this test run so the analytics query
	// below can't be satisfied by unrelated data (e.g. the repo's own
	// 1,000,000-row benchmark seed, or other test runs).
	provider := fmt.Sprintf("e2e_test_%d", time.Now().UnixNano())
	idempotencyKey := fmt.Sprintf("e2e-%d", time.Now().UnixNano())

	body, err := json.Marshal(map[string]string{
		"idempotency_key":      idempotencyKey,
		"source_country":       "CO",
		"destination_country":  "US",
		"source_currency":      "COP",
		"destination_currency": "USD",
		"source_amount":        "999.00",
		"fx_rate":              "0.00025",
		"provider":             provider,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	resp, err := client.Post(paymentsURL+"/api/v1/transactions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v1/transactions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	var created struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected a non-empty transaction id in the response")
	}

	// Poll analytics-service until the corridor reflects this payment,
	// or time out. This is the real critical path: MariaDB -> outbox
	// -> Kafka -> payment-processor -> Kafka -> analytics-service ->
	// ClickHouse -> this API.
	deadline := time.Now().Add(30 * time.Second)
	var lastBody string
	for time.Now().Before(deadline) {
		resp, err := client.Get(fmt.Sprintf("%s/api/v1/analytics/corridors?source_country=CO&destination_country=US&provider=%s", analyticsURL, provider))
		if err == nil {
			var result struct {
				Corridors []struct {
					TransactionCount int    `json:"transaction_count"`
					TotalVolume      string `json:"total_volume"`
				} `json:"corridors"`
			}
			if decodeErr := json.NewDecoder(resp.Body).Decode(&result); decodeErr == nil {
				resp.Body.Close()
				if len(result.Corridors) == 1 && result.Corridors[0].TransactionCount == 1 {
					if result.Corridors[0].TotalVolume != "999.00" {
						t.Fatalf("payment reached analytics but with unexpected volume: %+v", result.Corridors[0])
					}
					return // success: the full critical path works.
				}
				lastBody = fmt.Sprintf("%+v", result)
			} else {
				resp.Body.Close()
			}
		}
		time.Sleep(1 * time.Second)
	}

	t.Fatalf("timed out waiting for payment %s to become visible in analytics-service; last seen: %s", created.ID, lastBody)
}
