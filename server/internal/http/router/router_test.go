// SPDX-License-Identifier: AGPL-3.0-or-later

package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/telemetry"
)

func TestStatsRoute_NoDatabase(t *testing.T) {
	app := fiber.New()
	s := store.NewMemoryStore()
	tel := telemetry.New(nil, false, "", false, 0)

	Setup(app, s, nil, nil, nil, tel)

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}