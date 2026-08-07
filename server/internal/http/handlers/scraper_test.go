// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func TestResolveSyncTimeout(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected time.Duration
	}{
		{"zero uses default", 0, 30 * time.Second},
		{"negative uses default", -5, 30 * time.Second},
		{"custom 60s", 60, 60 * time.Second},
		{"exactly 120s", 120, 120 * time.Second},
		{"above max is capped at 120s", 200, 120 * time.Second},
		{"1s minimum valid", 1, 1 * time.Second},
		// time.Duration(seconds) * time.Second overflows int64 nanoseconds past
		// roughly 9.2e9 seconds and wraps negative, which a clamp applied after
		// the conversion does not catch.
		{"overflowing value is capped, not wrapped", 10000000000, 120 * time.Second},
		{"large power of two is capped, not wrapped", 1 << 40, 120 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSyncTimeout(tt.input)
			if got != tt.expected {
				t.Errorf("resolveSyncTimeout(%d) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestScrapeSync_ReturnsCompletedJob(t *testing.T) {
	completedAt := time.Now().UTC()
	st := &fakeStore{record: &store.JobRecord{
		Status:      models.JobStatusCompleted,
		JobType:     models.JobTypeURLScraper,
		URL:         "https://example.com",
		Result:      `{"markdown":"# Example\n\nbody text","html":"<h1>Example</h1>"}`,
		DurationMs:  1234,
		CompletedAt: &completedAt,
	}}

	resp := doScrapeSync(t, st, `{"url":"https://example.com"}`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body models.JobResponse
	decodeBody(t, resp, &body)

	if body.Status != models.JobStatusCompleted {
		t.Errorf("status = %q, want %q", body.Status, models.JobStatusCompleted)
	}
	if body.Markdown == nil || *body.Markdown != "# Example\n\nbody text" {
		t.Errorf("markdown = %v, want the stored markdown", body.Markdown)
	}
	if body.DurationMs == nil || *body.DurationMs != 1234 {
		t.Errorf("durationMs = %v, want 1234", body.DurationMs)
	}
	if body.ID == "" {
		t.Error("id is empty; the response should carry the generated job id")
	}
}

func TestScrapeSync_FailedJobReturnsError(t *testing.T) {
	st := &fakeStore{record: &store.JobRecord{
		Status:  models.JobStatusFailed,
		JobType: models.JobTypeURLScraper,
		URL:     "https://example.com",
		Error:   "all handlers failed: HTTP 403: Forbidden",
	}}

	resp := doScrapeSync(t, st, `{"url":"https://example.com"}`)
	defer resp.Body.Close()

	// A job that ran and failed is still a completed request/response cycle.
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body models.JobResponse
	decodeBody(t, resp, &body)

	if body.Status != models.JobStatusFailed {
		t.Errorf("status = %q, want %q", body.Status, models.JobStatusFailed)
	}
	if body.Error == nil || *body.Error == "" {
		t.Error("error field should be populated for a failed job")
	}
}

// A job that never leaves `pending` must hit the deadline and return 408 with
// the documented pointer to the async endpoint.
func TestScrapeSync_TimesOutWhileStillProcessing(t *testing.T) {
	st := &fakeStore{record: &store.JobRecord{
		Status:  models.JobStatusProcessing,
		JobType: models.JobTypeURLScraper,
		URL:     "https://example.com",
	}}

	start := time.Now()
	resp := doScrapeSync(t, st, `{"url":"https://example.com","timeout":1}`)
	defer resp.Body.Close()
	elapsed := time.Since(start)

	if resp.StatusCode != fiber.StatusRequestTimeout {
		t.Fatalf("status = %d, want 408", resp.StatusCode)
	}
	if elapsed > 10*time.Second {
		t.Errorf("took %v; the 1s timeout from the request body was not honoured", elapsed)
	}

	var body models.ErrorResponse
	decodeBody(t, resp, &body)

	if body.Error != "timeout" {
		t.Errorf("error = %q, want %q", body.Error, "timeout")
	}
	if !strings.Contains(body.Message, "/v1/url-scraper/") {
		t.Errorf("message = %q, want it to point at the async polling endpoint", body.Message)
	}
}

func TestScrapeSync_RejectsInvalidURL(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{"missing url", `{}`},
		{"empty url", `{"url":""}`},
		{"unsupported scheme", `{"url":"ftp://example.com/file"}`},
		{"no host", `{"url":"https://"}`},
		{"not a url", `{"url":"just some text"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{}
			resp := doScrapeSync(t, st, tt.body)
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if st.created {
				t.Error("a job was created for an invalid URL; validation must run first")
			}

			var body models.ErrorResponse
			decodeBody(t, resp, &body)
			if body.Error != "invalid_url" && body.Error != "invalid_request" {
				t.Errorf("error = %q, want invalid_url or invalid_request", body.Error)
			}
		})
	}
}

func TestScrapeSync_RejectsMalformedBody(t *testing.T) {
	st := &fakeStore{}
	resp := doScrapeSync(t, st, `{"url":`)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// --- helpers ---

// doScrapeSync wires a ScraperHandler to the given store and issues one
// POST /v1/scrape with the supplied JSON body.
func doScrapeSync(t *testing.T, st store.JobStore, body string) *http.Response {
	t.Helper()

	app := fiber.New()
	// The pool is buffered and never drained here: Submit only needs to accept
	// the job, since the fake store decides what the poll loop observes.
	h := NewScraperHandler(st, worker.NewPool(nil, 0, 16, time.Minute))
	app.Post("/v1/scrape", h.ScrapeSync)

	req := httptest.NewRequest(http.MethodPost, "/v1/scrape", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Generous ceiling: app.Test defaults to 1s, and the poll loop ticks at
	// syncPollInterval.
	resp, err := app.Test(req, 15000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func decodeBody(t *testing.T, resp *http.Response, into any) {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if err := json.Unmarshal(raw, into); err != nil {
		t.Fatalf("unmarshal %q: %v", raw, err)
	}
}

// fakeStore is a JobStore whose GetJob always reports the same record, so a
// test can pin the state ScrapeSync's poll loop observes.
type fakeStore struct {
	mu      sync.Mutex
	record  *store.JobRecord
	created bool
}

func (f *fakeStore) CreateJob(_ context.Context, _ store.JobRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.created = true
	return nil
}

func (f *fakeStore) GetJob(_ context.Context, id string) (*store.JobRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.record == nil {
		return nil, fmt.Errorf("not found")
	}
	r := *f.record
	r.ID = id
	return &r, nil
}

func (f *fakeStore) GetChildJobs(context.Context, string) ([]store.JobRecord, error) {
	return nil, nil
}
func (f *fakeStore) UpdateStatus(context.Context, string, string, *string, *int) error { return nil }
func (f *fakeStore) UpdateCompleted(context.Context, string, int, int) error           { return nil }
func (f *fakeStore) StoreResult(context.Context, string, string) error                 { return nil }
func (f *fakeStore) UpdateParentBatchStatus(context.Context, string) error             { return nil }
func (f *fakeStore) Ping(context.Context) error                                        { return nil }
