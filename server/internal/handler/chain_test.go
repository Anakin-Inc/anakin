package handler

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// mockHandler is a minimal ScrapingHandler for testing the chain logic.
type mockHandler struct {
	name      string
	canHandle bool
	healthy   bool
	result    *models.ScrapeResult
	err       error
	called    bool

	// Deadline observed on the context Execute passed to Scrape.
	gotDeadline   time.Time
	hadDeadline   bool
	blockUntilCtx bool // block until the context is done, then return its error
}

func (m *mockHandler) Name() string { return m.name }

func (m *mockHandler) CanHandle(_ context.Context, _ *models.HandlerRequest) bool {
	return m.canHandle
}

func (m *mockHandler) IsHealthy() bool { return m.healthy }

func (m *mockHandler) Scrape(ctx context.Context, _ *models.HandlerRequest) (*models.ScrapeResult, error) {
	m.called = true
	m.gotDeadline, m.hadDeadline = ctx.Deadline()
	if m.blockUntilCtx {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return m.result, m.err
}

// --- Tests ---

func TestChain_EmptyChainReturnsError(t *testing.T) {
	chain := NewChain(nil)
	_, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error from empty chain, got nil")
	}
	if err.Error() != "no handlers registered" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestChain_SingleHandlerSuccess(t *testing.T) {
	h := &mockHandler{
		name:      "test-handler",
		canHandle: true,
		healthy:   true,
		result:    &models.ScrapeResult{HTML: "<h1>hi</h1>", StatusCode: 200},
	}
	chain := NewChain([]ScrapingHandler{h})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Handler != "test-handler" {
		t.Errorf("expected Handler=test-handler, got %q", result.Handler)
	}
	if result.StatusCode != 200 {
		t.Errorf("expected StatusCode=200, got %d", result.StatusCode)
	}
}

func TestChain_FallbackOnFirstFailure(t *testing.T) {
	failing := &mockHandler{
		name:      "failing",
		canHandle: true,
		healthy:   true,
		err:       fmt.Errorf("connection refused"),
	}
	succeeding := &mockHandler{
		name:      "succeeding",
		canHandle: true,
		healthy:   true,
		result:    &models.ScrapeResult{HTML: "<p>ok</p>", StatusCode: 200},
	}
	chain := NewChain([]ScrapingHandler{failing, succeeding})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !failing.called {
		t.Error("expected failing handler to be called")
	}
	if !succeeding.called {
		t.Error("expected succeeding handler to be called")
	}
	if result.Handler != "succeeding" {
		t.Errorf("expected Handler=succeeding, got %q", result.Handler)
	}
}

func TestChain_AllowedHandlersFiltering(t *testing.T) {
	h1 := &mockHandler{name: "http", canHandle: true, healthy: true, result: &models.ScrapeResult{StatusCode: 200}}
	h2 := &mockHandler{name: "browser", canHandle: true, healthy: true, result: &models.ScrapeResult{StatusCode: 200}}

	chain := NewChain([]ScrapingHandler{h1, h2})
	req := &models.HandlerRequest{
		URL:             "https://example.com",
		AllowedHandlers: []string{"browser"},
	}
	result, err := chain.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h1.called {
		t.Error("http handler should NOT have been called (not in AllowedHandlers)")
	}
	if result.Handler != "browser" {
		t.Errorf("expected Handler=browser, got %q", result.Handler)
	}
}

func TestChain_UnhealthyHandlerSkipped(t *testing.T) {
	unhealthy := &mockHandler{
		name:      "unhealthy",
		canHandle: true,
		healthy:   false,
		result:    &models.ScrapeResult{StatusCode: 200},
	}
	healthy := &mockHandler{
		name:      "healthy",
		canHandle: true,
		healthy:   true,
		result:    &models.ScrapeResult{StatusCode: 200},
	}
	chain := NewChain([]ScrapingHandler{unhealthy, healthy})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if unhealthy.called {
		t.Error("unhealthy handler should NOT have been called")
	}
	if result.Handler != "healthy" {
		t.Errorf("expected Handler=healthy, got %q", result.Handler)
	}
}

func TestChain_CanHandleFalseSkipsHandler(t *testing.T) {
	cantHandle := &mockHandler{
		name:      "cant-handle",
		canHandle: false,
		healthy:   true,
		result:    &models.ScrapeResult{StatusCode: 200},
	}
	canHandle := &mockHandler{
		name:      "can-handle",
		canHandle: true,
		healthy:   true,
		result:    &models.ScrapeResult{StatusCode: 200},
	}
	chain := NewChain([]ScrapingHandler{cantHandle, canHandle})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cantHandle.called {
		t.Error("handler with CanHandle=false should NOT have been called")
	}
	if result.Handler != "can-handle" {
		t.Errorf("expected Handler=can-handle, got %q", result.Handler)
	}
}

func TestChain_AllHandlersFailReturnsError(t *testing.T) {
	h1 := &mockHandler{name: "a", canHandle: true, healthy: true, err: fmt.Errorf("fail a")}
	h2 := &mockHandler{name: "b", canHandle: true, healthy: true, err: fmt.Errorf("fail b")}

	chain := NewChain([]ScrapingHandler{h1, h2})
	_, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when all handlers fail, got nil")
	}
	if !h1.called || !h2.called {
		t.Error("both handlers should have been called")
	}
}

