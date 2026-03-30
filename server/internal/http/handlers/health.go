// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
)

// HealthHandler handles health check requests.
type HealthHandler struct {
	store store.JobStore
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(s store.JobStore) *HealthHandler {
	return &HealthHandler{store: s}
}

// Health returns the service health status.
func (h *HealthHandler) Health(c *fiber.Ctx) error {
	dbOK := true
	if err := h.store.Ping(c.Context()); err != nil {
		dbOK = false
	}

	return c.JSON(fiber.Map{
		"status":   "ok",
		"database": dbOK,
		"service":  "anakinscraper",
	})
}
