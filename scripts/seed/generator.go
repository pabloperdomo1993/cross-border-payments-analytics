// This is benchmark/seed tooling, not application code: it exists to
// produce a large, deterministic, reproducible dataset for the SQL
// performance experiments in docs/performance/. It intentionally writes
// directly to MariaDB and/or ClickHouse rather than going through Kafka
// — pushing a million individual messages through the real
// payments-service -> Kafka -> payment-processor -> analytics-service
// pipeline would test Kafka throughput, not SQL/query performance, and
// would take far longer for no benefit to what's being measured. The
// real Kafka path is unaffected and still used for actual application
// traffic.
package main

import (
	"fmt"
	"math/rand"
	"time"
)

// country -> its real-world currency, used to keep generated payments
// internally consistent (a payment sourced in Colombia is denominated
// in COP, etc).
var countryCurrency = map[string]string{
	"CO": "COP",
	"US": "USD",
	"MX": "MXN",
	"BR": "BRL",
	"AR": "ARS",
	"CL": "CLP",
}

var countries = []string{"CO", "US", "MX", "BR", "AR", "CL"}

var providers = []string{"provider_a", "provider_b", "provider_c"}

// unitsPerUSD gives each currency's approximate real-world order of
// magnitude relative to the US dollar, used only to derive plausible
// fx_rate values and amount ranges for synthetic data — this is data
// generation, not monetary arithmetic in the application domain (which
// never uses float64; see backend/*/internal/domain). The values here
// are illustrative approximations, not live market rates.
var unitsPerUSD = map[string]float64{
	"USD": 1,
	"COP": 4000,
	"MXN": 17,
	"BRL": 5,
	"ARS": 900,
	"CLP": 950,
}

// statusWeights gives each status's relative frequency in the generated
// dataset, roughly matching a healthy real-world payment system.
var statusWeights = []struct {
	status string
	weight float64
}{
	{"completed", 0.80},
	{"failed", 0.15},
	{"pending", 0.05},
}

// Record is one fully-generated synthetic payment, with every value
// already formatted the way each target database expects it.
type Record struct {
	ID                  string
	IdempotencyKey      string
	SourceCountry       string
	DestinationCountry  string
	SourceCurrency      string
	DestinationCurrency string
	SourceAmount        string // decimal string, 2 places
	DestinationAmount   string // decimal string, 2 places
	FXRate              string // decimal string, 8 places
	Provider            string
	Status              string
	CreatedAt           time.Time
}

// deterministicUUID builds a UUIDv4-shaped string from rng, so the
// entire dataset (IDs included) is reproducible from a fixed --seed.
func deterministicUUID(rng *rand.Rand) string {
	var b [16]byte
	rng.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func pickStatus(rng *rand.Rand) string {
	r := rng.Float64()
	var cumulative float64
	for _, sw := range statusWeights {
		cumulative += sw.weight
		if r < cumulative {
			return sw.status
		}
	}
	return statusWeights[len(statusWeights)-1].status
}

// pickDistinctCountries returns two different countries from the list,
// simulating source/destination.
func pickDistinctCountries(rng *rand.Rand) (string, string) {
	src := countries[rng.Intn(len(countries))]
	dst := src
	for dst == src {
		dst = countries[rng.Intn(len(countries))]
	}
	return src, dst
}

// generateRecord produces the i-th record from rng. Calling this
// repeatedly on the same freshly-seeded rng, in order, always produces
// the same sequence of records — that determinism is what makes the
// dataset reproducible across separate runs/targets.
func generateRecord(rng *rand.Rand, baseTime time.Time, spread time.Duration) Record {
	srcCountry, dstCountry := pickDistinctCountries(rng)
	srcCurrency := countryCurrency[srcCountry]
	dstCurrency := countryCurrency[dstCountry]

	rate := unitsPerUSD[dstCurrency] / unitsPerUSD[srcCurrency]
	rate *= 0.97 + rng.Float64()*0.06 // +/-3% jitter for realism

	// Amount magnitude scaled to the source currency's own unit size,
	// so e.g. COP amounts land in the millions and USD amounts in the
	// hundreds — both "realistic-looking" in their own currency.
	baseUnits := unitsPerUSD[srcCurrency] * (10 + rng.Float64()*990) // ~10-1000 USD equivalent
	sourceAmount := baseUnits
	destinationAmount := sourceAmount * rate

	createdAt := baseTime.Add(-time.Duration(rng.Int63n(int64(spread))))

	return Record{
		ID:                  deterministicUUID(rng),
		IdempotencyKey:      deterministicUUID(rng),
		SourceCountry:       srcCountry,
		DestinationCountry:  dstCountry,
		SourceCurrency:      srcCurrency,
		DestinationCurrency: dstCurrency,
		SourceAmount:        fmt.Sprintf("%.2f", sourceAmount),
		DestinationAmount:   fmt.Sprintf("%.2f", destinationAmount),
		FXRate:              fmt.Sprintf("%.8f", rate),
		Provider:            providers[rng.Intn(len(providers))],
		Status:              pickStatus(rng),
		CreatedAt:           createdAt,
	}
}
