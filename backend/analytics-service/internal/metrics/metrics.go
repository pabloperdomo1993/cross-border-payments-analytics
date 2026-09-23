// Package metrics defines every Prometheus collector analytics-service
// exposes, in one place, so nothing is registered twice.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// HTTPRequestsTotal and HTTPRequestDuration are labeled by route
// PATTERN, not the raw path — none of this service's routes have path
// parameters today, but this keeps the convention consistent with
// payments-service and safe if one is added later.
var HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "http_requests_total",
	Help: "Total number of HTTP requests, by method, route pattern, and status.",
}, []string{"method", "pattern", "status"})

var HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_request_duration_seconds",
	Help:    "HTTP request duration in seconds, by method and route pattern.",
	Buckets: prometheus.DefBuckets,
}, []string{"method", "pattern"})

// KafkaMessagesConsumedTotal counts individual payments.processed /
// payments.dlq messages consumed (before batching), regardless of
// whether their batch's insert ultimately succeeds.
var KafkaMessagesConsumedTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "kafka_messages_consumed_total",
	Help: "Total number of Kafka messages consumed from payments.processed/payments.dlq.",
})

// ClickHouseInsertBatchesTotal and ClickHouseInsertErrorsTotal track the
// batch-insert ingestion path's health.
var ClickHouseInsertBatchesTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "clickhouse_insert_batches_total",
	Help: "Total number of batch inserts attempted against ClickHouse.",
})

var ClickHouseInsertErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
	Name: "clickhouse_insert_errors_total",
	Help: "Total number of batch inserts against ClickHouse that failed.",
})
