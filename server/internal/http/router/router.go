// SPDX-License-Identifier: AGPL-3.0-or-later

package router

import (
	"crypto/subtle"
	"database/sql"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/http/handlers"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/proxy"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/telemetry"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

// Setup configures all routes. db may be nil when running without PostgreSQL.
// apiKey may be empty, which leaves the instance open — see requireAPIKey.
func Setup(app *fiber.App, s store.JobStore, db *sql.DB, pool *worker.Pool, proxyPool *proxy.Pool, tel *telemetry.Collector, apiKey string, allowPrivateTargets bool) {
	healthHandler := handlers.NewHealthHandler(s)
	scraperHandler := handlers.NewScraperHandler(s, pool, allowPrivateTargets)
	proxyScoresHandler := handlers.NewProxyScoresHandler(proxyPool)

	// /health stays open so container and load-balancer probes work without credentials.
	app.Get("/health", healthHandler.Health)

	v1 := app.Group("/v1")
	if apiKey != "" {
		v1.Use(requireAPIKey(apiKey))
	}

	v1.Post("/scrape", scraperHandler.ScrapeSync)
	v1.Post("/url-scraper", scraperHandler.CreateJob)
	v1.Get("/url-scraper/:id", scraperHandler.GetJob)
	v1.Post("/url-scraper/batch", scraperHandler.CreateBatchJob)
	v1.Get("/url-scraper/batch/:id", scraperHandler.GetBatchJob)

	if db != nil {
		domainConfigHandler := handlers.NewDomainConfigHandler(db)
		// Writes here change how every future scrape is routed (proxy, headers, blocking),
		// so they always need a key — an open instance gets read-only access.
		write := requireKeyConfigured(apiKey)
		v1.Get("/domain-configs", domainConfigHandler.List)
		v1.Post("/domain-configs", write, domainConfigHandler.Create)
		v1.Get("/domain-configs/:domain", domainConfigHandler.Get)
		v1.Put("/domain-configs/:domain", write, domainConfigHandler.Update)
		v1.Delete("/domain-configs/:domain", write, domainConfigHandler.Delete)
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

// requireAPIKey rejects requests that do not present the configured key as
// X-API-Key, Api-Key, or Authorization: Bearer <key>.
func requireAPIKey(apiKey string) fiber.Handler {
	want := []byte(apiKey)
	return func(c *fiber.Ctx) error {
		if subtle.ConstantTimeCompare([]byte(presentedKey(c)), want) == 1 {
			return c.Next()
		}
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Missing or invalid API key. Send it as X-API-Key or Authorization: Bearer <key>.",
		})
	}
}

// requireKeyConfigured blocks a route on instances running without API_KEY. On a
// keyed instance requireAPIKey has already authenticated the caller.
func requireKeyConfigured(apiKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if apiKey != "" {
			return c.Next()
		}
		return c.Status(fiber.StatusUnauthorized).JSON(models.ErrorResponse{
			Error:   "unauthorized",
			Message: "Set API_KEY on the server to modify domain configs.",
		})
	}
}

func presentedKey(c *fiber.Ctx) string {
	if k := c.Get("X-API-Key"); k != "" {
		return k
	}
	if k := c.Get("Api-Key"); k != "" {
		return k
	}
	if auth := c.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
