// SPDX-License-Identifier: AGPL-3.0-or-later

package processor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/handler"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

type cancelingProcessorHandler struct {
	cancel context.CancelFunc
	calls  int
}

func (h *cancelingProcessorHandler) Name() string { return "canceling" }

func (h *cancelingProcessorHandler) CanHandle(context.Context, *models.HandlerRequest) bool {
	return true
}

func (h *cancelingProcessorHandler) IsHealthy() bool { return true }

func (h *cancelingProcessorHandler) Scrape(ctx context.Context, _ *models.HandlerRequest) (*models.ScrapeResult, error) {
	h.calls++
	h.cancel()
	return nil, ctx.Err()
}

func TestProcessScrapeJobStopsRetriesAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scraper := &cancelingProcessorHandler{cancel: cancel}
	processor := NewProcessor(
		store.NewMemoryStore(),
		handler.NewChain([]handler.ScrapingHandler{scraper}),
		nil,
		nil,
		nil,
		nil,
	)

	err := processor.processScrapeJob(ctx, models.JobMessage{
		JobID:  "job-1",
		URL:    "https://example.com",
		JobType: models.JobTypeURLScraper,
	}, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if scraper.calls != 1 {
		t.Fatalf("handler calls = %d, want 1", scraper.calls)
	}
}
