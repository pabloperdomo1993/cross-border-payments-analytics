// Package workerpool implements a bounded worker pool: a fixed number of
// long-lived goroutines draining a fixed-capacity job channel. It is the
// concurrency primitive payment-processor uses to bound how many
// payment jobs run at once, regardless of how fast Kafka delivers
// messages.
package workerpool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrPoolClosed is returned by Execute once Shutdown has been called.
var ErrPoolClosed = errors.New("workerpool: pool is closed")

// Task is a unit of work executed by a pooled worker. Implementations
// must respect ctx cancellation/deadline rather than running unbounded.
type Task func(ctx context.Context) error

type job struct {
	ctx    context.Context
	task   Task
	result chan<- error
}

// Pool is a bounded worker pool. The zero value is not usable; construct
// one with New.
type Pool struct {
	jobs chan job

	// closedMu/closed guard the jobs channel against the classic
	// "send on closed channel" panic: Go's select does not treat a
	// send on an already-closed channel as blocking — if that case
	// were ever selectable concurrently with Shutdown's close(jobs),
	// it could be chosen and panic. Holding closedMu for reading
	// across the submit (send) prevents Shutdown (which takes the
	// write lock before closing) from ever closing the channel while
	// a send to it is in flight.
	closedMu sync.RWMutex
	closed   bool
	closeOne sync.Once

	wg sync.WaitGroup

	capacity int
	active   atomic.Int64
}

// New builds a Pool with workerCount long-lived workers draining a job
// queue with capacity queueSize. Both must be positive; validating
// configuration values is the caller's responsibility (see
// internal/config) — New itself panics on nonsensical input since that
// indicates a programming error, not a runtime condition.
func New(workerCount, queueSize int) *Pool {
	if workerCount <= 0 {
		panic("workerpool: workerCount must be > 0")
	}
	if queueSize <= 0 {
		panic("workerpool: queueSize must be > 0")
	}

	p := &Pool{
		jobs:     make(chan job, queueSize),
		capacity: queueSize,
	}

	p.wg.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go p.worker()
	}

	return p
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for j := range p.jobs {
		p.active.Add(1)
		err := j.task(j.ctx)
		p.active.Add(-1)
		j.result <- err
	}
}

// Execute submits task to the pool and blocks until it has been
// accepted AND completed, or ctx is done, or the pool is closed.
//
// Submission itself blocks when the job queue is full — this is the
// pool's backpressure mechanism: a caller flooding the pool faster than
// workers can drain it is naturally slowed down by this call, rather
// than the pool growing an unbounded queue or spawning unbounded
// goroutines.
//
// If ctx is done before task starts running, Execute returns ctx.Err()
// without running task. If ctx is done while task is already running,
// Execute returns ctx.Err() without waiting further for task's result;
// task is expected to observe ctx itself and stop promptly (the result
// is still delivered into a buffered channel so the worker never
// blocks on a caller that has stopped waiting).
func (p *Pool) Execute(ctx context.Context, task Task) error {
	p.closedMu.RLock()
	if p.closed {
		p.closedMu.RUnlock()
		return ErrPoolClosed
	}

	resultCh := make(chan error, 1)
	select {
	case p.jobs <- job{ctx: ctx, task: task, result: resultCh}:
		p.closedMu.RUnlock()
	case <-ctx.Done():
		p.closedMu.RUnlock()
		return ctx.Err()
	}

	select {
	case err := <-resultCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Shutdown stops accepting new work (subsequent Execute calls return
// ErrPoolClosed), waits for already-enqueued and in-flight jobs to
// finish, then returns. It is safe to call exactly once; the job
// channel is closed here and only here, under the write lock so no
// concurrent Execute can be mid-send when it happens (see closedMu).
func (p *Pool) Shutdown() {
	p.closeOne.Do(func() {
		p.closedMu.Lock()
		p.closed = true
		close(p.jobs)
		p.closedMu.Unlock()
	})
	p.wg.Wait()
}

// ActiveWorkers reports how many workers are currently executing a task.
func (p *Pool) ActiveWorkers() int64 {
	return p.active.Load()
}

// QueueLen reports how many jobs are currently buffered (accepted but
// not yet picked up by a worker).
func (p *Pool) QueueLen() int {
	return len(p.jobs)
}

// Capacity reports the configured queue capacity.
func (p *Pool) Capacity() int {
	return p.capacity
}
