package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/models"
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

// Submit adds a job to the pool. Non-blocking if buffer has space.
func (p *Pool) Submit(msg models.JobMessage) {
	p.jobs <- msg
}

// Drain closes the job channel and waits for all in-flight jobs to finish.
func (p *Pool) Drain() {
	close(p.jobs)
	p.wg.Wait()
	slog.Info("worker pool drained")
}
