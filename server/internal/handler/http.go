package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/models"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// HTTPHandler implements ScrapingHandler using direct HTTP requests.
type HTTPHandler struct {
	client  *http.Client
	timeout time.Duration
}

// NewHTTPHandler creates a new HTTP handler with optional proxy support.
func NewHTTPHandler(timeout time.Duration, proxyURL string) *HTTPHandler {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	}

	return &HTTPHandler{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		timeout: timeout,
	}
}

func (h *HTTPHandler) Name() string                                                   { return "http" }
func (h *HTTPHandler) CanHandle(_ context.Context, req *models.HandlerRequest) bool { return !req.UseBrowser }
func (h *HTTPHandler) IsHealthy() bool                                                { return true }

func (h *HTTPHandler) Scrape(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("User-Agent", defaultUserAgent)
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return &models.ScrapeResult{
		HTML:       string(body),
		StatusCode: resp.StatusCode,
	}, nil
}
