// SPDX-License-Identifier: AGPL-3.0-or-later

package worker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// blockingHandler holds every job until release is closed, so a test can pin
// all workers as busy and fill the buffer deterministically.
type blockingHandler struct {
	release chan struct{}
	mu      sync.Mutex
	seen    []string
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{release: make(chan struct{})}
}

func (h *blockingHandler) ProcessJob(_ context.Context, msg models.JobMessage) error {
	h.mu.Lock()
	h.seen = append(h.seen, msg.JobID)
	h.mu.Unlock()
	<-h.release
	return nil
}

func (h *blockingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.seen)
}

// countingHandler records jobs and returns immediately.
type countingHandler struct {
	mu   sync.Mutex
	seen []string
}

func (h *countingHandler) ProcessJob(_ context.Context, msg models.JobMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seen = append(h.seen, msg.JobID)
	return nil
}

func (h *countingHandler) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.seen)
}

func job(id string) models.JobMessage {
	return models.JobMessage{JobID: id, URL: "https://example.com", JobType: models.JobTypeURLScraper}
}

// Submit used to be a bare channel send, so a full buffer parked the request
// goroutine indefinitely. It must refuse instead.
func TestPool_SubmitReturnsErrQueueFullInsteadOfBlocking(t *testing.T) {
	h := newBlockingHandler()
	defer close(h.release)

	// One worker, buffer of 2: the worker takes one job and blocks, then two
	// more fill the buffer.
	p := NewPool(h, 1, 2, time.Minute)
	p.Start(context.Background())

	// Wait for the worker to pick up its job so the buffer state is settled.
	if err := p.Submit(job("in-flight")); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	waitFor(t, func() bool { return h.count() == 1 }, "worker to pick up the first job")

	for i := 0; i < 2; i++ {
		if err := p.Submit(job("buffered")); err != nil {
			t.Fatalf("Submit while buffer had space: %v", err)
		}
	}

	// The buffer is now full and the only worker is blocked. This must return
	// rather than park.
	done := make(chan error, 1)
	go func() { done <- p.Submit(job("overflow")) }()

	select {
	case err := <-done:
		if !errors.Is(err, ErrQueueFull) {
			t.Errorf("Submit on a full queue = %v, want ErrQueueFull", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Submit blocked on a full queue instead of returning ErrQueueFull")
	}
}

func TestPool_SubmitSucceedsWhenBufferHasSpace(t *testing.T) {
	h := &countingHandler{}
	p := NewPool(h, 2, 10, time.Minute)
	p.Start(context.Background())

	for i := 0; i < 5; i++ {
		if err := p.Submit(job("job")); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	p.Drain()

	if got := h.count(); got != 5 {
		t.Errorf("processed %d jobs, want 5", got)
	}
}

// Drain closes the job channel. A Submit racing that close used to panic with
// "send on closed channel", taking the process down.
func TestPool_SubmitAfterDrainReturnsErrPoolClosed(t *testing.T) {
	p := NewPool(&countingHandler{}, 1, 4, time.Minute)
	p.Start(context.Background())
	p.Drain()

	err := p.Submit(job("after-drain"))
	if !errors.Is(err, ErrPoolClosed) {
		t.Errorf("Submit after Drain = %v, want ErrPoolClosed", err)
	}
}

// The shutdown window is real: main.go bounds app.ShutdownWithContext at 30s
// but ScrapeSync can legitimately still be running at 120s, so a handler can
// call Submit while Drain is closing the channel.
//
// Panics on the previous implementation.
func TestPool_SubmitRacingDrainDoesNotPanic(t *testing.T) {
	for attempt := 0; attempt < 20; attempt++ {
		p := NewPool(&countingHandler{}, 2, 8, time.Minute)
		p.Start(context.Background())

		var wg sync.WaitGroup
		start := make(chan struct{})

		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				// Only these two outcomes are acceptable; a panic here fails the
				// whole test binary.
				if err := p.Submit(job("racer")); err != nil && !errors.Is(err, ErrPoolClosed) && !errors.Is(err, ErrQueueFull) {
					t.Errorf("Submit = %v, want nil, ErrPoolClosed or ErrQueueFull", err)
				}
			}()
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			p.Drain()
		}()

		close(start)
		wg.Wait()
	}
}

// main.go calls Drain during shutdown; a second call from a test helper or a
// future cleanup path must not double-close the channel.
func TestPool_DrainIsIdempotent(t *testing.T) {
	p := NewPool(&countingHandler{}, 1, 4, time.Minute)
	p.Start(context.Background())

	p.Drain()
	p.Drain() // must not panic on a second close
}

func TestPool_DrainWaitsForInFlightJobs(t *testing.T) {
	h := newBlockingHandler()
	p := NewPool(h, 2, 8, time.Minute)
	p.Start(context.Background())

	for i := 0; i < 2; i++ {
		if err := p.Submit(job("slow")); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}
	waitFor(t, func() bool { return h.count() == 2 }, "both workers to pick up jobs")

	drained := make(chan struct{})
	go func() { p.Drain(); close(drained) }()

	select {
	case <-drained:
		t.Fatal("Drain returned while jobs were still in flight")
	case <-time.After(200 * time.Millisecond):
	}

	close(h.release)

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("Drain did not return after in-flight jobs finished")
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
