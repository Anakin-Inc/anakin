// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/worker"
)

type ScraperHandler struct {
	store store.JobStore
	pool  *worker.Pool
}

func NewScraperHandler(s store.JobStore, pool *worker.Pool) *ScraperHandler {
	return &ScraperHandler{store: s, pool: pool}
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

	payload, _ := json.Marshal(req)
	if err := h.store.CreateJob(c.Context(), store.JobRecord{
		ID: jobID, JobType: models.JobTypeURLScraper, URL: req.URL,
		Country: req.Country, Payload: string(payload), ForceFresh: req.ForceFresh,
	}); err != nil {
		slog.Error("failed to insert job", "error", err, "jobId", jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to create job",
		})
	}

	h.pool.Submit(models.JobMessage{
		JobID:        jobID,
		URL:          req.URL,
		JobType:      models.JobTypeURLScraper,
		Country:      req.Country,
		ForceFresh:   req.ForceFresh,
		UseBrowser:   req.UseBrowser,
		GenerateJson: req.GenerateJson,
	})

	slog.Info("job created", "jobId", jobID, "url", req.URL)

	return c.Status(fiber.StatusCreated).JSON(models.JobResponse{
		ID:        jobID,
		Status:    models.JobStatusPending,
		URL:       req.URL,
		JobType:   models.JobTypeURLScraper,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// ScrapeSync is the synchronous endpoint: submit a job and wait for the result.
// Polls the database every 500ms until the job completes or the 30s timeout expires.
func (h *ScraperHandler) ScrapeSync(c *fiber.Ctx) error {
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
	if err := h.store.CreateJob(c.Context(), store.JobRecord{
		ID: jobID, JobType: models.JobTypeURLScraper, URL: req.URL,
		Country: req.Country, Payload: string(payload), ForceFresh: req.ForceFresh,
	}); err != nil {
		slog.Error("failed to insert job", "error", err, "jobId", jobID)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to create job",
		})
	}

	h.pool.Submit(models.JobMessage{
		JobID:        jobID,
		URL:          req.URL,
		JobType:      models.JobTypeURLScraper,
		Country:      req.Country,
		ForceFresh:   req.ForceFresh,
		UseBrowser:   req.UseBrowser,
		GenerateJson: req.GenerateJson,
		SyncRequest:  true,
	})

	slog.Info("sync scrape started", "jobId", jobID, "url", req.URL)

	// Poll store until job completes or 30s timeout
	const (
		pollInterval = 500 * time.Millisecond
		maxWait      = 30 * time.Second
	)
	deadline := time.Now().Add(maxWait)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			j, err := h.store.GetJob(c.Context(), jobID)
			if err != nil {
				continue
			}

			if j.Status == models.JobStatusCompleted || j.Status == models.JobStatusFailed {
				resp := models.JobResponse{
					ID: jobID, Status: j.Status, URL: req.URL, JobType: models.JobTypeURLScraper,
					CreatedAt: now.Format(time.RFC3339),
				}
				if j.CompletedAt != nil {
					t := j.CompletedAt.Format(time.RFC3339)
					resp.CompletedAt = &t
				}
				if j.Error != "" {
					resp.Error = &j.Error
				}
				if j.DurationMs > 0 {
					d := j.DurationMs
					resp.DurationMs = &d
				}
				if j.Status == models.JobStatusCompleted && j.Result != "" {
					var result scrapeResultJSON
					if err := json.Unmarshal([]byte(j.Result), &result); err == nil {
						resp.HTML = result.HTML
						resp.CleanedHTML = result.CleanedHTML
						resp.Markdown = result.Markdown
						resp.GeneratedJson = result.GeneratedJson
						resp.Cached = result.Cached
					}
				}
				return c.JSON(resp)
			}

			if time.Now().After(deadline) {
				return c.Status(fiber.StatusRequestTimeout).JSON(models.ErrorResponse{
					Error:   "timeout",
					Message: fmt.Sprintf("Job %s is still processing. Use GET /v1/url-scraper/%s to poll for the result, or use the async POST /v1/url-scraper endpoint for long-running scrapes.", jobID, jobID),
				})
			}
		case <-c.Context().Done():
			// Client disconnected
			return nil
		}
	}
}

