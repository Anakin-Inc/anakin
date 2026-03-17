package handlers

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/models"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/worker"
)

type MapHandler struct {
	db   *sql.DB
	pool *worker.Pool
}

func NewMapHandler(db *sql.DB, pool *worker.Pool) *MapHandler {
	return &MapHandler{db: db, pool: pool}
}

func (h *MapHandler) CreateMapJob(c *fiber.Ctx) error {
	var req models.MapRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Invalid request body",
		})
	}
	if err := validateURL(req.URL); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_url", Message: err.Error(),
		})
	}

	jobID := uuid.New().String()
	now := time.Now().UTC()
	payload, _ := json.Marshal(req)

	_, err := h.db.ExecContext(c.Context(),
		`INSERT INTO scrape_requests (id, url, job_type, status, country, payload, force_fresh, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		jobID, req.URL, models.JobTypeMap, models.JobStatusPending,
		"", string(payload), false, now,
	)
	if err != nil {
		slog.Error("failed to insert map job", "error", err, "jobId", jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to create job",
		})
	}

	h.pool.Submit(models.JobMessage{
		JobID:             jobID,
		URL:               req.URL,
		JobType:           models.JobTypeMap,
		UseBrowser:        req.UseBrowser,
		IncludeSubdomains: req.IncludeSubdomains,
		Limit:             req.Limit,
		Search:            req.Search,
	})

	slog.Info("map job created", "jobId", jobID, "url", req.URL)

	return c.Status(fiber.StatusCreated).JSON(models.MapResponse{
		ID: jobID, Status: models.JobStatusPending, URL: req.URL,
		CreatedAt: now.Format(time.RFC3339),
	})
}

func (h *MapHandler) GetMapJob(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Job ID is required",
		})
	}

	var (
		id, jobURL, jobType, status string
		createdAt                   time.Time
		completedAt                 sql.NullTime
		resultJSON                  sql.NullString
		durationMs                  sql.NullInt64
	)

	err := h.db.QueryRowContext(c.Context(),
		`SELECT id, url, job_type, status, created_at, completed_at, result, duration_ms
		 FROM scrape_requests WHERE id = $1`,
		jobID,
	).Scan(&id, &jobURL, &jobType, &status, &createdAt, &completedAt, &resultJSON, &durationMs)

	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "not_found", Message: "Job not found",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to fetch job",
		})
	}

	resp := models.MapResponse{
		ID: id, Status: status, URL: jobURL,
		CreatedAt: createdAt.Format(time.RFC3339),
	}
	if completedAt.Valid {
		t := completedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &t
	}
	if durationMs.Valid {
		d := int(durationMs.Int64)
		resp.DurationMs = &d
	}
	if status == models.JobStatusCompleted && resultJSON.Valid && resultJSON.String != "" {
		var result mapResultJSON
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err == nil {
			resp.Links = result.Links
			resp.TotalLinks = len(result.Links)
		}
	}
	return c.JSON(resp)
}

type mapResultJSON struct {
	Links []string `json:"links"`
}
