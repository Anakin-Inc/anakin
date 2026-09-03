// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/playwright-community/playwright-go"
)

// BrowserHandler implements ScrapingHandler using Playwright browser automation.
type BrowserHandler struct {
	wsURL    string
	timeout  time.Duration
	loadWait time.Duration
	pw       *playwright.Playwright
	once     sync.Once
	initErr  error
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

func (h *BrowserHandler) IsHealthy() bool {
	h.ensurePlaywright()
	return h.initErr == nil
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

	browserCtx, err := browser.NewContext(contextOptions(req))
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

// contextOptions maps the per-request overrides a domain config can carry onto the
// browser context. Unlike the HTTP handler, this handler does not build the outgoing
// request itself — Camoufox does — so the context is the only place a custom
// user-agent or custom headers can be applied.
func contextOptions(req *models.HandlerRequest) playwright.BrowserNewContextOptions {
	var opts playwright.BrowserNewContextOptions
	if req.CustomUserAgent != "" {
		opts.UserAgent = playwright.String(req.CustomUserAgent)
	}
	if len(req.CustomHeaders) > 0 {
		headers := make(map[string]string, len(req.CustomHeaders))
		for k, v := range req.CustomHeaders {
			headers[k] = v
		}
		opts.ExtraHttpHeaders = headers
	}
	return opts
}

// Stop cleans up the Playwright driver.
func (h *BrowserHandler) Stop() {
	if h.pw != nil {
		if err := h.pw.Stop(); err != nil {
			slog.Warn("failed to stop playwright", "error", err)
		}
	}
}
