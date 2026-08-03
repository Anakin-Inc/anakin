// SPDX-License-Identifier: AGPL-3.0-or-later

package router

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	_ "github.com/lib/pq"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

// newTestApp wires the real routes. The DB handle is opened but never dialled — the
// domain-config routes only need it to be non-nil to register, and auth rejects the
// request long before any query runs.
func newTestApp(t *testing.T, apiKey string) *fiber.App {
	t.Helper()

	db, err := sql.Open("postgres", "postgres://unused:unused@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	app := fiber.New()
	Setup(app, store.NewMemoryStore(), db, nil, nil, nil, apiKey)
	return app
}

func do(t *testing.T, app *fiber.App, method, path string, headers map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	return res.StatusCode
}

func TestAuth_KeyedInstance(t *testing.T) {
	app := newTestApp(t, "s3cret")

	tests := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"no key", nil, http.StatusUnauthorized},
		{"wrong key", map[string]string{"X-API-Key": "nope"}, http.StatusUnauthorized},
		{"empty key header", map[string]string{"X-API-Key": ""}, http.StatusUnauthorized},
		{"x-api-key", map[string]string{"X-API-Key": "s3cret"}, http.StatusOK},
		{"api-key", map[string]string{"Api-Key": "s3cret"}, http.StatusOK},
		{"bearer", map[string]string{"Authorization": "Bearer s3cret"}, http.StatusOK},
		{"bare authorization", map[string]string{"Authorization": "s3cret"}, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := do(t, app, http.MethodGet, "/v1/telemetry/status", tt.headers); got != tt.want {
				t.Errorf("GET /v1/telemetry/status = %d, want %d", got, tt.want)
			}
		})
	}

	// Health must stay reachable for probes.
	if got := do(t, app, http.MethodGet, "/health", nil); got != http.StatusOK {
		t.Errorf("GET /health = %d, want 200", got)
	}
}

func TestAuth_OpenInstance(t *testing.T) {
	app := newTestApp(t, "")

	if got := do(t, app, http.MethodGet, "/v1/telemetry/status", nil); got != http.StatusOK {
		t.Errorf("GET /v1/telemetry/status = %d, want 200 on an open instance", got)
	}

	// Domain config writes stay closed without a key, whatever the caller sends.
	writes := []struct{ method, path string }{
		{http.MethodPost, "/v1/domain-configs"},
		{http.MethodPut, "/v1/domain-configs/example.com"},
		{http.MethodDelete, "/v1/domain-configs/example.com"},
	}
	for _, w := range writes {
		if got := do(t, app, w.method, w.path, nil); got != http.StatusUnauthorized {
			t.Errorf("%s %s = %d, want 401", w.method, w.path, got)
		}
	}
}

func TestAuth_KeyedInstanceAllowsDomainConfigWrites(t *testing.T) {
	app := newTestApp(t, "s3cret")

	// With a valid key the request reaches the handler, which then fails on the
	// unreachable database — a 500, not a 401. That is the pass condition here.
	got := do(t, app, http.MethodPost, "/v1/domain-configs",
		map[string]string{"X-API-Key": "s3cret"})
	if got == http.StatusUnauthorized {
		t.Error("POST /v1/domain-configs was rejected as unauthorized despite a valid key")
	}
}
