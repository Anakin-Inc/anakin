// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

func TestScrapeSyncTimeout(t *testing.T) {
	const (
		defaultTimeout = 30 * time.Second
		maxTimeout     = 120 * time.Second
	)

	resolveTimeout := func(reqTimeout int) time.Duration {
		if reqTimeout <= 0 {
			return defaultTimeout
		}
		d := time.Duration(reqTimeout) * time.Second
		if d > maxTimeout {
			return maxTimeout
		}
		return d
	}

	tests := []struct {
		name     string
		input    int
		expected time.Duration
	}{
		{"zero uses default", 0, 30 * time.Second},
		{"negative uses default", -5, 30 * time.Second},
		{"custom 60s", 60, 60 * time.Second},
		{"exactly 120s", 120, 120 * time.Second},
		{"above max is capped at 120s", 200, 120 * time.Second},
		{"1s minimum valid", 1, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTimeout(tt.input)
			if got != tt.expected {
				t.Errorf("resolveTimeout(%d) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

type failingBatchStore struct {
	batchCalls      int
	individualCalls int
}

func (s *failingBatchStore) CreateJob(context.Context, store.JobRecord) error {
	s.individualCalls++
	return nil
}

func (s *failingBatchStore) CreateBatchJobs(context.Context, store.JobRecord, []store.JobRecord) error {
	s.batchCalls++
	return errors.New("simulated child insert failure")
}

func (*failingBatchStore) GetJob(context.Context, string) (*store.JobRecord, error) {
	return nil, errors.New("not implemented")
}

func (*failingBatchStore) GetChildJobs(context.Context, string) ([]store.JobRecord, error) {
	return nil, errors.New("not implemented")
}

func (*failingBatchStore) UpdateStatus(context.Context, string, string, *string, *int) error {
	return errors.New("not implemented")
}

func (*failingBatchStore) UpdateCompleted(context.Context, string, int, int) error {
	return errors.New("not implemented")
}

func (*failingBatchStore) StoreResult(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (*failingBatchStore) UpdateParentBatchStatus(context.Context, string) error {
	return errors.New("not implemented")
}

func (*failingBatchStore) Ping(context.Context) error {
	return nil
}

func TestCreateBatchJobReturnsErrorWhenAtomicInsertFails(t *testing.T) {
	jobStore := &failingBatchStore{}
	app := fiber.New()
	app.Post("/v1/url-scraper/batch", NewScraperHandler(jobStore, nil).CreateBatchJob)

	req := httptest.NewRequest(
		http.MethodPost,
		"/v1/url-scraper/batch",
		strings.NewReader(`{"urls":["https://example.com/one","https://example.com/two"]}`),
	)
	req.Header.Set("Content-Type", "application/json")
	res, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusInternalServerError)
	}
	if jobStore.batchCalls != 1 {
		t.Fatalf("CreateBatchJobs calls = %d, want 1", jobStore.batchCalls)
	}
	if jobStore.individualCalls != 0 {
		t.Fatalf("individual CreateJob calls = %d, want 0", jobStore.individualCalls)
	}
}
