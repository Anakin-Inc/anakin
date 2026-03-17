package router

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/http/handlers"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/worker"
)

// Setup configures all routes.
func Setup(app *fiber.App, db *sql.DB, pool *worker.Pool) {
	healthHandler := handlers.NewHealthHandler(db)
	scraperHandler := handlers.NewScraperHandler(db, pool)
	mapHandler := handlers.NewMapHandler(db, pool)
	crawlHandler := handlers.NewCrawlHandler(db, pool)

	app.Get("/health", healthHandler.Health)

	v1 := app.Group("/v1")

	v1.Post("/url-scraper", scraperHandler.CreateJob)
	v1.Get("/url-scraper/:id", scraperHandler.GetJob)
	v1.Post("/url-scraper/batch", scraperHandler.CreateBatchJob)
	v1.Get("/url-scraper/batch/:id", scraperHandler.GetBatchJob)

	v1.Post("/map", mapHandler.CreateMapJob)
	v1.Get("/map/:id", mapHandler.GetMapJob)

	v1.Post("/crawl", crawlHandler.CreateCrawlJob)
	v1.Get("/crawl/:id", crawlHandler.GetCrawlJob)
}
