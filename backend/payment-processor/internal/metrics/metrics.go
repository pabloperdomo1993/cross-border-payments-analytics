// Package metrics defines every Prometheus collector payment-processor
// exposes, in one place, so nothing is registered twice and every
// package that needs to record a metric imports this one rather than
// declaring its own collectors.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// gaugeSource is satisfied by *workerpool.Pool without this package
// importing workerpool, avoiding a dependency cycle (workerpool would
// otherwise need to import metrics to register these gauges itself).
type gaugeSource interface {
	ActiveWorkers() int64
	QueueLen() int
	Capacity() int
}

// RegisterWorkerPoolGauges wires payment_worker_active,
// payment_worker_queue_size, and payment_worker_queue_capacity to pool.
// They're implemented as GaugeFuncs (read on every scrape) rather than
// gauges updated by a background goroutine, so there's no extra
// goroutine or mutable global state to manage.
func RegisterWorkerPoolGauges(pool gaugeSource) {
	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "payment_worker_active",
		Help: "Number of worker goroutines currently executing a payment job.",
	}, func() float64 { return float64(pool.ActiveWorkers()) })

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "payment_worker_queue_size",
		Help: "Number of payment jobs currently buffered in the worker pool's queue.",
	}, func() float64 { return float64(pool.QueueLen()) })

	promauto.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "payment_worker_queue_capacity",
		Help: "Configured capacity of the worker pool's job queue.",
	}, func() float64 { return float64(pool.Capacity()) })
}

// ProcessingTotal counts every processing attempt outcome. The "status"
// label is low-cardinality (completed|failed) by design — no
// transaction/event/correlation IDs are ever used as label values.
var ProcessingTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "payments_processing_total",
	Help: "Total number of payment processing attempts, by outcome status.",
}, []string{"status"})

// ProcessingFailedTotal counts failures, split by whether the failure
// was retryable — also low-cardinality.
var ProcessingFailedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "payments_processing_failed_total",
	Help: "Total number of payment processing failures, by retryability.",
}, []string{"retryable"})

// ProcessingDuration records how long a full processing attempt
// (including any in-process retries) takes.
var ProcessingDuration = promauto.NewHistogram(prometheus.HistogramOpts{
	Name:    "payment_processing_duration_seconds",
	Help:    "Duration of a payment processing attempt, in seconds.",
	Buckets: prometheus.DefBuckets,
})
