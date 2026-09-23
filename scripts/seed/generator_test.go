package main

import (
	"math/rand"
	"testing"
	"time"
)

func TestGenerateRecord_Deterministic(t *testing.T) {
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spread := 90 * 24 * time.Hour

	gen := func(seed int64, n int) []Record {
		rng := rand.New(rand.NewSource(seed))
		records := make([]Record, n)
		for i := 0; i < n; i++ {
			records[i] = generateRecord(rng, baseTime, spread)
		}
		return records
	}

	a := gen(42, 100)
	b := gen(42, 100)

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("record %d differs between runs with the same seed:\n%+v\nvs\n%+v", i, a[i], b[i])
		}
	}
}

func TestGenerateRecord_DifferentSeedsDiffer(t *testing.T) {
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spread := 90 * 24 * time.Hour

	rngA := rand.New(rand.NewSource(1))
	rngB := rand.New(rand.NewSource(2))

	a := generateRecord(rngA, baseTime, spread)
	b := generateRecord(rngB, baseTime, spread)

	if a.ID == b.ID {
		t.Error("expected different seeds to produce different IDs")
	}
}

func TestGenerateRecord_ValidDimensions(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	spread := 90 * 24 * time.Hour

	for i := 0; i < 1000; i++ {
		r := generateRecord(rng, baseTime, spread)

		if r.SourceCountry == r.DestinationCountry {
			t.Fatalf("record %d: source and destination country must differ, got %s twice", i, r.SourceCountry)
		}
		if _, ok := countryCurrency[r.SourceCountry]; !ok {
			t.Fatalf("record %d: unexpected source country %s", i, r.SourceCountry)
		}
		if _, ok := countryCurrency[r.DestinationCountry]; !ok {
			t.Fatalf("record %d: unexpected destination country %s", i, r.DestinationCountry)
		}
		if countryCurrency[r.SourceCountry] != r.SourceCurrency {
			t.Fatalf("record %d: source currency %s doesn't match source country %s", i, r.SourceCurrency, r.SourceCountry)
		}
		if countryCurrency[r.DestinationCountry] != r.DestinationCurrency {
			t.Fatalf("record %d: destination currency %s doesn't match destination country %s", i, r.DestinationCurrency, r.DestinationCountry)
		}

		found := false
		for _, p := range providers {
			if p == r.Provider {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("record %d: unexpected provider %s", i, r.Provider)
		}

		switch r.Status {
		case "completed", "failed", "pending":
		default:
			t.Fatalf("record %d: unexpected status %s", i, r.Status)
		}

		if r.CreatedAt.After(baseTime) || r.CreatedAt.Before(baseTime.Add(-spread)) {
			t.Fatalf("record %d: created_at %v outside expected window", i, r.CreatedAt)
		}

		if len(r.ID) == 0 || len(r.IdempotencyKey) == 0 {
			t.Fatalf("record %d: expected non-empty id/idempotency_key", i)
		}
		if r.ID == r.IdempotencyKey {
			t.Fatalf("record %d: id and idempotency_key should not collide", i)
		}
	}
}
