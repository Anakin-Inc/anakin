// SPDX-License-Identifier: AGPL-3.0-or-later

package processor

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/handler"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

// ctxStrictStore behaves like database/sql: a query whose context is already done is
// rejected before it reaches the driver. MemoryStore ignores ctx entirely, which is why
// this bug only ever showed up with DATABASE_URL set.
type ctxStrictStore struct {
	mu     sync.Mutex
	status string
	errMsg string
	result string
}

func (s *ctxStrictStore) record(ctx context.Context, apply func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	apply()
	return nil
}

func (s *ctxStrictStore) state() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.errMsg
}

func (s *ctxStrictStore) UpdateStatus(ctx context.Context, _, status string, errMsg *string, _ *int) error {
	return s.record(ctx, func() {
		s.status = status
		if errMsg != nil {
			s.errMsg = *errMsg
		}
	})
}

func (s *ctxStrictStore) UpdateCompleted(ctx context.Context, _ string, _, _ int) error {
	return s.record(ctx, func() { s.status = models.JobStatusCompleted })
}

func (s *ctxStrictStore) StoreResult(ctx context.Context, _, resultJSON string) error {
	return s.record(ctx, func() { s.result = resultJSON })
}

func (s *ctxStrictStore) UpdateParentBatchStatus(ctx context.Context, _ string) error {
	return ctx.Err()
}

func (s *ctxStrictStore) CreateJob(ctx context.Context, _ store.JobRecord) error { return ctx.Err() }
func (s *ctxStrictStore) GetJob(ctx context.Context, _ string) (*store.JobRecord, error) {
	return nil, ctx.Err()
}
func (s *ctxStrictStore) GetChildJobs(ctx context.Context, _ string) ([]store.JobRecord, error) {
	return nil, ctx.Err()
}
func (s *ctxStrictStore) Ping(ctx context.Context) error { return ctx.Err() }

// hangingHandler stands in for a scrape that outlives JOB_TIMEOUT.
type hangingHandler struct{}

func (hangingHandler) Name() string                                           { return "hanging" }
func (hangingHandler) CanHandle(context.Context, *models.HandlerRequest) bool { return true }
func (hangingHandler) IsHealthy() bool                                        { return true }
func (hangingHandler) Scrape(ctx context.Context, _ *models.HandlerRequest) (*models.ScrapeResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestProcessJob_RecordsFailureAfterJobTimeout(t *testing.T) {
	st := &ctxStrictStore{}
	chain := handler.NewChain([]handler.ScrapingHandler{hangingHandler{}})
	p := NewProcessor(st, chain, nil, nil, nil, nil)

	// Mirrors worker.Pool: the job context carries JOB_TIMEOUT.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := p.ProcessJob(ctx, models.JobMessage{
		JobID:   "job-1",
		URL:     "https://example.com",
		JobType: models.JobTypeURLScraper,
	}); err == nil {
		t.Fatal("expected ProcessJob to report the timeout")
	}

	status, errMsg := st.state()
	if status != models.JobStatusFailed {
		t.Errorf("job status = %q, want %q — a timed-out job must not be left mid-flight", status, models.JobStatusFailed)
	}
	if errMsg == "" {
		t.Error("expected the failure reason to be persisted")
	}
}

func TestProcessJob_RecordsFailureAfterCancellation(t *testing.T) {
	st := &ctxStrictStore{}
	chain := handler.NewChain([]handler.ScrapingHandler{hangingHandler{}})
	p := NewProcessor(st, chain, nil, nil, nil, nil)

	// Mirrors shutdown: bgCancel() cancels the parent context of in-flight jobs.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := p.ProcessJob(ctx, models.JobMessage{
		JobID:   "job-2",
		URL:     "https://example.com",
		JobType: models.JobTypeURLScraper,
	}); err == nil {
		t.Fatal("expected ProcessJob to report the cancellation")
	}

	if status, _ := st.state(); status != models.JobStatusFailed {
		t.Errorf("job status = %q, want %q — a job cancelled at shutdown must not be left mid-flight", status, models.JobStatusFailed)
	}
}
