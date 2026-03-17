package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/models"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/worker"
)

type ScraperHandler struct {
	db   *sql.DB
	pool *worker.Pool
}

func NewScraperHandler(db *sql.DB, pool *worker.Pool) *ScraperHandler {
	return &ScraperHandler{db: db, pool: pool}
}

func (h *ScraperHandler) CreateJob(c *fiber.Ctx) error {
	var req models.ScrapeRequest
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
		jobID, req.URL, models.JobTypeURLScraper, models.JobStatusPending,
		req.Country, string(payload), req.ForceFresh, now,
	)
	if err != nil {
		slog.Error("failed to insert job", "error", err, "jobId", jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to create job",
		})
	}

	h.pool.Submit(models.JobMessage{
		JobID:      jobID,
		URL:        req.URL,
		JobType:    models.JobTypeURLScraper,
		Country:    req.Country,
		ForceFresh: req.ForceFresh,
		UseBrowser: req.UseBrowser,
	})

	slog.Info("job created", "jobId", jobID, "url", req.URL)

	return c.Status(fiber.StatusCreated).JSON(models.JobResponse{
		ID:        jobID,
		Status:    models.JobStatusPending,
		URL:       req.URL,
		JobType:   models.JobTypeURLScraper,
		CreatedAt: now.Format(time.RFC3339),
	})
}

func (h *ScraperHandler) GetJob(c *fiber.Ctx) error {
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
		errorMsg                    sql.NullString
		resultJSON                  sql.NullString
		durationMs                  sql.NullInt64
	)

	err := h.db.QueryRowContext(c.Context(),
		`SELECT id, url, job_type, status, created_at, completed_at, error, result, duration_ms
		 FROM scrape_requests WHERE id = $1`,
		jobID,
	).Scan(&id, &jobURL, &jobType, &status, &createdAt, &completedAt, &errorMsg, &resultJSON, &durationMs)

	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "not_found", Message: "Job not found",
		})
	}
	if err != nil {
		slog.Error("failed to fetch job", "error", err, "jobId", jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to fetch job",
		})
	}

	resp := models.JobResponse{
		ID: id, Status: status, URL: jobURL, JobType: jobType,
		CreatedAt: createdAt.Format(time.RFC3339),
	}
	if completedAt.Valid {
		t := completedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &t
	}
	if errorMsg.Valid {
		resp.Error = &errorMsg.String
	}
	if durationMs.Valid {
		d := int(durationMs.Int64)
		resp.DurationMs = &d
	}

	if status == models.JobStatusCompleted && resultJSON.Valid && resultJSON.String != "" {
		var result scrapeResultJSON
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err == nil {
			resp.HTML = result.HTML
			resp.CleanedHTML = result.CleanedHTML
			resp.Markdown = result.Markdown
			resp.Cached = result.Cached
		}
	}

	return c.JSON(resp)
}

func (h *ScraperHandler) CreateBatchJob(c *fiber.Ctx) error {
	var req models.BatchScrapeRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Invalid request body",
		})
	}
	if len(req.URLs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "At least one URL is required",
		})
	}
	if len(req.URLs) > 10 {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Maximum 10 URLs per batch",
		})
	}
	for i, u := range req.URLs {
		if err := validateURL(u); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
				Error: "invalid_url", Message: fmt.Sprintf("Invalid URL at index %d: %s", i, err.Error()),
			})
		}
	}

	parentJobID := uuid.New().String()
	now := time.Now().UTC()
	payload, _ := json.Marshal(req)

	_, err := h.db.ExecContext(c.Context(),
		`INSERT INTO scrape_requests (id, url, job_type, status, country, payload, force_fresh, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		parentJobID, req.URLs[0], models.JobTypeBatchURLScraper, models.JobStatusPending,
		req.Country, string(payload), false, now,
	)
	if err != nil {
		slog.Error("failed to insert parent batch job", "error", err, "jobId", parentJobID)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to create batch job",
		})
	}

	for _, u := range req.URLs {
		childID := uuid.New().String()
		_, err = h.db.ExecContext(c.Context(),
			`INSERT INTO scrape_requests (id, url, job_type, status, country, payload, force_fresh, parent_job_id, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			childID, u, models.JobTypeURLScraper, models.JobStatusPending,
			req.Country, "{}", false, parentJobID, now,
		)
		if err != nil {
			slog.Error("failed to insert child job", "error", err, "childId", childID)
			continue
		}
		h.pool.Submit(models.JobMessage{
			JobID:       childID,
			URL:         u,
			JobType:     models.JobTypeURLScraper,
			Country:     req.Country,
			UseBrowser:  req.UseBrowser,
			ParentJobID: parentJobID,
		})
	}

	slog.Info("batch job created", "jobId", parentJobID, "urlCount", len(req.URLs))

	return c.Status(fiber.StatusCreated).JSON(models.BatchJobResponse{
		ID: parentJobID, Status: models.JobStatusPending,
		JobType: models.JobTypeBatchURLScraper, URLs: req.URLs,
		CreatedAt: now.Format(time.RFC3339),
	})
}

