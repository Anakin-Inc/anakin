// SPDX-License-Identifier: AGPL-3.0-or-later

package worker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// Errors returned by Submit. Callers are expected to translate both into a
// 503 rather than retrying inline.
var (
	// ErrQueueFull means every worker is busy and the buffer is full. The job
	// was not accepted and nothing was queued.
	ErrQueueFull = errors.New("job queue is full")

	// ErrPoolClosed means the pool is draining and no longer accepts work.
	ErrPoolClosed = errors.New("worker pool is shutting down")
)

// JobHandler processes a single job message.
type JobHandler interface {
	ProcessJob(ctx context.Context, msg models.JobMessage) error
}

// Pool is a bounded worker pool that processes jobs from an internal channel.
type Pool struct {
	jobs       chan models.JobMessage
	handler    JobHandler
	size       int
	jobTimeout time.Duration
	wg         sync.WaitGroup

	// closeMu guards closed and, critically, the send in Submit. Submit holds
	// it for reading across the send; Drain takes it for writing before closing
	// the channel, so a send can never be in flight when the close happens.
	closeMu sync.RWMutex
	closed  bool
}

// NewPool creates a worker pool.
func NewPool(handler JobHandler, size, bufferSize int, jobTimeout time.Duration) *Pool {
	return &Pool{
		jobs:       make(chan models.JobMessage, bufferSize),
		handler:    handler,
		size:       size,
		jobTimeout: jobTimeout,
	}
}

// Start launches the worker goroutines.
func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.size; i++ {
		p.wg.Add(1)
		go p.worker(ctx, i)
	}
	slog.Info("worker pool started", "workers", p.size, "buffer", cap(p.jobs))
}

func (p *Pool) worker(parentCtx context.Context, id int) {
	defer p.wg.Done()
	for msg := range p.jobs {
		jobCtx, cancel := context.WithTimeout(parentCtx, p.jobTimeout)
		if err := p.handler.ProcessJob(jobCtx, msg); err != nil {
			slog.Error("job failed", "worker", id, "job_id", msg.JobID, "error", err)
		}
		cancel()
	}
	slog.Debug("worker exited", "worker", id)
}

// Submit offers a job to the pool without ever blocking the caller.
//
// It returns ErrQueueFull when every worker is busy and the buffer is full, and
// ErrPoolClosed once Drain has begun. In both cases the job was not accepted.
// Callers are request handlers, so blocking here would park the request
// goroutine and stall the API instead of shedding load.
func (p *Pool) Submit(msg models.JobMessage) error {
	// Held for reading across the send so Drain cannot close the channel
	// underneath it. The send is non-blocking, so this is never held for long.
	p.closeMu.RLock()
	defer p.closeMu.RUnlock()

	if p.closed {
		return ErrPoolClosed
	}

	select {
	case p.jobs <- msg:
		return nil
	default:
		slog.Warn("job queue full, rejecting job",
			"job_id", msg.JobID, "url", msg.URL, "buffer", cap(p.jobs), "workers", p.size)
		return ErrQueueFull
	}
}

// Drain stops accepting new jobs and waits for queued and in-flight ones to
// finish. Safe to call more than once.
func (p *Pool) Drain() {
	p.closeMu.Lock()
	if p.closed {
		p.closeMu.Unlock()
		return
	}
	p.closed = true
	// Taking the write lock has waited out every in-flight Submit, so no send
	// can race this close.
	close(p.jobs)
	p.closeMu.Unlock()

	p.wg.Wait()
	slog.Info("worker pool drained")
}
