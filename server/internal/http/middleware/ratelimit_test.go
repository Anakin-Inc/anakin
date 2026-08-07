// SPDX-License-Identifier: AGPL-3.0-or-later

package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// TestRateLimitAllowsUpToMax verifies that requests within the limit succeed
// and the first request past the limit receives HTTP 429 with a Retry-After
// header so clients can back off.
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
	if resp.Header.Get("Retry-After") == "" {
		t.Error("over-limit response missing Retry-After header")
	}
}

// TestRateLimitSkipsHealth verifies the /health endpoint is exempt from rate
// limiting so uptime probes are never throttled.
func TestRateLimitSkipsHealth(t *testing.T) {
	app := fiber.New()
	app.Use(RateLimit(1))
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	for i := 1; i <= 5; i++ {
		resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/health", nil))
		if err != nil {
			t.Fatalf("health request %d: unexpected error: %v", i, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("health request %d: status = %d, want %d (health must be exempt)", i, resp.StatusCode, fiber.StatusOK)
		}
	}
}
