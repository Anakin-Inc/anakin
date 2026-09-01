// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

func TestValidateURLRejectsSpecialPurposeAddresses(t *testing.T) {
	h := NewScraperHandler(store.NewMemoryStore(), nil, false)
	ctx := context.Background()

	rejected := []struct {
		url string
		why string
	}{
		{"http://100.100.100.200/latest/meta-data/", "Alibaba Cloud instance metadata"},
		{"http://169.254.169.254/latest/meta-data/", "AWS/GCP/Azure instance metadata"},
		{"http://100.64.0.1/", "RFC 6598 shared address space"},
		{"http://198.18.0.1/", "RFC 2544 benchmarking range"},
		{"http://255.255.255.255/", "limited broadcast"},
		{"http://127.0.0.1:5432/", "loopback"},
		{"http://10.0.0.1/", "RFC 1918"},
	}
	for _, tt := range rejected {
		t.Run("reject "+tt.url, func(t *testing.T) {
			if err := h.validateURL(ctx, tt.url); err == nil {
				t.Errorf("validateURL(%q) = nil, want an error (%s)", tt.url, tt.why)
			}
		})
	}

	allowed := []string{
		"https://example.com/",
		"http://8.8.8.8/",
		"http://100.63.255.255/",  // one below 100.64.0.0/10
		"http://100.128.0.1/",     // one above 100.64.0.0/10
		"http://198.20.0.1/",      // one above 198.18.0.0/15
		"http://223.255.255.255/", // below the multicast range
	}
	for _, u := range allowed {
		t.Run("allow "+u, func(t *testing.T) {
			if err := h.validateURL(ctx, u); err != nil {
				t.Errorf("validateURL(%q) = %v, want nil", u, err)
			}
		})
	}
}

// TestValidateURLHonoursAllowPrivateTargets keeps the documented escape hatch for
// operators who deliberately scrape internal sites.
func TestValidateURLHonoursAllowPrivateTargets(t *testing.T) {
	h := NewScraperHandler(store.NewMemoryStore(), nil, true)

	for _, u := range []string{"http://100.100.100.200/", "http://127.0.0.1:8080/", "http://10.0.0.1/"} {
		if err := h.validateURL(context.Background(), u); err != nil {
			t.Errorf("with ALLOW_PRIVATE_TARGETS set, validateURL(%q) = %v, want nil", u, err)
		}
	}
}

// TestScrapeSyncRejectsMetadataAddress checks the response a caller actually sees:
// a 400 with the invalid_url code, before any job is created or queued.
func TestScrapeSyncRejectsMetadataAddress(t *testing.T) {
	h := NewScraperHandler(store.NewMemoryStore(), nil, false)

	app := fiber.New()
	app.Post("/v1/scrape", h.ScrapeSync)

	body := `{"url":"http://100.100.100.200/latest/meta-data/"}`
	req := httptest.NewRequest("POST", "/v1/scrape", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, fiber.StatusBadRequest)
	}

	var got struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decoding the error response: %v", err)
	}
	if got.Error != "invalid_url" {
		t.Errorf("error code = %q, want %q", got.Error, "invalid_url")
	}
	if !strings.Contains(got.Message, "100.100.100.200") {
		t.Errorf("message should name the rejected address, got %q", got.Message)
	}
}
