// Custom handler example — demonstrates extending the AnakinScraper handler chain.
//
// This example adds a "cached-html" handler that serves pre-cached HTML from a
// local directory, falling back to the built-in HTTP handler for any URL that
// isn't cached locally.
//
// To run this example:
//
//	cd server && go run ../examples/custom-handler/main.go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/handler"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/models"
)

// -----------------------------------------------------------------------
// Step 1: Define your custom handler by implementing handler.ScrapingHandler
// -----------------------------------------------------------------------

// CachedHTMLHandler serves pages from a local directory of pre-fetched HTML
// files. File names are derived from the URL hostname (e.g. "example.com.html").
type CachedHTMLHandler struct {
	cacheDir string
}

func NewCachedHTMLHandler(cacheDir string) *CachedHTMLHandler {
	return &CachedHTMLHandler{cacheDir: cacheDir}
}

func (h *CachedHTMLHandler) Name() string {
	return "cached-html"
}

// CanHandle returns true if a cached file exists for the request URL's host.
func (h *CachedHTMLHandler) CanHandle(_ context.Context, req *models.HandlerRequest) bool {
	_, err := os.Stat(h.filePath(req.URL))
	return err == nil
}

func (h *CachedHTMLHandler) IsHealthy() bool {
	info, err := os.Stat(h.cacheDir)
	return err == nil && info.IsDir()
}

func (h *CachedHTMLHandler) Scrape(_ context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	data, err := os.ReadFile(h.filePath(req.URL))
	if err != nil {
		return nil, fmt.Errorf("failed to read cached file: %w", err)
	}
	return &models.ScrapeResult{
		HTML:       string(data),
		StatusCode: 200,
		Cached:     true,
	}, nil
}

// filePath turns "https://example.com/page" into "<cacheDir>/example.com.html".
func (h *CachedHTMLHandler) filePath(rawURL string) string {
	host := rawURL
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	return filepath.Join(h.cacheDir, host+".html")
}

// -----------------------------------------------------------------------
// Step 2: Register your handler in the chain
// -----------------------------------------------------------------------

func main() {
	// Your custom handler — checked first
	cachedHandler := NewCachedHTMLHandler("./cached-pages")

	// Built-in HTTP handler as fallback
	httpHandler := handler.NewHTTPHandler(30*time.Second, "")

	// Build the chain: cached-html → http → (browser)
	chain := handler.NewChain([]handler.ScrapingHandler{
		cachedHandler,
		httpHandler,
	})

	slog.Info("handler chain ready", "handlers", chain.HandlerNames())

	// Try a request
	ctx := context.Background()
	url := "https://example.com"
	if len(os.Args) > 1 {
		url = os.Args[1]
	}

	result, err := chain.Execute(ctx, &models.HandlerRequest{
		URL: url,
	})
	if err != nil {
		slog.Error("scrape failed", "url", url, "error", err)
		os.Exit(1)
	}

	fmt.Printf("Handler: %s\n", result.Handler)
	fmt.Printf("Status:  %d\n", result.StatusCode)
	fmt.Printf("Cached:  %v\n", result.Cached)
	fmt.Printf("HTML:    %d bytes\n", len(result.HTML))
}
