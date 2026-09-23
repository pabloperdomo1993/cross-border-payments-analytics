# ADR-001: Bounded Worker Pool for Payment Processing

Status: Accepted

## Context

`payment-processor` consumes payment events from the `payments.created`
Kafka topic and must process each one (validate, classify, produce an
outcome to `payments.processed` or `payments.dlq`). We want concurrent
processing — a single-threaded consumer would leave the CPU idle while
waiting on I/O — but concurrency in Go is cheap to *start* and easy to
get wrong at scale if left unbounded.

## Options

### Option 1: Sequential processing

Process one message at a time, fully, before reading the next.

Advantages:
- Trivially correct — no concurrency bugs possible.
- Simplest possible code and mental model.

Disadvantages:
- Throughput is capped by one message's full processing latency; I/O
  wait time (e.g. the outcome publish) is pure idle time, not
  overlapped with the next message's work.
- Does not use the machine's available concurrency at all.

### Option 2: One goroutine per Kafka message

Spawn a new goroutine for every message as it arrives.

Advantages:
- Maximum theoretical concurrency — nothing waits on anything else.
- Very little code: `go handle(msg)`.

Disadvantages:
- **No ceiling.** A burst of Kafka traffic (a backlog being caught up,
  or simply a busy period) spawns an unbounded number of goroutines,
  each holding its own stack, its own in-flight HTTP/DB connections
  (if the processing step ever needs one), and its own timers.
- This directly produces **memory pressure** (goroutine stacks and
  in-flight message buffers), **connection pressure** (any downstream
  dependency — Kafka producer, database — sees an unbounded number of
  concurrent callers), **unstable latency** (the scheduler has to
  time-slice an ever-growing number of goroutines, so tail latency
  degrades unpredictably rather than staying flat), and **downstream
  overload** (nothing else in the system is designed to absorb
  unbounded concurrent load, so this simply moves the bottleneck
  somewhere less controlled).
- No natural backpressure mechanism: Kafka happily keeps delivering
  messages regardless of how many goroutines are already running.

### Option 3: Bounded worker pool

A fixed number of long-lived worker goroutines drain jobs from a
fixed-capacity channel; submitting a job blocks when the channel is full.

Advantages:
- Concurrency is bounded and configurable, independent of how bursty
  Kafka delivery is.
- Backpressure is automatic and free: a full queue makes the submitting
  side (here, the Kafka consumer's per-partition read loop) simply wait,
  rather than requiring a separate rate-limiting mechanism.
- Resource usage (goroutines, downstream connections, memory) has a
  known upper bound, which makes capacity planning and downstream
  protection possible.

Disadvantages:
- More code than either alternative: a bounded channel, a fixed set of
  worker goroutines, and — critically — a shutdown path that safely
  closes that channel exactly once without racing a concurrent
  `Execute` call into a closed channel.
- Choosing the wrong worker count/queue size still under- or
  over-provisions the service; it doesn't remove the tuning problem,
  it just makes it explicit and bounded instead of implicit and
  unbounded.

## Decision

Use a configurable bounded worker pool, implemented in
`backend/payment-processor/internal/workerpool`.

- A fixed number of long-lived worker **goroutines** (`WORKER_COUNT`,
  default 10) read jobs from a bounded Go **channel** (`WORKER_QUEUE_SIZE`,
  default 100) acting as the **bounded job queue**. Both are
  environment-configurable and validated (must be `> 0`) at startup —
  the service fails fast on an invalid value rather than silently
  running with a degenerate pool.
- Submitting a job (`Pool.Execute`) blocks on the channel send when the
  queue is full, which is the pool's **backpressure**: a burst of
  Kafka traffic naturally slows the consumer's fetch pace rather than
  growing memory or spawning more goroutines. This is **resource
  protection** by construction, not by a separate rate limiter.
- `Execute` also takes and respects a `context.Context`, so a caller
  blocked waiting for queue capacity unblocks on cancellation instead
  of hanging forever — this is what makes **graceful shutdown**
  possible: on `SIGINT`/`SIGTERM`, the Kafka consumer stops claiming
  new messages first, then `Pool.Shutdown()` closes the job channel
  (guarded by a `sync.Once` against a concurrent double-close panic)
  and waits for in-flight and already-queued jobs to finish, bounded by
  a timeout at the call site. See `docs/architecture.md §5` for the
  full shutdown sequence.
- This is already implemented, not aspirational — see
  `internal/workerpool/pool.go` and its tests in `pool_test.go`, and the
  concurrency-safety verification via `go test -race ./...`
  (`docs/testing-strategy.md` — Concurrency Tests).

Scalability exists at two independent levels:

1. **Within one instance**, by raising `WORKER_COUNT`/`WORKER_QUEUE_SIZE`
   — bounded by the machine's CPU/memory and by how much concurrent load
   downstream (Kafka producer, DLQ topic) can actually absorb.
2. **Across instances**, by running multiple `payment-processor`
   processes in the *same Kafka consumer group*. Kafka's consumer-group
   protocol assigns each partition to exactly one consumer instance, so
   adding instances increases parallelism up to the topic's partition
   count — independent of, and complementary to, each instance's own
   worker pool size. See `docs/architecture.md §5` for why true
   parallelism per instance is bounded by
   `min(WORKER_COUNT, partitions assigned to that instance)`.

## Trade-offs

We accept the added implementation complexity of a bounded pool — a
channel, a fixed worker set, and a shutdown path that must be race-free
under concurrent `Execute`/`Shutdown` calls — in exchange for bounded,
predictable resource usage and automatic backpressure. This is strictly
more code than either sequential processing or unbounded goroutines, and
it introduces two tunable parameters (`WORKER_COUNT`, `WORKER_QUEUE_SIZE`)
that must be sized sensibly for the actual workload; a misconfigured pool
(too small) simply moves the bottleneck into the bounded queue instead of
removing it. We accept this because the alternative — no ceiling at all —
fails in ways that are harder to diagnose and recover from in production
than "the queue is full and Kafka fetch has slowed down," which is
directly visible via the `payment_worker_queue_size`/
`payment_worker_queue_capacity` metrics (see
[003-observability.md](003-observability.md)).
