// SPDX-License-Identifier: AGPL-3.0-or-later

package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/converter"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/domain"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/gemini"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/handler"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/proxy"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/store"
	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/telemetry"
)

// Processor handles individual scraping jobs.
type Processor struct {
	store        store.JobStore
	chain        *handler.Chain
	domainCache  *domain.Cache
	proxyPool    *proxy.Pool
	detector     *domain.Detector
	geminiClient *gemini.Client
	telemetry    *telemetry.Collector
}

// NewProcessor creates a new job processor.
// domainCache, proxyPool, geminiClient, and tel are optional (can be nil).
func NewProcessor(s store.JobStore, chain *handler.Chain, domainCache *domain.Cache, proxyPool *proxy.Pool, geminiClient *gemini.Client, tel *telemetry.Collector) *Processor {
	var det *domain.Detector
	if domainCache != nil {
		det = domain.NewDetector()
	}
	return &Processor{
		store:        s,
		chain:        chain,
		domainCache:  domainCache,
		proxyPool:    proxyPool,
		detector:     det,
		geminiClient: geminiClient,
		telemetry:    tel,
	}
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

	if err := p.store.UpdateStatus(ctx, msg.JobID, models.JobStatusProcessing, nil, nil); err != nil {
		slog.Error("failed to update job to processing", "job_id", msg.JobID, "error", err)
	}

	err := p.processScrapeJob(ctx, msg, start)

	if err != nil {
		return p.handleFailure(ctx, msg, start, err)
	}

	if parentErr := p.store.UpdateParentBatchStatus(ctx, msg.ParentJobID); parentErr != nil {
		slog.Warn("failed to update parent batch status", "parent_job_id", msg.ParentJobID, "error", parentErr)
	}

	return nil
}

