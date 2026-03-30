// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// APIHandler implements ScrapingHandler by delegating to an external scraping API.
// Use this as a fallback when local HTTP and browser handlers fail, or as a
// template for integrating any third-party scraping service.
//
// HANDLER CHAIN POSITION:
//
//	HTTP (fast, local) ──fail──▶ Browser (local) ──fail──▶ API (external) ──fail──▶ ERROR
//
// BUILT-IN EXAMPLE (anakin.io):
//
//	ANAKIN_API_KEY=ak-xxx  →  adds anakin.io as chain fallback automatically
//
// CUSTOM THIRD-PARTY (copy this pattern):
//
//	To add any third-party scraping service:
//	1. Copy this file
//	2. Change Name(), requestBody(), and parseResponse() for your provider's API format
//	3. Register in main.go: chain = append(chain, NewMyHandler(apiKey))
//
// AUTH PATTERN:
//
//	API keys are passed via environment variables (same as GEMINI_API_KEY).
//	The handler receives the key at construction time — never reads env vars directly.
//	This keeps handlers testable and config centralized in config.go.
type APIHandler struct {
	name    string
	apiURL  string
	apiKey  string
	header  string // header name for the API key (e.g. "X-API-Key", "Authorization")
	client  *http.Client
	timeout time.Duration
}

// APIHandlerConfig configures an external API handler.
type APIHandlerConfig struct {
	// Name identifies this handler in logs and domain config handler_chain field.
	Name string

	// APIURL is the endpoint to POST scrape requests to.
	APIURL string

	// APIKey is the authentication credential. Handler is disabled when empty.
	APIKey string

	// AuthHeader is the HTTP header name for the API key.
	// Defaults to "X-API-Key" if empty.
	AuthHeader string

	// Timeout for the external API call. Defaults to 30s if zero.
	Timeout time.Duration
}

// NewAPIHandler creates a new external API handler.
func NewAPIHandler(cfg APIHandlerConfig) *APIHandler {
	if cfg.AuthHeader == "" {
		cfg.AuthHeader = "X-API-Key"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &APIHandler{
		name:    cfg.Name,
		apiURL:  cfg.APIURL,
		apiKey:  cfg.APIKey,
		header:  cfg.AuthHeader,
		timeout: cfg.Timeout,
		client:  &http.Client{Timeout: cfg.Timeout},
	}
}

// NewAnakinHandler creates an APIHandler pre-configured for anakin.io.
// Pass the API key from ANAKIN_API_KEY env var.
func NewAnakinHandler(apiKey string) *APIHandler {
	return NewAPIHandler(APIHandlerConfig{
		Name:       "anakin",
		APIURL:     "https://api.anakin.io/v1/scrape",
		APIKey:     apiKey,
		AuthHeader: "X-API-Key",
		Timeout:    60 * time.Second,
	})
}

func (h *APIHandler) Name() string { return h.name }

func (h *APIHandler) CanHandle(_ context.Context, _ *models.HandlerRequest) bool {
	return h.apiKey != ""
}

func (h *APIHandler) IsHealthy() bool {
	return h.apiKey != ""
}

func (h *APIHandler) Scrape(ctx context.Context, req *models.HandlerRequest) (*models.ScrapeResult, error) {
	// Build request body — matches the AnakinScraper API format.
	// Third-party handlers can override this by copying and modifying this file.
	body := map[string]interface{}{
		"url": req.URL,
	}
	if req.Country != "" {
		body["country"] = req.Country
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.apiURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set(h.header, h.apiKey)

	resp, err := h.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s API request failed: %w", h.name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s response: %w", h.name, err)
	}

	if resp.StatusCode >= 400 {
		slog.Warn("API handler returned error",
			"handler", h.name,
			"status", resp.StatusCode,
			"body", truncate(string(respBody), 200),
		)
		return nil, fmt.Errorf("%s API returned HTTP %d", h.name, resp.StatusCode)
	}

	// Parse the response — expects {"html": "...", "markdown": "...", ...}
	var result apiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		// If JSON parsing fails, treat the entire body as HTML
		return &models.ScrapeResult{
			HTML:       string(respBody),
			StatusCode: resp.StatusCode,
		}, nil
	}

	html := result.HTML
	if html == "" && result.Markdown != "" {
		// Some APIs return markdown only
		html = result.Markdown
	}
	if html == "" {
		return nil, fmt.Errorf("%s API returned empty result", h.name)
	}

	sr := &models.ScrapeResult{
		HTML:       html,
		StatusCode: resp.StatusCode,
	}
	if result.CleanedHTML != "" {
		sr.CleanedHTML = result.CleanedHTML
	}
	if result.Markdown != "" {
		sr.Markdown = result.Markdown
	}

	return sr, nil
}

// apiResponse is the expected JSON structure from external scraping APIs.
// Designed to be compatible with the AnakinScraper API response format.
// Third-party APIs that return different formats should override parseResponse.
type apiResponse struct {
	HTML        string `json:"html"`
	CleanedHTML string `json:"cleanedHtml"`
	Markdown    string `json:"markdown"`
	Status      string `json:"status"`
	Error       string `json:"error"`
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
