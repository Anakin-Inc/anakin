// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// Chain orchestrates handler execution with fallback.
type Chain struct {
	handlers []ScrapingHandler
}

// NewChain creates a handler chain from an ordered list of handlers.
func NewChain(handlers []ScrapingHandler) *Chain {
	return &Chain{handlers: handlers}
}

// Execute runs handlers in order until one succeeds.
func (c *Chain) Execute(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	if len(c.handlers) == 0 {
		return nil, fmt.Errorf("no handlers registered")
	}

	var lastErr error

	for _, h := range c.order(req.AllowedHandlers) {
		if !h.CanHandle(ctx, req) {
			slog.Debug("handler cannot handle request", "handler", h.Name(), "url", req.URL)
			continue
		}

		if !h.IsHealthy() {
			slog.Warn("handler unhealthy, skipping", "handler", h.Name())
			continue
		}

		start := time.Now()
		result, err := h.Scrape(ctx, req)
		elapsed := time.Since(start)

		if err != nil {
			slog.Warn("handler failed",
				"handler", h.Name(),
				"url", req.URL,
				"duration_ms", elapsed.Milliseconds(),
				"error", err,
			)
			lastErr = err
			continue
		}

		slog.Info("handler succeeded",
			"handler", h.Name(),
			"url", req.URL,
			"duration_ms", elapsed.Milliseconds(),
			"status_code", result.StatusCode,
		)

		result.Handler = h.Name()
		result.DurationMs = int(elapsed.Milliseconds())
		return result, nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no available handlers for request")
	}
	return nil, fmt.Errorf("all handlers failed: %w", lastErr)
}

// order returns the handlers to try, in the order they should be tried.
//
// A domain config's handlerChain is documented as "which handlers to try, in
// order", so it decides the fallback order and not just which handlers are
// eligible: ["browser", "http"] must reach the browser first even though the
// HTTP handler is registered first. With no allow-list the registration order
// from main.go stands.
//
// Names that match no registered handler are skipped — a config can ask for
// "anakin" on an instance started without ANAKIN_API_KEY. A name repeated in
// the config is tried once.
func (c *Chain) order(allowed []string) []ScrapingHandler {
	if len(allowed) == 0 {
		return c.handlers
	}

	byName := make(map[string]ScrapingHandler, len(c.handlers))
	for _, h := range c.handlers {
		byName[h.Name()] = h
	}

	ordered := make([]ScrapingHandler, 0, len(allowed))
	seen := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		if seen[name] {
			continue
		}
		seen[name] = true

		h, ok := byName[name]
		if !ok {
			slog.Warn("handler in chain is not registered, skipping", "handler", name)
			continue
		}
		ordered = append(ordered, h)
	}
	return ordered
}

// HandlerNames returns the names of all registered handlers.
func (c *Chain) HandlerNames() []string {
	names := make([]string, len(c.handlers))
	for i, h := range c.handlers {
		names[i] = h.Name()
	}
	return names
}