func (p *Processor) processScrapeJob(ctx context.Context, msg models.JobMessage, start time.Time) error {
	// Look up domain config
	var domainCfg *domain.DomainConfig
	if p.domainCache != nil {
		domainCfg = p.domainCache.GetConfig(msg.URL)
	}

	// Check if domain is blocked
	if domainCfg != nil && domainCfg.Blocked {
		reason := "domain is blocked"
		if domainCfg.BlockedReason != "" {
			reason += ": " + domainCfg.BlockedReason
		}
		return fmt.Errorf("%s", reason)
	}

	// Build handler request
	req := p.buildHandlerRequest(msg, msg.URL)

	// Apply domain config to request
	if domainCfg != nil && domainCfg.IsEnabled {
		if len(domainCfg.HandlerChain) > 0 {
			req.AllowedHandlers = domainCfg.HandlerChain
		}
		if domainCfg.RequestTimeoutMs > 0 {
			req.Timeout = time.Duration(domainCfg.RequestTimeoutMs) * time.Millisecond
		}
		if len(domainCfg.CustomHeaders) > 0 {
			req.CustomHeaders = domainCfg.CustomHeaders
		}
		if domainCfg.CustomUserAgent != "" {
			req.CustomUserAgent = domainCfg.CustomUserAgent
		}
		if domainCfg.ProxyURL != "" {
			req.ProxyURL = domainCfg.ProxyURL
		}
	}

	// Select proxy via Thompson Sampling (if no domain-specific proxy set)
	targetHost := domain.ExtractHost(msg.URL)
	if req.ProxyURL == "" && p.proxyPool != nil {
		req.ProxyURL = p.proxyPool.SelectProxy(targetHost)
	}

	// Execute with retries
	maxRetries := 1
	if domainCfg != nil && domainCfg.MaxRetries > 0 {
		maxRetries = domainCfg.MaxRetries
	}

	var result *models.ScrapeResult
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			slog.Debug("retrying scrape", "job_id", msg.JobID, "attempt", attempt)
		}

		var err error
		result, err = p.chain.Execute(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}

		// Content validation via failure detector
		if domainCfg != nil && p.detector != nil {
			detection := p.detector.Check(domainCfg, result.HTML)
			if detection.Failed {
				slog.Warn("content validation failed",
					"job_id", msg.JobID,
					"reason", detection.Reason,
					"attempt", attempt,
				)
				if detection.ShouldRetry {
					lastErr = fmt.Errorf("content validation: %s", detection.Reason)
					continue
				}
				lastErr = fmt.Errorf("content validation: %s", detection.Reason)
				break
			}
		}

		// Success
		lastErr = nil
		break
	}

	// Report proxy result
	if req.ProxyURL != "" && p.proxyPool != nil {
		if lastErr != nil {
			isBlocked := result != nil && result.StatusCode == 403
			p.proxyPool.RecordFailure(req.ProxyURL, targetHost, isBlocked)
		} else if result != nil {
			p.proxyPool.RecordSuccess(req.ProxyURL, targetHost, result.DurationMs)
		}
	}

	if lastErr != nil {
		return lastErr
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

	// Generate structured JSON (optional — requires GEMINI_API_KEY)
	var generatedJSON *models.GeneratedJsonResponse
	if msg.GenerateJson && p.geminiClient != nil && p.geminiClient.IsEnabled() {
		jsonResult, _, jsonErr := p.geminiClient.ExtractJSONFromMarkdown(ctx, result.Markdown, msg.URL)
		if jsonErr != nil {
			slog.Warn("JSON generation failed (non-blocking)", "job_id", msg.JobID, "error", jsonErr)
			generatedJSON = &models.GeneratedJsonResponse{
				Status: models.JsonStatusFailed,
			}
		} else if jsonResult != nil {
			generatedJSON = &models.GeneratedJsonResponse{
				Status: models.JsonStatusSuccess,
				Data:   []byte(*jsonResult),
			}
		}
	}

	duration := int(time.Since(start).Milliseconds())
	completedAt := time.Now().UTC().Format(time.RFC3339)
	cached := result.Cached

	response := models.JobStatusResponse{
		ID:            msg.JobID,
		Status:        models.JobStatusCompleted,
		URL:           msg.URL,
		JobType:       msg.JobType,
		HTML:          &result.HTML,
		CleanedHTML:   &result.CleanedHTML,
		Markdown:      &result.Markdown,
		GeneratedJson: generatedJSON,
		Cached:        &cached,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		CompletedAt:   &completedAt,
		DurationMs:    &duration,
	}

	resultData, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	if err := p.store.StoreResult(ctx, msg.JobID, string(resultData)); err != nil {
		slog.Error("failed to store result", "job_id", msg.JobID, "error", err)
		return fmt.Errorf("failed to store result: %w", err)
	}

	if err := p.store.UpdateCompleted(ctx, msg.JobID, duration, len(result.HTML)); err != nil {
		slog.Error("failed to update job in database", "job_id", msg.JobID, "error", err)
		return fmt.Errorf("failed to update job: %w", err)
	}

	slog.Info("job completed",
		"job_id", msg.JobID,
		"handler", result.Handler,
		"duration_ms", duration,
		"html_length", len(result.HTML),
	)

	p.telemetry.Record(telemetry.Event{
		Endpoint:   telemetryEndpoint(msg),
		Handler:    result.Handler,
		Status:     "success",
		DurationMs: duration,
	})

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
	errMsg := jobErr.Error() + hostedHint(jobErr.Error())
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

	failData, _ := json.Marshal(response)
	if err := p.store.StoreResult(ctx, msg.JobID, string(failData)); err != nil {
		slog.Error("failed to store failure", "job_id", msg.JobID, "error", err)
	}

	if err := p.store.UpdateStatus(ctx, msg.JobID, models.JobStatusFailed, &errMsg, &duration); err != nil {
		slog.Error("failed to update failed job in database", "job_id", msg.JobID, "error", err)
	}

	if parentErr := p.store.UpdateParentBatchStatus(ctx, msg.ParentJobID); parentErr != nil {
		slog.Warn("failed to update parent batch status", "parent_job_id", msg.ParentJobID, "error", parentErr)
	}

	p.telemetry.Record(telemetry.Event{
		Endpoint:     telemetryEndpoint(msg),
		Status:       "failed",
		DurationMs:   duration,
		FailedDomain: domain.ExtractHost(msg.URL),
	})

	return jobErr
}

// telemetryEndpoint maps a job message to a telemetry endpoint name.
func telemetryEndpoint(msg models.JobMessage) string {
	if msg.ParentJobID != "" || msg.JobType == models.JobTypeBatchURLScraper {
		return "scrape_batch"
	}
	if msg.SyncRequest {
		return "scrape_sync"
	}
	return "scrape_async"
}

// hostedHint returns an anakin.io upsell hint based on the error message.
// Returns an empty string if DISABLE_HOSTED_HINTS is set to "true" or "1".
func hostedHint(errMsg string) string {
	if v := os.Getenv("DISABLE_HOSTED_HINTS"); v == "true" || v == "1" {
		return ""
	}
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "403") || strings.Contains(lower, "blocked"):
		return " | Tip: anakin.io handles blocked sites with geo-proxies in 195 countries"
	case strings.Contains(lower, "timeout"):
		return " | Tip: anakin.io offers auto-scaling for faster scrapes"
	default:
		return " | Tip: try anakin.io for managed scraping with zero infrastructure"
	}
}
