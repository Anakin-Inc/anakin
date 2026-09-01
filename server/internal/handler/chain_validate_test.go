// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// countingHandler records how many times it was scraped, so a test can tell the
// difference between "the chain moved on" and "the chain re-ran the same handler".
type countingHandler struct {
	name  string
	html  string
	err   error
	calls int
}

func (h *countingHandler) Name() string                                           { return h.name }
func (h *countingHandler) CanHandle(context.Context, *models.HandlerRequest) bool { return true }
func (h *countingHandler) IsHealthy() bool                                        { return true }

func (h *countingHandler) Scrape(context.Context, *models.HandlerRequest) (*models.ScrapeResult, error) {
	h.calls++
	if h.err != nil {
		return nil, h.err
	}
	return &models.ScrapeResult{HTML: h.html, StatusCode: 200}, nil
}

// rejectHTML builds a validator that rejects any result whose HTML contains marker.
func rejectHTML(marker string) func(*models.ScrapeResult) error {
	return func(r *models.ScrapeResult) error {
		if strings.Contains(r.HTML, marker) {
			return errors.New("content validation: failure pattern matched: " + marker)
		}
		return nil
	}
}

func TestChain_RejectedResultFallsThroughToNextHandler(t *testing.T) {
	httpHandler := &countingHandler{name: "http", html: "<html>captcha, please verify you are a human</html>"}
	browserHandler := &countingHandler{name: "browser", html: "<html>the real page</html>"}
	chain := NewChain([]ScrapingHandler{httpHandler, browserHandler})

	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:      "https://example.com",
		Validate: rejectHTML("captcha"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Handler != "browser" {
		t.Errorf("result came from %q, want %q — a rejected page must not be returned as the result", result.Handler, "browser")
	}
	if browserHandler.calls != 1 {
		t.Errorf("browser handler called %d times, want 1 — failure detection must reach the next handler in the chain", browserHandler.calls)
	}
	if httpHandler.calls != 1 {
		t.Errorf("http handler called %d times, want 1", httpHandler.calls)
	}
}

func TestChain_ValidatorRejectingEveryHandlerReturnsTheValidationError(t *testing.T) {
	first := &countingHandler{name: "http", html: "<html>captcha</html>"}
	second := &countingHandler{name: "browser", html: "<html>captcha</html>"}
	chain := NewChain([]ScrapingHandler{first, second})

	_, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:      "https://example.com",
		Validate: rejectHTML("captcha"),
	})
	if err == nil {
		t.Fatal("expected an error when every handler's result is rejected")
	}
	if !strings.Contains(err.Error(), "content validation") {
		t.Errorf("error = %q, want it to carry the validation reason so the job records why it failed", err)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Errorf("handler calls = (%d, %d), want (1, 1) — every handler should be tried once", first.calls, second.calls)
	}
}

func TestChain_AcceptedResultStopsAtTheFirstHandler(t *testing.T) {
	first := &countingHandler{name: "http", html: "<html>the real page</html>"}
	second := &countingHandler{name: "browser", html: "<html>also fine</html>"}
	chain := NewChain([]ScrapingHandler{first, second})

	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:      "https://example.com",
		Validate: rejectHTML("captcha"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Handler != "http" {
		t.Errorf("result came from %q, want %q", result.Handler, "http")
	}
	if second.calls != 0 {
		t.Errorf("browser handler called %d times, want 0 — a valid result must not trigger fallback", second.calls)
	}
}

func TestChain_ValidatorNotConsultedWhenHandlerErrors(t *testing.T) {
	failing := &countingHandler{name: "http", err: errors.New("connection refused")}
	good := &countingHandler{name: "browser", html: "<html>the real page</html>"}
	chain := NewChain([]ScrapingHandler{failing, good})

	validated := 0
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL: "https://example.com",
		Validate: func(*models.ScrapeResult) error {
			validated++
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Handler != "browser" {
		t.Errorf("result came from %q, want %q", result.Handler, "browser")
	}
	if validated != 1 {
		t.Errorf("validator ran %d times, want 1 — it must only see results, not failed attempts", validated)
	}
}

func TestChain_NilValidatorLeavesBehaviourUnchanged(t *testing.T) {
	first := &countingHandler{name: "http", html: "<html>captcha</html>"}
	second := &countingHandler{name: "browser", html: "<html>the real page</html>"}
	chain := NewChain([]ScrapingHandler{first, second})

	result, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Handler != "http" {
		t.Errorf("result came from %q, want %q — without a validator the first success wins", result.Handler, "http")
	}
	if second.calls != 0 {
		t.Errorf("browser handler called %d times, want 0", second.calls)
	}
}
