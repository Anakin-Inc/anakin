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
// receive HTTP 429 with a JSON error body. Callers should install it only
// when maxPerMinute > 0 (see RATE_LIMIT in config).
func RateLimit(maxPerMinute int) fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        maxPerMinute,
		Expiration: 1 * time.Minute,
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "rate_limited",
				"message": "Too many requests. Please retry in a moment.",
			})
		},
	})
}
