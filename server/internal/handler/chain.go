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

	for _, h := range c.handlers {
		if len(req.AllowedHandlers) > 0 && !contains(req.AllowedHandlers, h.Name()) {
			continue
		}

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

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// HandlerNames returns the names of all registered handlers.
func (c *Chain) HandlerNames() []string {
	names := make([]string, len(c.handlers))
	for i, h := range c.handlers {
		names[i] = h.Name()
	}
	return names
}
