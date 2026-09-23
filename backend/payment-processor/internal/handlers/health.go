// Package handlers holds payment-processor's small HTTP surface
// (/health, /ready) as testable functions, separate from cmd/processor
// wiring so they can be exercised with httptest and a fake dependency
// instead of a real Kafka broker.
package handlers

import (
	"net/http"
	"time"

	"github.com/IBM/sarama"
)

// KafkaPinger reports whether Kafka is reachable. Satisfied in
// production by saramaPinger; tests substitute a fake.
type KafkaPinger interface {
	Ping() error
}

// saramaPinger dials a short-lived Kafka client to verify connectivity,
// mirroring the readiness check this package replaces.
type saramaPinger struct {
	brokers []string
}

// NewSaramaPinger builds the production KafkaPinger used by Ready.
func NewSaramaPinger(brokers []string) KafkaPinger {
	return saramaPinger{brokers: brokers}
}

func (p saramaPinger) Ping() error {
	cfg := sarama.NewConfig()
	cfg.Net.DialTimeout = 2 * time.Second
	client, err := sarama.NewClient(p.brokers, cfg)
	if err != nil {
		return err
	}
	return client.Close()
}

// Health reports whether the process itself is alive. It never touches
// Kafka, so it stays accurate even when the broker is unreachable.
func Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// Ready reports whether Kafka is reachable, using pinger.
func Ready(pinger KafkaPinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := pinger.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"unavailable"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}
}
