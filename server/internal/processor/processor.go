package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/AnakinAI/anakinscraper-oss/server/internal/converter"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/handler"
	"github.com/AnakinAI/anakinscraper-oss/server/internal/models"
)

// Processor handles individual scraping jobs.
type Processor struct {
	db    *sql.DB
	chain *handler.Chain
}

// NewProcessor creates a new job processor.
func NewProcessor(db *sql.DB, chain *handler.Chain) *Processor {
	return &Processor{db: db, chain: chain}
}

// ProcessJob handles a single job from the worker pool.
func (p *Processor) ProcessJob(ctx context.Context, msg models.JobMessage) error {
	start := time.Now()

	slog.Info("processing job",
		"job_id", msg.JobID,
		"url", msg.URL,
		"job_type", msg.JobType,
		"use_browser", msg.UseBrowser,
	)

	if err := p.updateJobStatus(ctx, msg.JobID, models.JobStatusProcessing, nil, nil); err != nil {
		slog.Error("failed to update job to processing", "job_id", msg.JobID, "error", err)
	}

	var err error
	switch msg.JobType {
	case models.JobTypeMap:
		err = p.processMapJob(ctx, msg, start)
	case models.JobTypeCrawl:
		err = p.processCrawlJob(ctx, msg, start)
	default:
		err = p.processScrapeJob(ctx, msg, start)
	}

	if err != nil {
		return p.handleFailure(ctx, msg, start, err)
	}

	if parentErr := p.updateParentBatchStatus(ctx, msg.ParentJobID); parentErr != nil {
		slog.Warn("failed to update parent batch status", "parent_job_id", msg.ParentJobID, "error", parentErr)
	}

	return nil
}

func (p *Processor) processScrapeJob(ctx context.Context, msg models.JobMessage, start time.Time) error {
	result, err := p.chain.Execute(ctx, p.buildHandlerRequest(msg, msg.URL))
	if err != nil {
		return err
	}

	converted, err := converter.HTMLToMarkdown(result.HTML, msg.URL)
	if err != nil {
		slog.Warn("markdown conversion failed", "job_id", msg.JobID, "error", err)
		converted = &converter.ConvertResult{
			CleanedHTML: result.HTML,
			Markdown:    result.HTML,
		}
	}

	result.CleanedHTML = converted.CleanedHTML
	result.Markdown = converted.Markdown

	duration := int(time.Since(start).Milliseconds())
	completedAt := time.Now().UTC().Format(time.RFC3339)
	cached := result.Cached

	response := models.JobStatusResponse{
		ID:          msg.JobID,
		Status:      models.JobStatusCompleted,
		URL:         msg.URL,
		JobType:     msg.JobType,
		HTML:        &result.HTML,
		CleanedHTML: &result.CleanedHTML,
		Markdown:    &result.Markdown,
		Cached:      &cached,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		CompletedAt: &completedAt,
		DurationMs:  &duration,
	}

	if err := p.storeResult(ctx, msg.JobID, response); err != nil {
		slog.Error("failed to store result", "job_id", msg.JobID, "error", err)
	}

	if err := p.updateJobCompleted(ctx, msg.JobID, duration, len(result.HTML)); err != nil {
		slog.Error("failed to update job in database", "job_id", msg.JobID, "error", err)
	}

	slog.Info("job completed",
		"job_id", msg.JobID,
		"handler", result.Handler,
		"duration_ms", duration,
		"html_length", len(result.HTML),
	)
	return nil
}

func (p *Processor) processMapJob(ctx context.Context, msg models.JobMessage, start time.Time) error {
	result, err := p.chain.Execute(ctx, p.buildHandlerRequest(msg, msg.URL))
	if err != nil {
		return err
	}

	limit := msg.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 5000 {
		limit = 5000
	}

	links := extractLinks(msg.URL, result.HTML, msg.IncludeSubdomains, msg.Search)
	if len(links) > limit {
		links = links[:limit]
	}

	duration := int(time.Since(start).Milliseconds())

	if err := p.storeResult(ctx, msg.JobID, models.MapJobResult{Links: links}); err != nil {
		slog.Error("failed to store map result", "job_id", msg.JobID, "error", err)
	}

	if err := p.updateJobCompleted(ctx, msg.JobID, duration, len(result.HTML)); err != nil {
		slog.Error("failed to update map job in database", "job_id", msg.JobID, "error", err)
	}

	return nil
}

