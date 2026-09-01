// SPDX-License-Identifier: AGPL-3.0-or-later

package processor

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/domain"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/handler"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

// --- a stub driver serving one domain_configs row, so domain.Cache can be
// populated the same way it is in production (repository -> refresh -> cache). ---

type domainConfigDriver struct{}

func (domainConfigDriver) Open(string) (driver.Conn, error) { return domainConfigConn{}, nil }

type domainConfigConn struct{}

func (domainConfigConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (domainConfigConn) Close() error                        { return nil }
func (domainConfigConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (domainConfigConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &domainConfigRows{}, nil
}

type domainConfigRows struct{ done bool }

func (*domainConfigRows) Columns() []string {
	return []string{
		"id", "domain", "is_enabled", "match_subdomains", "priority",
		"handler_chain", "request_timeout_ms", "max_retries",
		"min_content_length", "failure_patterns", "required_patterns",
		"custom_headers", "custom_user_agent", "proxy_url",
		"blocked", "blocked_reason", "notes", "created_at", "updated_at",
	}
}

func (*domainConfigRows) Close() error { return nil }

func (r *domainConfigRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	now := time.Now().UTC()
	copy(dest, []driver.Value{
		int64(1), "example.com", true, true, int64(0),
		"http,browser", int64(30000), int64(2),
		int64(0), "captcha", "",
		"{}", nil, nil,
		false, nil, nil, now, now,
	})
	return nil
}

func domainCacheWithCaptchaRule(t *testing.T, ctx context.Context) *domain.Cache {
	t.Helper()
	driverName := "domain_config_stub_" + t.Name()
	sql.Register(driverName, domainConfigDriver{})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open stub db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cache := domain.NewCache(domain.NewRepository(db))
	cache.Start(ctx)
	return cache
}

// --- a store that just records the terminal state ---

type recordingStore struct {
	mu     sync.Mutex
	status string
	errMsg string
}

func (s *recordingStore) UpdateStatus(_ context.Context, _, status string, errMsg *string, _ *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
	if errMsg != nil {
		s.errMsg = *errMsg
	}
	return nil
}

func (s *recordingStore) UpdateCompleted(context.Context, string, int, int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = models.JobStatusCompleted
	return nil
}

func (s *recordingStore) state() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.errMsg
}

func (*recordingStore) StoreResult(context.Context, string, string) error        { return nil }
func (*recordingStore) UpdateParentBatchStatus(context.Context, string) error    { return nil }
func (*recordingStore) CreateJob(context.Context, store.JobRecord) error         { return nil }
func (*recordingStore) GetJob(context.Context, string) (*store.JobRecord, error) { return nil, nil }
func (*recordingStore) GetChildJobs(context.Context, string) ([]store.JobRecord, error) {
	return nil, nil
}
func (*recordingStore) Ping(context.Context) error { return nil }

// --- handlers ---

type scriptedHandler struct {
	name  string
	html  string
	calls int
}

func (h *scriptedHandler) Name() string                                           { return h.name }
func (h *scriptedHandler) CanHandle(context.Context, *models.HandlerRequest) bool { return true }
func (h *scriptedHandler) IsHealthy() bool                                        { return true }

func (h *scriptedHandler) Scrape(context.Context, *models.HandlerRequest) (*models.ScrapeResult, error) {
	h.calls++
	return &models.ScrapeResult{HTML: h.html, StatusCode: 200}, nil
}

// A domain config with failurePatterns ["captcha"] must make a job that hits a CAPTCHA
// on the http handler fall through to the browser handler — the behaviour README.md and
// docs/domain-configs.md both describe. Before this fix the processor re-ran the whole
// chain, so the http handler answered every attempt with the same CAPTCHA page and the
// browser handler was never reached.
func TestProcessJob_FailureDetectionFallsBackToTheNextHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpHandler := &scriptedHandler{name: "http", html: "<html>captcha: please verify you are a human</html>"}
	browserHandler := &scriptedHandler{name: "browser", html: "<html>the real product page</html>"}
	chain := handler.NewChain([]handler.ScrapingHandler{httpHandler, browserHandler})

	st := &recordingStore{}
	p := NewProcessor(st, chain, domainCacheWithCaptchaRule(t, ctx), nil, nil, nil)

	err := p.ProcessJob(ctx, models.JobMessage{
		JobID:   "job-1",
		URL:     "https://example.com/product",
		JobType: models.JobTypeURLScraper,
	})
	if err != nil {
		t.Fatalf("ProcessJob returned %v, want success via the browser handler", err)
	}

	if browserHandler.calls == 0 {
		t.Fatalf("browser handler was never tried; http handler ran %d times returning the same CAPTCHA page", httpHandler.calls)
	}
	if httpHandler.calls != 1 {
		t.Errorf("http handler called %d times, want 1 — a handler whose page was rejected must not be retried", httpHandler.calls)
	}

	if status, _ := st.state(); status != models.JobStatusCompleted {
		t.Errorf("job status = %q, want %q", status, models.JobStatusCompleted)
	}
}

// When no handler can produce acceptable content the job still fails, and the reason the
// detector gave has to survive into the recorded error.
func TestProcessJob_FailureDetectionFailsJobWhenEveryHandlerIsRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpHandler := &scriptedHandler{name: "http", html: "<html>captcha</html>"}
	browserHandler := &scriptedHandler{name: "browser", html: "<html>captcha</html>"}
	chain := handler.NewChain([]handler.ScrapingHandler{httpHandler, browserHandler})

	st := &recordingStore{}
	p := NewProcessor(st, chain, domainCacheWithCaptchaRule(t, ctx), nil, nil, nil)

	err := p.ProcessJob(ctx, models.JobMessage{
		JobID:   "job-2",
		URL:     "https://example.com/product",
		JobType: models.JobTypeURLScraper,
	})
	if err == nil {
		t.Fatal("expected the job to fail when every handler returns a CAPTCHA page")
	}

	if browserHandler.calls == 0 {
		t.Error("browser handler was never tried")
	}

	status, errMsg := st.state()
	if status != models.JobStatusFailed {
		t.Errorf("job status = %q, want %q", status, models.JobStatusFailed)
	}
	if !strings.Contains(errMsg, "content validation") {
		t.Errorf("recorded error = %q, want it to name the failed content validation", errMsg)
	}
}
