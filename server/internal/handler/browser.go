// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/playwright-community/playwright-go"
)

const (
	// How long a reachability probe result is trusted before re-dialing.
	probeTTL = 10 * time.Second
	// Dial budget for a single probe.
	probeTimeout = 2 * time.Second
)

// BrowserHandler implements ScrapingHandler using Playwright browser automation.
type BrowserHandler struct {
	wsURL    string
	timeout  time.Duration
	loadWait time.Duration
	pw       *playwright.Playwright
	once     sync.Once
	initErr  error

	probeMu   sync.Mutex
	probedAt  time.Time
	reachable bool
}

func NewBrowserHandler(wsURL string, timeout, loadWait time.Duration) *BrowserHandler {
	return &BrowserHandler{
		wsURL:    wsURL,
		timeout:  timeout,
		loadWait: loadWait,
	}
}

func (h *BrowserHandler) Name() string                                               { return "browser" }
func (h *BrowserHandler) CanHandle(_ context.Context, _ *models.HandlerRequest) bool { return true }

// IsHealthy reports whether the browser endpoint can actually serve a scrape.
// The endpoint is probed first: when no browser service is running there is no
// point starting the local Playwright driver, and letting Scrape run anyway
// would bury the real error from an earlier handler behind a connection error.
func (h *BrowserHandler) IsHealthy() bool {
	if !h.endpointReachable() {
		return false
	}
	return h.ensurePlaywright() == nil
}

// endpointReachable TCP-dials the websocket endpoint, caching the outcome for
// probeTTL so the chain does not dial once per request.
// ponytail: a successful dial only proves something is listening, not that it
// speaks the CDP/Camoufox protocol. Upgrade to a real handshake if half-open
// browser services turn out to be a problem in practice.
func (h *BrowserHandler) endpointReachable() bool {
	h.probeMu.Lock()
	defer h.probeMu.Unlock()

	if !h.probedAt.IsZero() && time.Since(h.probedAt) < probeTTL {
		return h.reachable
	}
	h.probedAt = time.Now()
	h.reachable = false

	addr, err := wsDialAddr(h.wsURL)
	if err != nil {
		slog.Warn("browser endpoint unusable", "url", h.wsURL, "error", err)
		return false
	}
	conn, err := net.DialTimeout("tcp", addr, probeTimeout)
	if err != nil {
		slog.Debug("browser endpoint unreachable", "addr", addr, "error", err)
		return false
	}
	if closeErr := conn.Close(); closeErr != nil {
		slog.Debug("failed to close probe connection", "error", closeErr)
	}
	h.reachable = true
	return true
}

// wsDialAddr converts a ws:// or wss:// URL into a host:port dial target.
func wsDialAddr(wsURL string) (string, error) {
	u, err := url.Parse(wsURL)
	if err != nil {
		return "", fmt.Errorf("invalid browser URL %q: %w", wsURL, err)
	}
	if u.Hostname() == "" {
		return "", fmt.Errorf("browser URL %q has no host", wsURL)
	}
	if u.Port() != "" {
		return u.Host, nil
	}
	if u.Scheme == "wss" || u.Scheme == "https" {
		return net.JoinHostPort(u.Hostname(), "443"), nil
	}
	return net.JoinHostPort(u.Hostname(), "80"), nil
}

func (h *BrowserHandler) ensurePlaywright() error {
	h.once.Do(func() {
		pw, err := playwright.Run()
		if err != nil {
			h.initErr = fmt.Errorf("failed to start playwright: %w", err)
			return
		}
		h.pw = pw
	})
	return h.initErr
}

func (h *BrowserHandler) Scrape(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	if err := h.ensurePlaywright(); err != nil {
		return nil, err
	}

	browser, err := h.pw.Chromium.Connect(h.wsURL, playwright.BrowserTypeConnectOptions{
		Timeout: playwright.Float(float64(h.timeout.Milliseconds())),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to browser at %s: %w", h.wsURL, err)
	}
	defer func() {
		if disconnectErr := browser.Close(); disconnectErr != nil {
			slog.Warn("failed to close browser connection", "error", disconnectErr)
		}
	}()

	browserCtx, err := browser.NewContext()
	if err != nil {
		return nil, fmt.Errorf("failed to create browser context: %w", err)
	}
	defer func() {
		if closeErr := browserCtx.Close(); closeErr != nil {
			slog.Warn("failed to close browser context", "error", closeErr)
		}
	}()

	page, err := browserCtx.NewPage()
	if err != nil {
		return nil, fmt.Errorf("failed to create page: %w", err)
	}

	timeout := h.timeout.Milliseconds()
	_, err = page.Goto(req.URL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
		Timeout:   playwright.Float(float64(timeout)),
	})
	if err != nil {
		return nil, fmt.Errorf("navigation failed: %w", err)
	}

	if h.loadWait > 0 {
		select {
		case <-time.After(h.loadWait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	html, err := page.Content()
	if err != nil {
		return nil, fmt.Errorf("failed to get page content: %w", err)
	}

	return &models.ScrapeResult{
		HTML:       html,
		StatusCode: 200,
	}, nil
}

// Stop cleans up the Playwright driver.
func (h *BrowserHandler) Stop() {
	if h.pw != nil {
		if err := h.pw.Stop(); err != nil {
			slog.Warn("failed to stop playwright", "error", err)
		}
	}
}
