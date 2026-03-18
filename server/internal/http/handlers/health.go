// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	db *sql.DB
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(db *sql.DB) *HealthHandler {
	return &HealthHandler{db: db}
}

// Health returns the service health status.
func (h *HealthHandler) Health(c *fiber.Ctx) error {
	dbOK := true
	if err := h.db.PingContext(c.Context()); err != nil {
		dbOK = false
	}

	return c.JSON(fiber.Map{
		"status":   "ok",
		"database": dbOK,
		"service":  "anakinscraper",
	})
}