func (h *ScraperHandler) GetBatchJob(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Job ID is required",
		})
	}

	var (
		id, status  string
		createdAt   time.Time
		completedAt sql.NullTime
		durationMs  sql.NullInt64
	)
	err := h.db.QueryRowContext(c.Context(),
		`SELECT id, status, created_at, completed_at, duration_ms
		 FROM scrape_requests WHERE id = $1 AND job_type = $2`,
		jobID, models.JobTypeBatchURLScraper,
	).Scan(&id, &status, &createdAt, &completedAt, &durationMs)
	if err == sql.ErrNoRows {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "not_found", Message: "Batch job not found",
		})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to fetch batch job",
		})
	}

	rows, err := h.db.QueryContext(c.Context(),
		`SELECT id, url, status, error, result, duration_ms
		 FROM scrape_requests WHERE parent_job_id = $1 ORDER BY created_at ASC`, jobID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to fetch batch job results",
		})
	}
	defer rows.Close()

	results := make([]models.BatchResult, 0)
	urls := make([]string, 0)
	idx := 0
	hasPending, hasProcessing := false, false

	for rows.Next() {
		var (
			childID, childURL, childStatus string
			errorMsg, resultJSON           sql.NullString
			childDuration                  sql.NullInt64
		)
		if err := rows.Scan(&childID, &childURL, &childStatus, &errorMsg, &resultJSON, &childDuration); err != nil {
			continue
		}
		urls = append(urls, childURL)
		item := models.BatchResult{Index: idx, URL: childURL, Status: childStatus}
		idx++
		if childDuration.Valid {
			d := int(childDuration.Int64)
			item.DurationMs = &d
		}
		if errorMsg.Valid {
			item.Error = &errorMsg.String
		}
		if childStatus == models.JobStatusCompleted && resultJSON.Valid && resultJSON.String != "" {
			var result scrapeResultJSON
			if err := json.Unmarshal([]byte(resultJSON.String), &result); err == nil {
				item.HTML = result.HTML
				item.CleanedHTML = result.CleanedHTML
				item.Markdown = result.Markdown
				item.Cached = result.Cached
			}
		}
		if childStatus == models.JobStatusPending {
			hasPending = true
		}
		if childStatus == models.JobStatusProcessing {
			hasProcessing = true
		}
		results = append(results, item)
	}

	derivedStatus := status
	if hasPending {
		derivedStatus = models.JobStatusPending
	} else if hasProcessing {
		derivedStatus = models.JobStatusProcessing
	} else if len(results) > 0 {
		derivedStatus = models.JobStatusCompleted
	}

	resp := models.BatchJobResponse{
		ID: id, Status: derivedStatus, JobType: models.JobTypeBatchURLScraper,
		URLs: urls, Results: results, CreatedAt: createdAt.Format(time.RFC3339),
	}
	if completedAt.Valid {
		t := completedAt.Time.Format(time.RFC3339)
		resp.CompletedAt = &t
	}
	if durationMs.Valid {
		d := int(durationMs.Int64)
		resp.DurationMs = &d
	}
	return c.JSON(resp)
}

type scrapeResultJSON struct {
	HTML        *string `json:"html,omitempty"`
	CleanedHTML *string `json:"cleanedHtml,omitempty"`
	Markdown    *string `json:"markdown,omitempty"`
	Cached      *bool   `json:"cached,omitempty"`
}

func validateURL(rawURL string) error {
	if rawURL == "" {
		return fiber.NewError(fiber.StatusBadRequest, "URL is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "Invalid URL format")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fiber.NewError(fiber.StatusBadRequest, "URL must use http or https scheme")
	}
	if u.Host == "" {
		return fiber.NewError(fiber.StatusBadRequest, "URL must have a host")
	}
	return nil
}
