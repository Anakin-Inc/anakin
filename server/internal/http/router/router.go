// SPDX-License-Identifier: AGPL-3.0-or-later

package router

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/http/handlers"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/proxy"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

// Setup configures all routes.
func Setup(app *fiber.App, db *sql.DB, pool *worker.Pool, proxyPool *proxy.Pool) {
	healthHandler := handlers.NewHealthHandler(db)
	scraperHandler := handlers.NewScraperHandler(db, pool)
	domainConfigHandler := handlers.NewDomainConfigHandler(db)
	proxyScoresHandler := handlers.NewProxyScoresHandler(proxyPool)

	app.Get("/health", healthHandler.Health)

	v1 := app.Group("/v1")

	v1.Post("/scrape", scraperHandler.ScrapeSync)
	v1.Post("/url-scraper", scraperHandler.CreateJob)
	v1.Get("/url-scraper/:id", scraperHandler.GetJob)
	v1.Post("/url-scraper/batch", scraperHandler.CreateBatchJob)
	v1.Get("/url-scraper/batch/:id", scraperHandler.GetBatchJob)

	v1.Get("/domain-configs", domainConfigHandler.List)
	v1.Post("/domain-configs", domainConfigHandler.Create)
	v1.Get("/domain-configs/:domain", domainConfigHandler.Get)
	v1.Put("/domain-configs/:domain", domainConfigHandler.Update)
	v1.Delete("/domain-configs/:domain", domainConfigHandler.Delete)

	v1.Get("/proxy/scores", proxyScoresHandler.GetScores)
}
