// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/netguard"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

// HTTPHandler implements ScrapingHandler using direct HTTP requests.
type HTTPHandler struct {
	client  *http.Client
	timeout time.Duration

	// Clients for per-request proxies, keyed by proxy URL. Each carries its own
	// connection pool, so it has to outlive the request that created it.
	mu           sync.Mutex
	proxyClients map[string]*http.Client
}

// NewHTTPHandler creates a new HTTP handler with optional proxy support.
// Unless allowPrivateTargets is set, direct connections to internal addresses are
// refused at dial time, which also covers redirects and DNS rebinding.
func NewHTTPHandler(timeout time.Duration, proxyURL string, allowPrivateTargets bool) *HTTPHandler {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	if proxyURL != "" {
		if parsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(parsed)
		}
	} else if !allowPrivateTargets {
		// Only guard the dialer when we dial the target ourselves. Behind a proxy the
		// dial target is the proxy, which is commonly a private address.
		transport.DialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			Control:   netguard.DialControl,
		}).DialContext
	}

	return &HTTPHandler{
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
		timeout:      timeout,
		proxyClients: make(map[string]*http.Client),
	}
}

// proxyClient returns the client for proxyURL, building and caching one on first use.
//
// The cache is what makes proxied scraping reusable: an http.Transport owns a
// connection pool, so a transport built per request can never reuse a connection —
// every scrape pays a fresh TCP (and, for HTTPS targets, TLS) handshake, and the
// transport it belonged to is dropped without CloseIdleConnections, leaving that
// connection and its read/write goroutines alive until IdleConnTimeout expires.
//
// Keys come from the operator-configured proxy pool and domain configs, so the map
// stays as small as the proxy list.
func (h *HTTPHandler) proxyClient(proxyURL string) (*http.Client, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.proxyClients == nil {
		h.proxyClients = make(map[string]*http.Client)
	}
	if client, ok := h.proxyClients[proxyURL]; ok {
		return client, nil
	}

	parsed, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL %q: %w", proxyURL, err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyURL(parsed),
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: h.timeout,
	}
	h.proxyClients[proxyURL] = client
	return client, nil
}

func (h *HTTPHandler) Name() string { return "http" }
func (h *HTTPHandler) CanHandle(_ context.Context, req *models.HandlerRequest) bool {
	return !req.UseBrowser
}
func (h *HTTPHandler) IsHealthy() bool { return true }

func (h *HTTPHandler) Scrape(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// User-agent: per-request override > default
	ua := defaultUserAgent
	if req.CustomUserAgent != "" {
		ua = req.CustomUserAgent
	}
	httpReq.Header.Set("User-Agent", ua)
	httpReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	httpReq.Header.Set("Accept-Language", "en-US,en;q=0.9")

	// Apply custom headers
	for k, v := range req.CustomHeaders {
		httpReq.Header.Set(k, v)
	}

	// Use per-request proxy if specified, otherwise the default client
	client := h.client
	if req.ProxyURL != "" {
		// An unusable proxy URL must fail here. Falling through to h.client would
		// silently scrape from the server's own address, bypassing both the proxy
		// the caller asked for and, when one is configured, the dialer guard.
		proxied, proxyErr := h.proxyClient(req.ProxyURL)
		if proxyErr != nil {
			return nil, proxyErr
		}
		client = proxied
	}

	resp, err := client.Do(httpReq)
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
