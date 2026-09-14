// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

// recordingHandler notes, for every job it runs, whether the job's context was still
// live when the job finished. A cancelled context is the symptom of shutdown pulling
// the rug out from under an in-flight job.
type recordingHandler struct {
	work time.Duration

	mu        sync.Mutex
	completed []string
	cancelled []string
}

func (h *recordingHandler) ProcessJob(ctx context.Context, msg models.JobMessage) error {
	select {
	case <-time.After(h.work):
	case <-ctx.Done():
		h.mu.Lock()
		h.cancelled = append(h.cancelled, msg.JobID)
		h.mu.Unlock()
		return ctx.Err()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if err := ctx.Err(); err != nil {
		h.cancelled = append(h.cancelled, msg.JobID)
		return err
	}
	h.completed = append(h.completed, msg.JobID)
	return nil
}

func (h *recordingHandler) counts() (completed, cancelled int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.completed), len(h.cancelled)
}

// The shutdown path must let queued and in-flight jobs finish. Worker job contexts
// descend from the context given to pool.Start, so draining has to happen before that
// context is cancelled — otherwise every job the pool still holds dies instantly.
func TestDrainWorkersLetsInFlightJobsFinish(t *testing.T) {
	handler := &recordingHandler{work: 50 * time.Millisecond}
	pool := worker.NewPool(handler, 2, 10, time.Minute)

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	pool.Start(bgCtx)

	for i := 0; i < 6; i++ {
		pool.Submit(models.JobMessage{JobID: string(rune('a' + i)), URL: "https://example.com"})
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if !drainWorkers(shutdownCtx, pool) {
		t.Fatal("drainWorkers reported a timeout it had 30s of budget to avoid")
	}
	bgCancel()

	completed, cancelled := handler.counts()
	if cancelled != 0 {
		t.Errorf("%d jobs were cancelled during shutdown, want 0 — draining must precede cancellation", cancelled)
	}
	if completed != 6 {
		t.Errorf("%d of 6 jobs completed, want all of them", completed)
	}
}

// Draining must not be able to hang shutdown: a job that outlives the budget gives up
// the wait so the caller can cancel it.
func TestDrainWorkersGivesUpWhenTheBudgetExpires(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	pool := worker.NewPool(blockingHandler{release: release}, 1, 1, time.Minute)

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()
	pool.Start(bgCtx)
	pool.Submit(models.JobMessage{JobID: "stuck", URL: "https://example.com"})

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if drainWorkers(shutdownCtx, pool) {
		t.Error("drainWorkers reported success while a job was still blocked")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("drainWorkers blocked for %s past its budget", elapsed)
	}
}

type blockingHandler struct{ release <-chan struct{} }

func (h blockingHandler) ProcessJob(ctx context.Context, _ models.JobMessage) error {
	select {
	case <-h.release:
	case <-ctx.Done():
	}
	return nil
}

func TestStorageMode(t *testing.T) {
	if got := storageMode(nil); got != "memory" {
		t.Errorf("storageMode(nil) = %q, want %q", got, "memory")
	}
}