func (p *Processor) processCrawlJob(ctx context.Context, msg models.JobMessage, start time.Time) error {
	maxPages := msg.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}
	if maxPages > 100 {
		maxPages = 100
	}

	queue := []string{msg.URL}
	visited := map[string]struct{}{}
	enqueued := map[string]struct{}{msg.URL: {}}
	results := make([]models.CrawlResult, 0, maxPages)
	completedPages := 0

	for len(queue) > 0 && len(visited) < maxPages {
		current := queue[0]
		queue = queue[1:]
		if _, done := visited[current]; done {
			continue
		}
		visited[current] = struct{}{}

		pageStart := time.Now()
		pageResult, err := p.chain.Execute(ctx, p.buildHandlerRequest(msg, current))
		pageDuration := int(time.Since(pageStart).Milliseconds())
		if err != nil {
			errMsg := err.Error()
			results = append(results, models.CrawlResult{
				URL:        current,
				Status:     models.JobStatusFailed,
				Error:      &errMsg,
				DurationMs: &pageDuration,
			})
			continue
		}

		converted, convErr := converter.HTMLToMarkdown(pageResult.HTML, current)
		var markdown string
		if convErr != nil {
			markdown = pageResult.HTML
		} else {
			markdown = converted.Markdown
		}
		completedPages++
		results = append(results, models.CrawlResult{
			URL:        current,
			Status:     models.JobStatusCompleted,
			Markdown:   &markdown,
			DurationMs: &pageDuration,
		})

		links := extractLinks(current, pageResult.HTML, false, "")
		links = filterLinksByPatterns(links, msg.IncludePatterns, msg.ExcludePatterns)
		for _, link := range links {
			if _, done := visited[link]; done {
				continue
			}
			if _, queued := enqueued[link]; queued {
				continue
			}
			enqueued[link] = struct{}{}
			queue = append(queue, link)
		}
	}

	duration := int(time.Since(start).Milliseconds())

	crawlResult := models.CrawlJobResult{
		TotalPages:     maxPages,
		CompletedPages: completedPages,
		Results:        results,
	}

	if err := p.storeResult(ctx, msg.JobID, crawlResult); err != nil {
		slog.Error("failed to store crawl result", "job_id", msg.JobID, "error", err)
	}

	if err := p.updateJobCompleted(ctx, msg.JobID, duration, 0); err != nil {
		slog.Error("failed to update crawl job in database", "job_id", msg.JobID, "error", err)
	}

	return nil
}

func (p *Processor) buildHandlerRequest(msg models.JobMessage, targetURL string) *models.HandlerRequest {
	return &models.HandlerRequest{
		JobID:      msg.JobID,
		URL:        targetURL,
		Country:    msg.Country,
		UseBrowser: msg.UseBrowser,
		Timeout:    60 * time.Second,
	}
}

func (p *Processor) handleFailure(ctx context.Context, msg models.JobMessage, start time.Time, jobErr error) error {
	duration := int(time.Since(start).Milliseconds())
	errMsg := jobErr.Error()
	completedAt := time.Now().UTC().Format(time.RFC3339)

	response := models.JobStatusResponse{
		ID:          msg.JobID,
		Status:      models.JobStatusFailed,
		URL:         msg.URL,
		JobType:     msg.JobType,
		Error:       &errMsg,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		CompletedAt: &completedAt,
		DurationMs:  &duration,
	}

	if err := p.storeResult(ctx, msg.JobID, response); err != nil {
		slog.Error("failed to store failure", "job_id", msg.JobID, "error", err)
	}

	if err := p.updateJobStatus(ctx, msg.JobID, models.JobStatusFailed, &errMsg, &duration); err != nil {
		slog.Error("failed to update failed job in database", "job_id", msg.JobID, "error", err)
	}

	if parentErr := p.updateParentBatchStatus(ctx, msg.ParentJobID); parentErr != nil {
		slog.Warn("failed to update parent batch status", "parent_job_id", msg.ParentJobID, "error", parentErr)
	}
	return jobErr
}

// storeResult serializes the result as JSON and writes it to the scrape_requests.result column.
func (p *Processor) storeResult(ctx context.Context, jobID string, result interface{}) error {
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	_, err = p.db.ExecContext(ctx, "UPDATE scrape_requests SET result = $1 WHERE id = $2", string(data), jobID)
	return err
}

func (p *Processor) updateJobStatus(ctx context.Context, jobID, status string, errMsg *string, durationMs *int) error {
	query := `UPDATE scrape_requests SET status = $1, error = $2, duration_ms = $3`
	args := []interface{}{status, errMsg, durationMs}

	if status == models.JobStatusCompleted || status == models.JobStatusFailed {
		query += `, completed_at = NOW()`
	}

	query += fmt.Sprintf(` WHERE id = $%d`, len(args)+1)
	args = append(args, jobID)

	_, err := p.db.ExecContext(ctx, query, args...)
	return err
}

func (p *Processor) updateJobCompleted(ctx context.Context, jobID string, durationMs, htmlLength int) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE scrape_requests
		 SET status = $1, duration_ms = $2, html_length = $3, success = true, completed_at = NOW()
		 WHERE id = $4`,
		models.JobStatusCompleted, durationMs, htmlLength, jobID,
	)
	return err
}

func (p *Processor) updateParentBatchStatus(ctx context.Context, parentJobID string) error {
	if parentJobID == "" {
		return nil
	}

	var total, pending, processing int
	err := p.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = $2) AS pending,
			COUNT(*) FILTER (WHERE status = $3) AS processing
		 FROM scrape_requests
		 WHERE parent_job_id = $1`,
		parentJobID, models.JobStatusPending, models.JobStatusProcessing,
	).Scan(&total, &pending, &processing)
	if err != nil {
		return err
	}
	if total == 0 {
		return nil
	}

	status := models.JobStatusCompleted
	if pending == total {
		status = models.JobStatusPending
	} else if pending > 0 || processing > 0 {
		status = models.JobStatusProcessing
	}

	if status == models.JobStatusCompleted {
		_, err = p.db.ExecContext(ctx,
			`UPDATE scrape_requests
			 SET status = $1,
			     duration_ms = GREATEST(0, CAST(EXTRACT(EPOCH FROM (NOW() - created_at)) * 1000 AS INTEGER)),
			     completed_at = NOW()
			 WHERE id = $2`,
			status, parentJobID,
		)
		return err
	}

	_, err = p.db.ExecContext(ctx,
		`UPDATE scrape_requests SET status = $1 WHERE id = $2`,
		status, parentJobID,
	)
	return err
}
