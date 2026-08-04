// SPDX-License-Identifier: AGPL-3.0-or-later

package processor

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/gemini"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/handler"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/proxy"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/telemetry"
)

const (
	testHost = "blocked.example.com"
	testURL  = "https://blocked.example.com/product/1"
)

func TestIsBlockedErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"403 status error", &handler.StatusError{Code: http.StatusForbidden}, true},
		{"429 status error", &handler.StatusError{Code: http.StatusTooManyRequests}, true},
		{"404 status error", &handler.StatusError{Code: http.StatusNotFound}, false},
		{"503 status error", &handler.StatusError{Code: http.StatusServiceUnavailable}, false},
		{"plain error", errors.New("dial tcp: i/o timeout"), false},
		{"context deadline", context.DeadlineExceeded, false},
		{
			"403 wrapped the way Chain.Execute wraps it",
			wrapLikeChain(&handler.StatusError{Code: http.StatusForbidden}),
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBlockedErr(tt.err); got != tt.want {
				t.Errorf("isBlockedErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// End-to-end regression test for the reported bug: a 403 from the target must
// reach the proxy pool as a block, applying severePenalty and excluding the
// proxy from selection for that host. Before the fix the processor computed
// isBlocked from result.StatusCode, which is always nil on the failure path, so
// this always recorded an ordinary failure instead.
func TestProcessJob_TargetRefusalRecordsProxyBlock(t *testing.T) {
	// Two proxies, so one stays eligible after the other is blocked. With a
	// single-proxy pool SelectProxy falls back to returning it even when
	// blocked, which would mask the exclusion.
	pool := proxy.NewPool(nil, []string{"http://proxy-a:8080", "http://proxy-b:8080"})
	proc, jobID := newTestProcessor(t, pool, &handler.StatusError{Code: http.StatusForbidden})

	if err := proc.ProcessJob(context.Background(), jobMessage(jobID)); err == nil {
		t.Fatal("ProcessJob returned nil error for a 403 target")
	}

	// SelectProxy is called once per job and only the proxy actually used gets
	// a persisted score, so that entry identifies which one took the failure.
	blocked := scoredProxy(t, pool)

	sc := scoreFor(t, pool, blocked)
	// Beta starts at 1; a blocked failure adds severePenalty (10), an ordinary
	// failure adds 1. Asserting on the gap is what distinguishes the two paths.
	if sc.Beta != 11 {
		t.Errorf("Beta = %d, want 11 (prior 1 + severePenalty 10); got the ordinary-failure penalty instead", sc.Beta)
	}

	// The blocked proxy must now be excluded from selection for this host.
	for i := 0; i < 50; i++ {
		if got := pool.SelectProxy(testHost); got == blocked {
			t.Fatalf("SelectProxy returned the blocked proxy %q on attempt %d; it should be excluded for blockedTTL", got, i)
		}
	}
}

// A transient failure must not be treated as a block — otherwise a healthy
// proxy is penalised 10x and sidelined for five minutes over a timeout.
func TestProcessJob_TransientFailureDoesNotBlockProxy(t *testing.T) {
	const proxyURL = "http://proxy-a:8080"

	pool := proxy.NewPool(nil, []string{proxyURL})
	proc, jobID := newTestProcessor(t, pool, errors.New("dial tcp: i/o timeout"))

	if err := proc.ProcessJob(context.Background(), jobMessage(jobID)); err == nil {
		t.Fatal("ProcessJob returned nil error for a failing handler")
	}

	sc := scoreFor(t, pool, proxyURL)
	if sc.Beta != 2 {
		t.Errorf("Beta = %d, want 2 (prior 1 + ordinary failure 1)", sc.Beta)
	}

	if got := pool.SelectProxy(testHost); got != proxyURL {
		t.Errorf("SelectProxy = %q, want %q — a timeout must not exclude the proxy", got, proxyURL)
	}
}

// --- helpers ---

// wrapLikeChain reproduces the wrapping Chain.Execute applies to handler errors
// so the test exercises the same shape the processor sees at runtime.
func wrapLikeChain(err error) error {
	return errWrapper{err}
}

type errWrapper struct{ err error }

func (e errWrapper) Error() string { return "all handlers failed: " + e.err.Error() }
func (e errWrapper) Unwrap() error { return e.err }

// newTestProcessor builds a Processor wired to a real MemoryStore and proxy
// pool, with a single always-failing handler. Domain cache is nil (no DB),
// Gemini is disabled, and telemetry is a disabled no-op collector.
func newTestProcessor(t *testing.T, pool *proxy.Pool, handlerErr error) (*Processor, string) {
	t.Helper()

	st := store.NewMemoryStore()
	jobID := "test-job-" + t.Name()
	if err := st.CreateJob(context.Background(), store.JobRecord{
		ID: jobID, JobType: models.JobTypeURLScraper, URL: testURL,
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	chain := handler.NewChain([]handler.ScrapingHandler{&failingHandler{err: handlerErr}})
	tel := telemetry.New(nil, false, "", false, 0) // disabled: never touches the nil DB

	return NewProcessor(st, chain, nil, pool, gemini.NewClient(""), tel), jobID
}

func jobMessage(jobID string) models.JobMessage {
	return models.JobMessage{
		JobID:   jobID,
		URL:     testURL,
		JobType: models.JobTypeURLScraper,
	}
}

// scoredProxy returns the single proxy that has a recorded score for testHost.
// SelectProxy only creates persisted scores for the proxy it actually hands out,
// so after one job exactly one entry exists.
func scoredProxy(t *testing.T, pool *proxy.Pool) string {
	t.Helper()
	scores := pool.Scores()[testHost]
	if len(scores) != 1 {
		t.Fatalf("expected exactly 1 scored proxy after one job, got %d", len(scores))
	}
	return scores[0].ProxyURL
}

func scoreFor(t *testing.T, pool *proxy.Pool, proxyURL string) *proxy.Score {
	t.Helper()
	for _, sc := range pool.Scores()[testHost] {
		if sc.ProxyURL == proxyURL {
			return sc
		}
	}
	t.Fatalf("no score recorded for proxy %q on host %q", proxyURL, testHost)
	return nil
}

// failingHandler always fails with a fixed error.
type failingHandler struct{ err error }

func (h *failingHandler) Name() string { return "http" }
func (h *failingHandler) CanHandle(context.Context, *models.HandlerRequest) bool {
	return true
}
func (h *failingHandler) IsHealthy() bool { return true }
func (h *failingHandler) Scrape(context.Context, *models.HandlerRequest) (*models.ScrapeResult, error) {
	return nil, h.err
}
