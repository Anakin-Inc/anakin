// SPDX-License-Identifier: AGPL-3.0-or-later

package router

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/http/handlers"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/proxy"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/telemetry"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

// Setup configures all routes. db may be nil when running without PostgreSQL.
func Setup(app *fiber.App, s store.JobStore, db *sql.DB, pool *worker.Pool, proxyPool *proxy.Pool, tel *telemetry.Collector) {
	healthHandler := handlers.NewHealthHandler(s)
	scraperHandler := handlers.NewScraperHandler(s, pool)
	proxyScoresHandler := handlers.NewProxyScoresHandler(proxyPool)

	app.Get("/health", healthHandler.Health)

	v1 := app.Group("/v1")

	v1.Post("/scrape", scraperHandler.ScrapeSync)
	v1.Post("/url-scraper", scraperHandler.CreateJob)
	v1.Get("/url-scraper/:id", scraperHandler.GetJob)
	v1.Post("/url-scraper/batch", scraperHandler.CreateBatchJob)
	v1.Get("/url-scraper/batch/:id", scraperHandler.GetBatchJob)

	if db != nil {
		domainConfigHandler := handlers.NewDomainConfigHandler(db)
		v1.Get("/domain-configs", domainConfigHandler.List)
		v1.Post("/domain-configs", domainConfigHandler.Create)
		v1.Get("/domain-configs/:domain", domainConfigHandler.Get)
		v1.Put("/domain-configs/:domain", domainConfigHandler.Update)
		v1.Delete("/domain-configs/:domain", domainConfigHandler.Delete)
	} else {
		// Return helpful error when domain configs are unavailable without DB
		noDB := func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusServiceUnavailable).JSON(models.ErrorResponse{
				Error: "no_database", Message: "Domain configs require DATABASE_URL to be set",
			})
		}
		v1.Get("/domain-configs", noDB)
		v1.Post("/domain-configs", noDB)
		v1.Get("/domain-configs/:domain", noDB)
		v1.Put("/domain-configs/:domain", noDB)
		v1.Delete("/domain-configs/:domain", noDB)
	}

	v1.Get("/proxy/scores", proxyScoresHandler.GetScores)

	v1.Get("/telemetry/status", func(c *fiber.Ctx) error {
		return c.JSON(tel.Status())
	})
}
