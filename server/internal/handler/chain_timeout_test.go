// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// A domain config's requestTimeoutMs arrives as HandlerRequest.Timeout and must
// bound the individual handler attempt. It previously bounded nothing — no
// handler read the field.
func TestChain_AppliesPerAttemptTimeout(t *testing.T) {
	h := &ctxProbeHandler{result: &models.ScrapeResult{HTML: "<html></html>", StatusCode: 200}}
	chain := NewChain([]ScrapingHandler{h})

	_, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:     "https://example.com",
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !h.hadDeadline {
		t.Fatal("handler received a context with no deadline; requestTimeoutMs was not applied")
	}
	// Allow generous slack for scheduling; the point is that a deadline near the
	// requested value exists at all.
	if h.remaining <= 0 || h.remaining > 5*time.Second {
		t.Errorf("deadline left %v, want something in (0, 5s]", h.remaining)
	}
}

// Zero means "no override", which is the case for every request that has no
// domain config. Wrapping unconditionally would have silently capped every
// attempt in the tree.
func TestChain_ZeroTimeoutLeavesContextUnbounded(t *testing.T) {
	h := &ctxProbeHandler{result: &models.ScrapeResult{HTML: "<html></html>", StatusCode: 200}}
	chain := NewChain([]ScrapingHandler{h})

	if _, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:     "https://example.com",
		Timeout: 0,
	}); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if h.hadDeadline {
		t.Error("handler received a deadline for a request with Timeout == 0")
	}
}

// The deadline has to actually cut a slow handler off, not merely be present.
func TestChain_PerAttemptTimeoutAbortsSlowHandler(t *testing.T) {
	chain := NewChain([]ScrapingHandler{&blockingHandler{}})

	start := time.Now()
	_, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:     "https://example.com",
		Timeout: 100 * time.Millisecond,
	})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Execute returned nil error; the slow handler should have been cut off")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("took %v; the per-attempt deadline did not abort the handler", elapsed)
	}
}

// An expired attempt must not consume the parent context: the next handler in
// the chain still gets its turn.
func TestChain_TimeoutOnOneHandlerStillFallsBack(t *testing.T) {
	fallback := &mockHandler{
		name: "browser", canHandle: true, healthy: true,
		result: &models.ScrapeResult{HTML: "<html>ok</html>", StatusCode: 200},
	}
	chain := NewChain([]ScrapingHandler{&blockingHandler{}, fallback})

	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:     "https://example.com",
		Timeout: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !fallback.called {
		t.Error("fallback handler was never tried after the first handler timed out")
	}
	if result.Handler != "browser" {
		t.Errorf("result.Handler = %q, want %q", result.Handler, "browser")
	}
}

// ctxProbeHandler records what deadline, if any, its context carried.
type ctxProbeHandler struct {
	result      *models.ScrapeResult
	hadDeadline bool
	remaining   time.Duration
}

func (h *ctxProbeHandler) Name() string                                           { return "probe" }
func (h *ctxProbeHandler) CanHandle(context.Context, *models.HandlerRequest) bool { return true }
func (h *ctxProbeHandler) IsHealthy() bool                                        { return true }
func (h *ctxProbeHandler) Scrape(ctx context.Context, _ *models.HandlerRequest) (*models.ScrapeResult, error) {
	if dl, ok := ctx.Deadline(); ok {
		h.hadDeadline = true
		h.remaining = time.Until(dl)
	}
	return h.result, nil
}

// blockingHandler waits until its context is cancelled, standing in for a
// handler talking to an unresponsive target.
type blockingHandler struct{}

func (h *blockingHandler) Name() string                                           { return "slow" }
func (h *blockingHandler) CanHandle(context.Context, *models.HandlerRequest) bool { return true }
func (h *blockingHandler) IsHealthy() bool                                        { return true }
func (h *blockingHandler) Scrape(ctx context.Context, _ *models.HandlerRequest) (*models.ScrapeResult, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, errors.New("handler was not cut off by the per-attempt deadline")
	}
}
