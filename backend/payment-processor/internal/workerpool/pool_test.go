package workerpool_test

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pabloperdomo1993/cross-border-payments-analytics/backend/payment-processor/internal/workerpool"
)

// TestPool_ConcurrencyLimit asserts that no more than workerCount tasks
// ever run simultaneously, even when far more than workerCount tasks are
// submitted at once. Determinism: the test waits for exactly
// workerCount tasks to signal they've started (via a buffered channel),
// rather than sleeping, before releasing them all at once.
func TestPool_ConcurrencyLimit(t *testing.T) {
	const workerCount = 5
	const totalJobs = workerCount * 6

	pool := workerpool.New(workerCount, totalJobs)
	defer pool.Shutdown()

	ctx := context.Background()

	var current, max atomic.Int32
	started := make(chan struct{}, totalJobs)
	release := make(chan struct{})

	var wg sync.WaitGroup
	for i := 0; i < totalJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pool.Execute(ctx, func(ctx context.Context) error {
				n := current.Add(1)
				for {
					m := max.Load()
					if n <= m || max.CompareAndSwap(m, n) {
						break
					}
				}
				started <- struct{}{}
				<-release
				current.Add(-1)
				return nil
			})
		}()
	}

	// Wait until exactly workerCount tasks are running (the pool is
	// saturated) before releasing them — this is the deterministic
	// proof that concurrency is bounded, not an assumption based on
	// timing.
	for i := 0; i < workerCount; i++ {
		<-started
	}

	// At this point max must already be workerCount: no more than
	// workerCount workers exist, so it cannot rise further before we
	// release anyone.
	if got := max.Load(); got != workerCount {
		t.Fatalf("expected exactly %d concurrent tasks once saturated, got %d", workerCount, got)
	}

	close(release)
	wg.Wait()

	if got := max.Load(); got > workerCount {
		t.Fatalf("max concurrent tasks = %d, want <= %d", got, workerCount)
	}
}

// TestPool_Backpressure asserts that once the worker and queue are both
// full, submitting another job blocks until capacity frees up, rather
// than growing the queue or dropping work. Determinism: readiness at
// each stage is confirmed via channels/QueueLen polling bounded by an
// overall test deadline, not by sleeping a guessed duration.
func TestPool_Backpressure(t *testing.T) {
	const queueSize = 1
	pool := workerpool.New(1, queueSize)
	defer pool.Shutdown()

	ctx := context.Background()
	deadline := time.After(5 * time.Second)

	// Job 1 occupies the single worker.
	job1Started := make(chan struct{})
	job1Release := make(chan struct{})
	job1Done := make(chan struct{})
	go func() {
		_ = pool.Execute(ctx, func(ctx context.Context) error {
			close(job1Started)
			<-job1Release
			return nil
		})
		close(job1Done)
	}()
	<-job1Started

	// Job 2 fills the single queue slot: accepted into the buffer
	// immediately (the worker already dequeued job 1), but it cannot
	// start running because the worker is busy. Note that
	// pool.Execute only returns once its task has both been accepted
	// AND completed — so "job2Done" below signals full completion,
	// not just acceptance; acceptance is observed separately via
	// QueueLen and "job2Started".
	job2Started := make(chan struct{})
	job2Release := make(chan struct{})
	job2Done := make(chan struct{})
	go func() {
		_ = pool.Execute(ctx, func(ctx context.Context) error {
			close(job2Started)
			<-job2Release
			return nil
		})
		close(job2Done)
	}()
	for pool.QueueLen() < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for job 2 to fill the queue")
		default:
			runtime.Gosched()
		}
	}

	// Job 3: worker busy AND queue full — submitting it must block.
	job3Done := make(chan struct{})
	go func() {
		_ = pool.Execute(ctx, func(ctx context.Context) error { return nil })
		close(job3Done)
	}()

	select {
	case <-job3Done:
		t.Fatal("job 3 was accepted despite the worker and queue both being full — no backpressure")
	case <-time.After(100 * time.Millisecond):
		// Expected: job 3 is still blocked trying to submit.
	}

	// Free job 1 -> worker dequeues job 2 and starts running it -> the
	// queue now has a free slot for job 3's still-blocked submission.
	close(job1Release)
	<-job1Done
	<-job2Started

	// Job 3 may now be accepted into the freed queue slot, but it must
	// not run to completion yet: the single worker is still busy
	// running job 2.
	select {
	case <-job3Done:
		t.Fatal("job 3 completed before job 2 finished — the single worker can't run both at once")
	case <-time.After(50 * time.Millisecond):
	}

	close(job2Release)
	<-job2Done

	select {
	case <-job3Done:
	case <-deadline:
		t.Fatal("timed out waiting for job 3 to run after capacity freed")
	}
}