// req.Timeout is what a domain config's requestTimeoutMs becomes. Before this
// was wired up, handlers received the bare job context and the setting did nothing.
func TestChain_RequestTimeoutBoundsTheAttempt(t *testing.T) {
	h := &mockHandler{
		name: "http", canHandle: true, healthy: true,
		blockUntilCtx: true,
	}
	chain := NewChain([]ScrapingHandler{h})

	start := time.Now()
	_, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:     "https://example.com",
		Timeout: 50 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the attempt to time out, got nil error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected a DeadlineExceeded error, got %v", err)
	}
	if elapsed > time.Second {
		t.Errorf("attempt ran for %v, the 50ms timeout was not applied", elapsed)
	}
}

// "Timeout per handler attempt" — each handler gets its own budget, so a slow
// first handler must not eat the fallback's.
func TestChain_TimeoutIsPerHandlerNotShared(t *testing.T) {
	slow := &mockHandler{name: "http", canHandle: true, healthy: true, blockUntilCtx: true}
	fallback := &mockHandler{
		name: "browser", canHandle: true, healthy: true,
		result: &models.ScrapeResult{StatusCode: 200},
	}

	chain := NewChain([]ScrapingHandler{slow, fallback})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:     "https://example.com",
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("fallback should still have run after the first attempt timed out: %v", err)
	}
	if result.Handler != "browser" {
		t.Errorf("expected Handler=browser, got %q", result.Handler)
	}

	if !fallback.hadDeadline {
		t.Fatal("fallback attempt had no deadline")
	}
	if remaining := time.Until(fallback.gotDeadline); remaining <= 0 {
		t.Errorf("fallback inherited an already-expired deadline (%v remaining)", remaining)
	}
}

func TestChain_ZeroTimeoutLeavesContextUnbounded(t *testing.T) {
	h := &mockHandler{
		name: "http", canHandle: true, healthy: true,
		result: &models.ScrapeResult{StatusCode: 200},
	}
	chain := NewChain([]ScrapingHandler{h})

	if _, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.hadDeadline {
		t.Error("a zero Timeout should not impose a deadline")
	}
}

// A per-attempt timeout must never extend the job's own deadline.
func TestChain_JobDeadlineStillWinsWhenShorter(t *testing.T) {
	h := &mockHandler{name: "http", canHandle: true, healthy: true, blockUntilCtx: true}
	chain := NewChain([]ScrapingHandler{h})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := chain.Execute(ctx, &models.HandlerRequest{
		URL:     "https://example.com",
		Timeout: time.Hour,
	}); err == nil {
		t.Fatal("expected an error once the job context expired")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("ran for %v — the job deadline was overridden by the longer per-attempt timeout", elapsed)
	}
}

func TestChain_HandlerNames(t *testing.T) {
	h1 := &mockHandler{name: "alpha"}
	h2 := &mockHandler{name: "bravo"}
	chain := NewChain([]ScrapingHandler{h1, h2})
	names := chain.HandlerNames()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "bravo" {
		t.Errorf("HandlerNames() = %v, want [alpha bravo]", names)
	}
}
