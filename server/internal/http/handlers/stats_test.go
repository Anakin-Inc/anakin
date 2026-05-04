// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gofiber/fiber/v2"
)

func TestStatsHandlerGetSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT\\s+COUNT\\(\\*\\) AS total_jobs").
		WillReturnRows(sqlmock.NewRows([]string{
			"total_jobs", "completed", "failed", "avg_duration_ms", "jobs_today",
		}).AddRow(1234, 1100, 134, 2300, 45))

	mock.ExpectQuery("SELECT result::jsonb->>'handler' AS handler, COUNT\\(\\*\\) AS count").
		WillReturnRows(sqlmock.NewRows([]string{"handler", "count"}).
			AddRow("http", 900).
			AddRow("browser", 200))

	app := fiber.New()
	h := NewStatsHandler(db)
	app.Get("/v1/stats", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusOK)
	}

	var body StatsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if body.TotalJobs != 1234 || body.Completed != 1100 || body.Failed != 134 || body.AvgDurationMs != 2300 || body.JobsToday != 45 {
		t.Fatalf("unexpected aggregate stats: %+v", body)
	}
	if body.TopHandlers["http"] != 900 || body.TopHandlers["browser"] != 200 {
		t.Fatalf("unexpected top_handlers: %+v", body.TopHandlers)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestStatsHandlerGetAggregateQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT\\s+COUNT\\(\\*\\) AS total_jobs").
		WillReturnError(assertErr("aggregate query failed"))

	app := fiber.New()
	h := NewStatsHandler(db)
	app.Get("/v1/stats", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

func TestStatsHandlerGetTopHandlersQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("SELECT\\s+COUNT\\(\\*\\) AS total_jobs").
		WillReturnRows(sqlmock.NewRows([]string{
			"total_jobs", "completed", "failed", "avg_duration_ms", "jobs_today",
		}).AddRow(1, 1, 0, 10, 1))

	mock.ExpectQuery("SELECT result::jsonb->>'handler' AS handler, COUNT\\(\\*\\) AS count").
		WillReturnError(assertErr("top handlers query failed"))

	app := fiber.New()
	h := NewStatsHandler(db)
	app.Get("/v1/stats", h.Get)

	req := httptest.NewRequest(http.MethodGet, "/v1/stats", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("unexpected status: got %d want %d", resp.StatusCode, http.StatusInternalServerError)
	}
}

type testErr string

func (e testErr) Error() string { return string(e) }

func assertErr(msg string) error { return testErr(msg) }