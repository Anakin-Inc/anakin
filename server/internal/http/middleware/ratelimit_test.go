// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestRateLimitAllowsUpToMax verifies that requests within the limit succeed
// and the first request past the limit receives HTTP 429.
func TestRateLimitAllowsUpToMax(t *testing.T) {
	app := fiber.New()
	app.Use(RateLimit(2))
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := 1; i <= 2; i++ {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil))
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i, resp.StatusCode, fiber.StatusOK)
		}
	}

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("over-limit request: unexpected error: %v", err)
	}
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("over-limit request: status = %d, want %d", resp.StatusCode, fiber.StatusTooManyRequests)
	}
}
