// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/handler"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// The example is the documented starting point for a third-party handler, so it has to
// keep satisfying the interface it is teaching.
var _ handler.ScrapingHandler = (*CachedHTMLHandler)(nil)

func TestCachedHTMLHandlerFilePath(t *testing.T) {
	h := NewCachedHTMLHandler("cache")

	for _, tc := range []struct {
		name   string
		rawURL string
		want   string
	}{
		{"https URL with path", "https://example.com/page", "example.com.html"},
		{"http URL without path", "http://example.com", "example.com.html"},
		{"bare host", "example.com", "example.com.html"},
		{"host with port", "https://example.com:8443/page", "example.com:8443.html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.filePath(tc.rawURL); got != filepath.Join("cache", tc.want) {
				t.Errorf("filePath(%q) = %q, want %q", tc.rawURL, got, filepath.Join("cache", tc.want))
			}
		})
	}
}

func TestCachedHTMLHandlerServesCachedPage(t *testing.T) {
	dir := t.TempDir()
	body := "<html><body>Cached!</body></html>"
	if err := os.WriteFile(filepath.Join(dir, "example.com.html"), []byte(body), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	h := NewCachedHTMLHandler(dir)
	req := &models.HandlerRequest{URL: "https://example.com/page"}

	if !h.CanHandle(context.Background(), req) {
		t.Fatal("CanHandle = false, want true for a URL with a cached file")
	}

	result, err := h.Scrape(context.Background(), req)
	if err != nil {
		t.Fatalf("Scrape returned %v", err)
	}
	if result.HTML != body {
		t.Errorf("HTML = %q, want %q", result.HTML, body)
	}
	if !result.Cached {
		t.Error("Cached = false, want true — a page served from disk is cached")
	}
	if result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", result.StatusCode)
	}
}

func TestCachedHTMLHandlerDeclinesUncachedURL(t *testing.T) {
	h := NewCachedHTMLHandler(t.TempDir())

	if h.CanHandle(context.Background(), &models.HandlerRequest{URL: "https://example.com"}) {
		t.Error("CanHandle = true for an uncached URL; the chain would stop instead of falling through to http")
	}
}

func TestCachedHTMLHandlerIsUnhealthyWithoutCacheDir(t *testing.T) {
	if NewCachedHTMLHandler(filepath.Join(t.TempDir(), "missing")).IsHealthy() {
		t.Error("IsHealthy = true for a missing cache directory, want false")
	}
	if !NewCachedHTMLHandler(t.TempDir()).IsHealthy() {
		t.Error("IsHealthy = false for an existing cache directory, want true")
	}
}

// The chain must skip the custom handler when it declines and reach the next one.
func TestExampleChainFallsThroughToTheNextHandler(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cached.example.html"), []byte("<html>cached</html>"), 0o600); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	fallback := &stubHandler{name: "http", html: "<html>fetched</html>"}
	chain := handler.NewChain([]handler.ScrapingHandler{NewCachedHTMLHandler(dir), fallback})

	result, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://uncached.example/page"})
	if err != nil {
		t.Fatalf("Execute returned %v", err)
	}
	if result.Handler != "http" {
		t.Errorf("result came from %q, want %q", result.Handler, "http")
	}
}

type stubHandler struct {
	name string
	html string
}

func (h *stubHandler) Name() string                                           { return h.name }
func (h *stubHandler) CanHandle(context.Context, *models.HandlerRequest) bool { return true }
func (h *stubHandler) IsHealthy() bool                                        { return true }
func (h *stubHandler) Scrape(context.Context, *models.HandlerRequest) (*models.ScrapeResult, error) {
	return &models.ScrapeResult{HTML: h.html, StatusCode: 200}, nil
}