// TestPool_ContextCancellation asserts that a task respecting ctx stops
// promptly when the caller's context is cancelled, and Execute returns
// the context error instead of hanging.
func TestPool_ContextCancellation(t *testing.T) {
	pool := workerpool.New(2, 2)
	defer pool.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())

	taskStarted := make(chan struct{})
	taskDone := make(chan error, 1)
	go func() {
		taskDone <- pool.Execute(ctx, func(ctx context.Context) error {
			close(taskStarted)
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	<-taskStarted
	cancel()

	select {
	case err := <-taskDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return after context cancellation")
	}
}

// TestPool_JobTimeout asserts that a per-job timeout derived via
// context.WithTimeout ends the job with context.DeadlineExceeded,
// without the worker being occupied indefinitely.
func TestPool_JobTimeout(t *testing.T) {
	pool := workerpool.New(1, 1)
	defer pool.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := pool.Execute(ctx, func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	// The worker must be free again immediately afterwards, proving it
	// wasn't permanently occupied by the timed-out job.
	err = pool.Execute(context.Background(), func(ctx context.Context) error { return nil })
	if err != nil {
		t.Fatalf("expected worker to be available after timeout, got error: %v", err)
	}
}

// TestPool_GracefulShutdown asserts Shutdown waits for in-flight work,
// returns without panicking, and that Execute calls after Shutdown fail
// cleanly with ErrPoolClosed instead of panicking on a closed channel.
func TestPool_GracefulShutdown(t *testing.T) {
	pool := workerpool.New(2, 2)

	var completed atomic.Bool
	release := make(chan struct{})
	started := make(chan struct{})

	go func() {
		_ = pool.Execute(context.Background(), func(ctx context.Context) error {
			close(started)
			<-release
			completed.Store(true)
			return nil
		})
	}()
	<-started

	shutdownDone := make(chan struct{})
	go func() {
		pool.Shutdown()
		close(shutdownDone)
	}()

	// Shutdown must wait for the in-flight job rather than aborting it.
	select {
	case <-shutdownDone:
		t.Fatal("Shutdown returned before the in-flight job finished")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)

	select {
	case <-shutdownDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not return after the in-flight job finished")
	}

	if !completed.Load() {
		t.Fatal("expected in-flight job to complete before Shutdown returned")
	}

	// Execute after Shutdown must not panic.
	err := pool.Execute(context.Background(), func(ctx context.Context) error { return nil })
	if !errors.Is(err, workerpool.ErrPoolClosed) {
		t.Fatalf("expected ErrPoolClosed after Shutdown, got %v", err)
	}
}

// TestPool_ExecuteRaceWithShutdown exercises Execute and Shutdown
// concurrently under the race detector to confirm the closedMu guard
// actually prevents a send-on-closed-channel panic.
func TestPool_ExecuteRaceWithShutdown(t *testing.T) {
	pool := workerpool.New(4, 8)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pool.Execute(context.Background(), func(ctx context.Context) error { return nil })
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.Shutdown()
	}()

	wg.Wait()
}

// TestPool_NoGoroutineLeakAfterShutdown verifies that once Shutdown
// returns, all worker goroutines have actually exited — not just that
// Shutdown's own call returned, but that the runtime's goroutine count
// has gone back down to (approximately) its pre-pool baseline. A leak
// here would mean worker goroutines are blocked forever (e.g. stuck on
// a channel operation) even though the caller believes shutdown
// completed.
func TestPool_NoGoroutineLeakAfterShutdown(t *testing.T) {
	baseline := goroutineCountStable(t)

	const workerCount = 8
	pool := workerpool.New(workerCount, 16)

	var wg sync.WaitGroup
	for i := 0; i < workerCount*3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = pool.Execute(context.Background(), func(ctx context.Context) error { return nil })
		}()
	}
	wg.Wait()

	pool.Shutdown()

	after := goroutineCountStable(t)
	// Allow a small tolerance: the test runner itself and any
	// in-flight cleanup goroutines from other tests can wobble this by
	// one or two: the workerCount pool goroutines specifically must be
	// gone, which a large tolerance would mask, so keep it tight.
	const tolerance = 2
	if after > baseline+tolerance {
		t.Errorf("expected goroutine count to return to ~baseline after Shutdown: baseline=%d, after=%d (tolerance=%d)", baseline, after, tolerance)
	}
}

// goroutineCountStable samples runtime.NumGoroutine() after letting the
// scheduler settle, to avoid flaking on goroutines that are mid-exit
// rather than actually leaked.
func goroutineCountStable(t *testing.T) int {
	t.Helper()
	runtime.Gosched()
	last := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		time.Sleep(2 * time.Millisecond)
		runtime.Gosched()
		current := runtime.NumGoroutine()
		if current == last {
			return current
		}
		last = current
	}
	return last
}
