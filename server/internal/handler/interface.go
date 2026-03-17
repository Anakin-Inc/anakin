package handler

import (
	"context"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/models"
)

// ScrapingHandler defines the contract for scraping implementations.
type ScrapingHandler interface {
	Name() string
	CanHandle(ctx context.Context, req *models.HandlerRequest) bool
	Scrape(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error)
	IsHealthy() bool
}
