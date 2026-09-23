// Package metrics defines every Prometheus collector payments-service
// exposes, in one place, so nothing is registered twice.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTPRequestsTotal and HTTPRequestDuration are labeled by the route
// PATTERN (e.g. "/api/v1/transactions/{id}"), never the raw request
// path — using the raw path would make transaction IDs a label value,
// an unbounded-cardinality metric label.
var HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "http_requests_total",
	Help: "Total number of HTTP requests, by method, route pattern, and status.",
}, []string{"method", "pattern", "status"})

var HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "HTTP request duration in seconds, by method and route pattern.",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "pattern"})

// PaymentsCreatedTotal counts successful payment creations.
var PaymentsCreatedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "payments_created_total",
	Help: "Total number of payments successfully created.",
})

// PaymentsCreationFailedTotal counts failed payment-creation attempts,
// by a low-cardinality failure reason (never the raw error message).
var PaymentsCreationFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "payments_creation_failed_total",
	Help: "Total number of payment creation attempts that failed, by reason.",
}, []string{"reason"})

// OutboxEventsPublishedTotal and OutboxEventsPublishErrorsTotal track
// the relay's own throughput/health.
var OutboxEventsPublishedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "outbox_events_published_total",
	Help: "Total number of outbox events successfully published to Kafka.",
})

var OutboxEventsPublishErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "outbox_events_publish_errors_total",
	Help: "Total number of outbox event publish attempts that failed.",
})
