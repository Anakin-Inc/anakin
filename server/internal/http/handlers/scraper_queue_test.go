// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

// saturatedPool has no workers and no buffer, so every Submit is refused —
// the state a real pool reaches when WORKER_POOL_SIZE workers are busy and
// JOB_BUFFER_SIZE jobs are queued, without any timing dependence.
func saturatedPool() *worker.Pool {
	return worker.NewPool(nil, 0, 0, time.Minute)
}

func newTestApp(s store.JobStore, pool *worker.Pool) *fiber.App {
	app := fiber.New()
	h := NewScraperHandler(s, pool)
	app.Post("/v1/scrape", h.ScrapeSync)
	app.Post("/v1/url-scraper", h.CreateJob)
	app.Post("/v1/url-scraper/batch", h.CreateBatchJob)
	return app
}

func post(t *testing.T, app *fiber.App, path, body string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

// Submit used to block the request goroutine once the queue filled, so these
// endpoints stopped responding rather than shedding load.
func TestHandlers_RejectedJobReturns503(t *testing.T) {
	for _, tt := range []struct{ name, path, body string }{
		{"async", "/v1/url-scraper", `{"url":"https://example.com"}`},
		{"sync", "/v1/scrape", `{"url":"https://example.com"}`},
		{"batch", "/v1/url-scraper/batch", `{"urls":["https://example.com/1","https://example.com/2"]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &stubStore{}
			resp := post(t, newTestApp(s, saturatedPool()), tt.path, tt.body)
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503", resp.StatusCode)
			}
			if got := resp.Header.Get("Retry-After"); got == "" {
				t.Error("Retry-After header is missing; clients have no backoff hint")
			}
		})
	}
}

// A job row is created before the pool is asked to take it. If the pool refuses,
// that row must not be left pending forever.
func TestHandlers_RejectedJobIsMarkedFailed(t *testing.T) {
	s := &stubStore{}
	resp := post(t, newTestApp(s, saturatedPool()), "/v1/url-scraper", `{"url":"https://example.com"}`)
	defer resp.Body.Close()

	statuses := s.statuses()
	if len(statuses) != 1 {
		t.Fatalf("recorded %d status updates (%v), want 1", len(statuses), statuses)
	}
	if statuses[0] != models.JobStatusFailed {
		t.Errorf("status = %q, want %q", statuses[0], models.JobStatusFailed)
	}
}

// Every child of a rejected batch is recorded as failed, so the batch does not
// report children that will never run as pending.
func TestHandlers_RejectedBatchMarksEveryChildFailed(t *testing.T) {
	s := &stubStore{}
	resp := post(t, newTestApp(s, saturatedPool()), "/v1/url-scraper/batch",
		`{"urls":["https://example.com/1","https://example.com/2","https://example.com/3"]}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when nothing could be queued", resp.StatusCode)
	}
	statuses := s.statuses()
	if len(statuses) != 3 {
		t.Fatalf("recorded %d status updates, want one per child (3)", len(statuses))
	}
	for i, st := range statuses {
		if st != models.JobStatusFailed {
			t.Errorf("child %d status = %q, want %q", i, st, models.JobStatusFailed)
		}
	}
}

// A healthy pool must be unaffected.
func TestHandlers_AcceptedJobStillReturns201(t *testing.T) {
	pool := worker.NewPool(nil, 0, 8, time.Minute) // buffered, no workers consuming
	resp := post(t, newTestApp(&stubStore{}, pool), "/v1/url-scraper", `{"url":"https://example.com"}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusCreated {
		t.Errorf("status = %d, want 201 when the queue has space", resp.StatusCode)
	}
}

// stubStore records status transitions and never fails.
type stubStore struct {
	mu       sync.Mutex
	recorded []string
}

func (s *stubStore) statuses() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.recorded...)
}

func (s *stubStore) CreateJob(context.Context, store.JobRecord) error { return nil }

func (s *stubStore) GetJob(_ context.Context, id string) (*store.JobRecord, error) {
	return nil, fmt.Errorf("not found")
}

func (s *stubStore) GetChildJobs(context.Context, string) ([]store.JobRecord, error) {
	return nil, nil
}

func (s *stubStore) UpdateStatus(_ context.Context, _, status string, _ *string, _ *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recorded = append(s.recorded, status)
	return nil
}

func (s *stubStore) UpdateCompleted(context.Context, string, int, int) error { return nil }
func (s *stubStore) StoreResult(context.Context, string, string) error       { return nil }
func (s *stubStore) UpdateParentBatchStatus(context.Context, string) error   { return nil }
func (s *stubStore) Ping(context.Context) error                              { return nil }
