// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"database/sql"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/domain"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/proxy"
)

// configRepository is the part of *domain.Repository these routes use. It is an
// interface so the request handling can be exercised without a database.
type configRepository interface {
	GetAll(ctx context.Context) ([]*domain.DomainConfig, error)
	GetByDomain(ctx context.Context, domainName string) (*domain.DomainConfig, error)
	Create(ctx context.Context, cfg *domain.DomainConfig) error
	Update(ctx context.Context, cfg *domain.DomainConfig) error
	Delete(ctx context.Context, domainName string) error
}

type DomainConfigHandler struct {
	repo configRepository
}

func NewDomainConfigHandler(db *sql.DB) *DomainConfigHandler {
	return &DomainConfigHandler{repo: domain.NewRepository(db)}
}

func (h *DomainConfigHandler) List(c *fiber.Ctx) error {
	configs, err := h.repo.GetAll(c.Context())
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to list domain configs",
		})
	}
	if configs == nil {
		configs = []*domain.DomainConfig{}
	}
	return c.JSON(configs)
}

func (h *DomainConfigHandler) Get(c *fiber.Ctx) error {
	domainName := c.Params("domain")
	cfg, err := h.repo.GetByDomain(c.Context(), domainName)
	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "not_found", Message: "Domain config not found",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to get domain config",
		})
	}
	return c.JSON(cfg)
}

func (h *DomainConfigHandler) Create(c *fiber.Ctx) error {
	var cfg domain.DomainConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Invalid request body",
		})
	}
	if cfg.Domain == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Domain is required",
		})
	}
	if len(cfg.HandlerChain) == 0 {
		cfg.HandlerChain = []string{"http", "browser"}
	}
	if cfg.RequestTimeoutMs == 0 {
		cfg.RequestTimeoutMs = 30000
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 2
	}
	if err := cfg.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	if err := h.repo.Create(c.Context(), &cfg); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to create domain config",
		})
	}
	return c.Status(fiber.StatusCreated).JSON(cfg)
}

func (h *DomainConfigHandler) Update(c *fiber.Ctx) error {
	domainName := c.Params("domain")
	var cfg domain.DomainConfig
	if err := c.BodyParser(&cfg); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Invalid request body",
		})
	}
	cfg.Domain = domainName
	if err := cfg.Validate(); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: err.Error(),
		})
	}

	err := h.repo.Update(c.Context(), &cfg)
	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "not_found", Message: "Domain config not found",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to update domain config",
		})
	}
	return c.JSON(cfg)
}

func (h *DomainConfigHandler) Delete(c *fiber.Ctx) error {
	domainName := c.Params("domain")
	if err := h.repo.Delete(c.Context(), domainName); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to delete domain config",
		})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ProxyScoresHandler returns current proxy scores for monitoring.
type ProxyScoresHandler struct {
	pool *proxy.Pool
}

func NewProxyScoresHandler(pool *proxy.Pool) *ProxyScoresHandler {
	return &ProxyScoresHandler{pool: pool}
}

func (h *ProxyScoresHandler) GetScores(c *fiber.Ctx) error {
	if h.pool == nil {
		return c.JSON(fiber.Map{"proxies": []string{}, "scores": map[string]interface{}{}})
	}
	return c.JSON(fiber.Map{"scores": h.pool.Scores()})
}