func (h *ScraperHandler) GetJob(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Job ID is required",
		})
	}

	j, err := h.store.GetJob(c.Context(), jobID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "not_found", Message: "Job not found",
		})
	}

	resp := models.JobResponse{
		ID: j.ID, Status: j.Status, URL: j.URL, JobType: j.JobType,
		CreatedAt: j.CreatedAt.Format(time.RFC3339),
	}
	if j.CompletedAt != nil {
		t := j.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &t
	}
	if j.Error != "" {
		resp.Error = &j.Error
	}
	if j.DurationMs > 0 {
		d := j.DurationMs
		resp.DurationMs = &d
	}

	if j.Status == models.JobStatusCompleted && j.Result != "" {
		var result scrapeResultJSON
		if err := json.Unmarshal([]byte(j.Result), &result); err == nil {
			resp.HTML = result.HTML
			resp.CleanedHTML = result.CleanedHTML
			resp.Markdown = result.Markdown
			resp.GeneratedJson = result.GeneratedJson
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
	payload, _ := json.Marshal(req)

	if err := h.store.CreateJob(c.Context(), store.JobRecord{
		ID: parentJobID, JobType: models.JobTypeBatchURLScraper, URL: req.URLs[0],
		Country: req.Country, Payload: string(payload),
	}); err != nil {
		slog.Error("failed to insert parent batch job", "error", err, "jobId", parentJobID)
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to create batch job",
		})
	}

	for _, u := range req.URLs {
		childID := uuid.New().String()
		if err := h.store.CreateJob(c.Context(), store.JobRecord{
			ID: childID, JobType: models.JobTypeURLScraper, URL: u,
			Country: req.Country, Payload: "{}", ParentJobID: parentJobID,
		}); err != nil {
			slog.Error("failed to insert child job", "error", err, "childId", childID)
			continue
		}
		h.pool.Submit(models.JobMessage{
			JobID:        childID,
			URL:          u,
			JobType:      models.JobTypeURLScraper,
			Country:      req.Country,
			UseBrowser:   req.UseBrowser,
			GenerateJson: req.GenerateJson,
			ParentJobID:  parentJobID,
		})
	}

	slog.Info("batch job created", "jobId", parentJobID, "urlCount", len(req.URLs))

	return c.Status(fiber.StatusCreated).JSON(models.BatchJobResponse{
		ID: parentJobID, Status: models.JobStatusPending,
		JobType: models.JobTypeBatchURLScraper, URLs: req.URLs,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *ScraperHandler) GetBatchJob(c *fiber.Ctx) error {
	jobID := c.Params("id")
	if jobID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.ErrorResponse{
			Error: "invalid_request", Message: "Job ID is required",
		})
	}

	parent, err := h.store.GetJob(c.Context(), jobID)
	if err != nil || parent.JobType != models.JobTypeBatchURLScraper {
		return c.Status(fiber.StatusNotFound).JSON(models.ErrorResponse{
			Error: "not_found", Message: "Batch job not found",
		})
	}

	children, err := h.store.GetChildJobs(c.Context(), jobID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.ErrorResponse{
			Error: "internal_error", Message: "Failed to fetch batch job results",
		})
	}

	results := make([]models.BatchResult, 0, len(children))
	urls := make([]string, 0, len(children))
	hasPending, hasProcessing := false, false

	for idx, child := range children {
		urls = append(urls, child.URL)
		item := models.BatchResult{Index: idx, URL: child.URL, Status: child.Status}
		if child.DurationMs > 0 {
			d := child.DurationMs
			item.DurationMs = &d
		}
		if child.Error != "" {
			item.Error = &child.Error
		}
		if child.Status == models.JobStatusCompleted && child.Result != "" {
			var result scrapeResultJSON
			if err := json.Unmarshal([]byte(child.Result), &result); err == nil {
				item.HTML = result.HTML
				item.CleanedHTML = result.CleanedHTML
				item.Markdown = result.Markdown
				item.GeneratedJson = result.GeneratedJson
				item.Cached = result.Cached
			}
		}
		if child.Status == models.JobStatusPending {
			hasPending = true
		}
		if child.Status == models.JobStatusProcessing {
			hasProcessing = true
		}
		results = append(results, item)
	}

	derivedStatus := parent.Status
	if hasPending {
		derivedStatus = models.JobStatusPending
	} else if hasProcessing {
		derivedStatus = models.JobStatusProcessing
	} else if len(results) > 0 {
		derivedStatus = models.JobStatusCompleted
	}

	resp := models.BatchJobResponse{
		ID: parent.ID, Status: derivedStatus, JobType: models.JobTypeBatchURLScraper,
		URLs: urls, Results: results, CreatedAt: parent.CreatedAt.Format(time.RFC3339),
	}
	if parent.CompletedAt != nil {
		t := parent.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &t
	}
	if parent.DurationMs > 0 {
		d := parent.DurationMs
		resp.DurationMs = &d
	}
	return c.JSON(resp)
}

type scrapeResultJSON struct {
	HTML          *string                       `json:"html,omitempty"`
	CleanedHTML   *string                       `json:"cleanedHtml,omitempty"`
	Markdown      *string                       `json:"markdown,omitempty"`
	GeneratedJson *models.GeneratedJsonResponse `json:"generatedJson,omitempty"`
	Cached        *bool                         `json:"cached,omitempty"`
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
