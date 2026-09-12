package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// recordingHandler appends its name to a shared log every time the chain runs
// it, so a test can assert the order handlers were tried in and not merely
// which one won.
type recordingHandler struct {
	name string
	log  *[]string
	err  error
}

func (r *recordingHandler) Name() string { return r.name }

func (r *recordingHandler) CanHandle(_ context.Context, _ *models.HandlerRequest) bool { return true }

func (r *recordingHandler) IsHealthy() bool { return true }

func (r *recordingHandler) Scrape(_ context.Context, _ *models.HandlerRequest) (*models.ScrapeResult, error) {
	*r.log = append(*r.log, r.name)
	if r.err != nil {
		return nil, r.err
	}
	return &models.ScrapeResult{HTML: "<p>" + r.name + "</p>", StatusCode: 200}, nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A domain config's handlerChain is documented as "which handlers to try, in
// order", so ["browser", "http"] has to reach the browser first even though the
// HTTP handler is registered first in main.go.
func TestChain_AllowedHandlersDecideOrder(t *testing.T) {
	var log []string
	httpH := &recordingHandler{name: "http", log: &log, err: fmt.Errorf("HTTP 403")}
	browserH := &recordingHandler{name: "browser", log: &log}

	chain := NewChain([]ScrapingHandler{httpH, browserH})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:             "https://example.com",
		AllowedHandlers: []string{"browser", "http"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"browser"}; !equalStrings(log, want) {
		t.Errorf("handlers tried in order %v, want %v", log, want)
	}
	if result.Handler != "browser" {
		t.Errorf("expected Handler=browser, got %q", result.Handler)
	}
}

// The configured order also has to survive a fallback: browser first, then the
// HTTP handler, never the registration order.
func TestChain_AllowedHandlersOrderSurvivesFallback(t *testing.T) {
	var log []string
	httpH := &recordingHandler{name: "http", log: &log}
	browserH := &recordingHandler{name: "browser", log: &log, err: fmt.Errorf("browser unreachable")}

	chain := NewChain([]ScrapingHandler{httpH, browserH})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:             "https://example.com",
		AllowedHandlers: []string{"browser", "http"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"browser", "http"}; !equalStrings(log, want) {
		t.Errorf("handlers tried in order %v, want %v", log, want)
	}
	if result.Handler != "http" {
		t.Errorf("expected Handler=http, got %q", result.Handler)
	}
}

// With no allow-list the registration order from main.go still decides.
func TestChain_NoAllowedHandlersKeepsRegistrationOrder(t *testing.T) {
	var log []string
	httpH := &recordingHandler{name: "http", log: &log, err: fmt.Errorf("HTTP 403")}
	browserH := &recordingHandler{name: "browser", log: &log}

	chain := NewChain([]ScrapingHandler{httpH, browserH})
	if _, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL: "https://example.com",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"http", "browser"}; !equalStrings(log, want) {
		t.Errorf("handlers tried in order %v, want %v", log, want)
	}
}

// A config can name a handler this instance never registered — "anakin" without
// ANAKIN_API_KEY — and that must not stop the handlers that do exist, or be
// tried twice when the config repeats it.
func TestChain_AllowedHandlersSkipsUnknownAndDuplicates(t *testing.T) {
	var log []string
	httpH := &recordingHandler{name: "http", log: &log}
	browserH := &recordingHandler{name: "browser", log: &log, err: fmt.Errorf("browser unreachable")}

	chain := NewChain([]ScrapingHandler{httpH, browserH})
	result, err := chain.Execute(context.Background(), &models.HandlerRequest{
		URL:             "https://example.com",
		AllowedHandlers: []string{"anakin", "browser", "browser", "http"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"browser", "http"}; !equalStrings(log, want) {
		t.Errorf("handlers tried in order %v, want %v", log, want)
	}
	if result.Handler != "http" {
		t.Errorf("expected Handler=http, got %q", result.Handler)
	}
}
