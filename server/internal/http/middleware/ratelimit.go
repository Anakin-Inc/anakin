// SPDX-License-Identifier: AGPL-3.0-or-later

// Package middleware provides HTTP middleware for the AnakinScraper server.
package middleware

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
)

// RateLimit returns per-IP rate-limiting middleware that allows up to
// maxPerMinute requests per client IP per minute. Requests beyond the limit
// receive HTTP 429 with a JSON error body; Fiber also sets Retry-After and
// X-RateLimit-* headers so clients can back off. The /health endpoint is
// exempt so uptime probes are never throttled.
//
// Keying uses Fiber's c.IP() (the socket peer). Behind a reverse proxy or
// load balancer, set ProxyHeader on fiber.Config (with trusted-proxy checks)
// so the real client IP is used; otherwise all clients share one bucket.
// Storage is in-process, so the limit applies per replica.
//
// Callers should install it only when maxPerMinute > 0 (see RATE_LIMIT).
func RateLimit(maxPerMinute int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        maxPerMinute,
		Expiration: 1 * time.Minute,
		// Never rate-limit health checks — probes must not be throttled into
		// reporting the service as unhealthy.
		Next: func(c *fiber.Ctx) bool {
			return c.Path() == "/health"
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate_limited",
				"message": "Too many requests. Please retry in a moment.",
			})
		},
	})
}
