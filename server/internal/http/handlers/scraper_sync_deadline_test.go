// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

// stubStore is a JobStore whose GetJob behaviour is driven by the test.
type stubStore struct {
	getJob   func(id string) (*store.JobRecord, error)
	getCalls atomic.Int64
}

func (s *stubStore) CreateJob(context.Context, store.JobRecord) error { return nil }

func (s *stubStore) GetJob(_ context.Context, id string) (*store.JobRecord, error) {
	s.getCalls.Add(1)
	return s.getJob(id)
}

func (s *stubStore) GetChildJobs(context.Context, string) ([]store.JobRecord, error) {
	return nil, nil
}
func (s *stubStore) UpdateStatus(context.Context, string, string, *string, *int) error { return nil }
func (s *stubStore) UpdateCompleted(context.Context, string, int, int) error           { return nil }
func (s *stubStore) StoreResult(context.Context, string, string) error                 { return nil }
func (s *stubStore) UpdateParentBatchStatus(context.Context, string) error             { return nil }
func (s *stubStore) Ping(context.Context) error                                        { return nil }

// syncApp wires ScrapeSync onto a fiber app backed by st. The worker pool is
// created but never started, so submitted jobs are buffered and never run —
// the job stays unfinished and the store stub decides what polling observes.
func syncApp(st store.JobStore) *fiber.App {
	app := fiber.New()
	app.Post("/v1/scrape", NewScraperHandler(st, worker.NewPool(nil, 0, 16, time.Second), false).ScrapeSync)
	return app
}

// ScrapeSync must honour its deadline even when every store lookup fails.
// Regression test: the error path used to `continue`, which skipped the
// deadline check and left the handler polling forever.
func TestScrapeSyncTimesOutWhenStoreKeepsFailing(t *testing.T) {
	st := &stubStore{
		getJob: func(string) (*store.JobRecord, error) {
			return nil, errors.New("database is down")
		},
	}

	req := httptest.NewRequest("POST", "/v1/scrape",
		strings.NewReader(`{"url":"https://example.com","timeout":1}`))
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	// A 1s request timeout; 10s is a generous ceiling that still fails fast
	// if the handler regresses to spinning forever.
	resp, err := syncApp(st).Test(req, 10000)
	if err != nil {
		t.Fatalf("ScrapeSync never returned: %v", err)
	}
	elapsed := time.Since(start)

	if resp.StatusCode != fiber.StatusRequestTimeout {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusRequestTimeout)
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v to time out, want close to the requested 1s", elapsed)
	}
	if st.getCalls.Load() == 0 {
		t.Error("store was never polled")
	}
}

// A job that reaches a terminal state is still returned normally — the deadline
// fix must not swallow results.
func TestScrapeSyncReturnsTerminalJob(t *testing.T) {
	completedAt := time.Now().UTC()
	st := &stubStore{
		getJob: func(id string) (*store.JobRecord, error) {
			return &store.JobRecord{
				ID:          id,
				Status:      models.JobStatusCompleted,
				URL:         "https://example.com",
				JobType:     models.JobTypeURLScraper,
				Result:      `{"markdown":"# hello"}`,
				DurationMs:  42,
				CompletedAt: &completedAt,
			}, nil
		},
	}

	req := httptest.NewRequest("POST", "/v1/scrape",
		strings.NewReader(`{"url":"https://example.com","timeout":5}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := syncApp(st).Test(req, 10000)
	if err != nil {
		t.Fatalf("ScrapeSync never returned: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
}

// A store that recovers after a few failed polls should still produce the
// result rather than being cut short by the earlier errors.
func TestScrapeSyncRecoversAfterTransientStoreErrors(t *testing.T) {
	var calls atomic.Int64
	st := &stubStore{
		getJob: func(id string) (*store.JobRecord, error) {
			if calls.Add(1) <= 2 {
				return nil, errors.New("temporary failure")
			}
			return &store.JobRecord{
				ID: id, Status: models.JobStatusFailed,
				URL: "https://example.com", JobType: models.JobTypeURLScraper,
				Error: "scrape failed",
			}, nil
		},
	}

	req := httptest.NewRequest("POST", "/v1/scrape",
		strings.NewReader(`{"url":"https://example.com","timeout":10}`))
	req.Header.Set("Content-Type", "application/json")

	resp, err := syncApp(st).Test(req, 10000)
	if err != nil {
		t.Fatalf("ScrapeSync never returned: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	if calls.Load() < 3 {
		t.Errorf("store polled %d times, want at least 3", calls.Load())
	}
}
