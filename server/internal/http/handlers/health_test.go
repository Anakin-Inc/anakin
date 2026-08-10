// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

type pingStore struct {
	store.JobStore
	err error
}

func (s pingStore) Ping(context.Context) error { return s.err }

func TestHealthReturnsReadyWhenStoreIsHealthy(t *testing.T) {
	status, body := requestHealth(t, store.NewMemoryStore())

	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if body["status"] != "ok" || body["database"] != true {
		t.Fatalf("body = %#v, want status ok and database true", body)
	}
}

func TestHealthReturnsServiceUnavailableWhenStorePingFails(t *testing.T) {
	jobStore := pingStore{
		JobStore: store.NewMemoryStore(),
		err:      errors.New("database unavailable"),
	}
	status, body := requestHealth(t, jobStore)

	if status != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", status, http.StatusServiceUnavailable)
	}
	if body["status"] != "unhealthy" || body["database"] != false {
		t.Fatalf("body = %#v, want status unhealthy and database false", body)
	}
}

func requestHealth(t *testing.T, jobStore store.JobStore) (int, map[string]interface{}) {
	t.Helper()
	app := fiber.New()
	app.Get("/health", NewHealthHandler(jobStore).Health)

	res, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	var body map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return res.StatusCode, body
}
