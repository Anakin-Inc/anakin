// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

type StatsHandler struct {
	db *sql.DB
}

type StatsResponse struct {
	TotalJobs     int64            `json:"total_jobs"`
	Completed     int64            `json:"completed"`
	Failed        int64            `json:"failed"`
	AvgDurationMs int64            `json:"avg_duration_ms"`
	JobsToday     int64            `json:"jobs_today"`
	TopHandlers   map[string]int64 `json:"top_handlers"`
}

func NewStatsHandler(db *sql.DB) *StatsHandler {
	return &StatsHandler{db: db}
}

func (h *StatsHandler) Get(c *fiber.Ctx) error {
	var stats StatsResponse

	err := h.db.QueryRowContext(c.Context(), `
		SELECT
			COUNT(*) AS total_jobs,
			COUNT(*) FILTER (WHERE status = 'completed') AS completed,
			COUNT(*) FILTER (WHERE status = 'failed') AS failed,
			COALESCE(ROUND(AVG(duration_ms) FILTER (WHERE status = 'completed')), 0)::BIGINT AS avg_duration_ms,
			COUNT(*) FILTER (WHERE created_at >= date_trunc('day', NOW())) AS jobs_today
		FROM scrape_requests
	`).Scan(
		&stats.TotalJobs,
		&stats.Completed,
		&stats.Failed,
		&stats.AvgDurationMs,
		&stats.JobsToday,
	)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to fetch stats",
		})
	}

	stats.TopHandlers = map[string]int64{}
	rows, err := h.db.QueryContext(c.Context(), `
		SELECT result::jsonb->>'handler' AS handler, COUNT(*) AS count
		FROM scrape_requests
		WHERE result IS NOT NULL
			AND result <> ''
			AND result::jsonb ? 'handler'
			AND COALESCE(result::jsonb->>'handler', '') <> ''
		GROUP BY handler
		ORDER BY count DESC
	`)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to fetch handler stats",
		})
	}
	defer rows.Close()

	for rows.Next() {
		var (
			handler string
			count   int64
		)
		if err := rows.Scan(&handler, &count); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
				Error: "internal_error", Message: "Failed to parse handler stats",
			})
		}
		stats.TopHandlers[handler] = count
	}
	if err := rows.Err(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to read handler stats",
		})
	}

	return c.JSON(stats)
}