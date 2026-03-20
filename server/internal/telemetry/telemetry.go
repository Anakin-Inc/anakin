// SPDX-License-Identifier: AGPL-3.0-or-later

// Package telemetry provides anonymous, privacy-first usage telemetry.
//
// DATA FLOW:
//
//	┌──────────┐    Record()     ┌───────────┐   hourly    ┌──────────────────┐
//	│Processor │───────────────▶│ Collector  │────────────▶│telemetry.anakin.io│
//	│(per job) │  O(1) atomic   │(in-memory) │  HTTP POST  │   /v1/collect     │
//	└──────────┘                └─────┬──────┘             └──────────────────┘
//	                                  │
//	                                  │ Status()
//	                                  ▼
//	                           ┌──────────────┐
//	                           │GET /v1/      │
//	                           │telemetry/    │
//	                           │status        │
//	                           └──────────────┘
//
// THREAD SAFETY: All counters use sync/atomic. No mutex needed.
// FAILURE MODE: send() failures are logged and dropped. Never blocks caller.
// DISABLE: Set TELEMETRY=off. All methods become safe no-ops.
package telemetry

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

const (
	defaultEndpoint  = "https://telemetry.anakin.io/v1/collect"
	sendInterval     = 1 * time.Hour
	sendTimeout      = 5 * time.Second
	maxRetries       = 2
	retryDelay       = 5 * time.Second
	serverVersion    = "0.1.0"
	createTableQuery = `CREATE TABLE IF NOT EXISTS telemetry_instance (
		id SERIAL PRIMARY KEY,
		instance_id UUID NOT NULL UNIQUE,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	)`
)

// Event represents a single telemetry event emitted after a job completes.
type Event struct {
	Endpoint   string // "scrape_sync", "scrape_async", "scrape_batch"
	Handler    string // "http", "browser", ""
	Status     string // "success", "failed"
	DurationMs int
}

// Collector aggregates telemetry events in memory and sends them periodically.
// All public methods are safe to call on a nil or disabled Collector.
type Collector struct {
	enabled     bool
	instanceID  string
	endpointURL string
	startedAt   time.Time

	// Atomic counters — thread-safe without mutex
	scrapeSync  atomic.Int64
	scrapeAsync atomic.Int64
	scrapeBatch atomic.Int64

	handlerHTTP    atomic.Int64
	handlerBrowser atomic.Int64

	statusSuccess atomic.Int64
	statusFailed  atomic.Int64

	durationUnder1s atomic.Int64
	duration1to5s   atomic.Int64
	duration5to30s  atomic.Int64
	durationOver30s atomic.Int64

	lastSentAt atomic.Value // time.Time
	sendCount  atomic.Int64

	// Feature flags (set once at init, read-only after)
	geminiEnabled bool
	proxyPoolSize int

	cancel context.CancelFunc
	done   chan struct{}
	client *http.Client
}

// payload is the JSON structure sent to the telemetry endpoint.
type payload struct {
	InstanceID string    `json:"instance_id"`
	Version    string    `json:"version"`
	UptimeH    float64   `json:"uptime_hours"`
	SentAt     time.Time `json:"sent_at"`

	Endpoints struct {
		ScrapeSync  int64 `json:"scrape_sync"`
		ScrapeAsync int64 `json:"scrape_async"`
		ScrapeBatch int64 `json:"scrape_batch"`
	} `json:"endpoints"`

	Handlers struct {
		HTTP    int64 `json:"http"`
		Browser int64 `json:"browser"`
	} `json:"handlers"`

	Status struct {
		Success int64 `json:"success"`
		Failed  int64 `json:"failed"`
	} `json:"status"`

	Duration struct {
		Under1s int64 `json:"under_1s"`
		From1to5s  int64 `json:"from_1s_to_5s"`
		From5to30s int64 `json:"from_5s_to_30s"`
		Over30s int64 `json:"over_30s"`
	} `json:"duration"`

	Features struct {
		GeminiEnabled bool `json:"gemini_enabled"`
		ProxyPoolSize int  `json:"proxy_pool_size"`
	} `json:"features"`
}

// StatusResponse is returned by the /v1/telemetry/status endpoint.
type StatusResponse struct {
	Enabled    bool        `json:"enabled"`
	InstanceID string      `json:"instance_id,omitempty"`
	Version    string      `json:"version,omitempty"`
	UptimeH    float64     `json:"uptime_hours,omitempty"`
	LastSentAt *time.Time  `json:"last_sent_at,omitempty"`
	SendCount  int64       `json:"send_count,omitempty"`
	NextPayload *payload   `json:"next_payload,omitempty"`
}

// New creates a Collector. If enabled is false, returns a disabled (noop) collector.
// The db connection is used to persist the instance UUID across restarts.
func New(db *sql.DB, enabled bool, endpointURL string, geminiEnabled bool, proxyPoolSize int) *Collector {
	c := &Collector{
		enabled:       enabled,
		startedAt:     time.Now(),
		geminiEnabled: geminiEnabled,
		proxyPoolSize: proxyPoolSize,
		done:          make(chan struct{}),
		client:        &http.Client{Timeout: sendTimeout},
	}

	if !enabled {
		slog.Info("telemetry disabled")
		close(c.done)
		return c
	}

	if endpointURL == "" {
		endpointURL = defaultEndpoint
	}
	c.endpointURL = endpointURL

	// Auto-create table and load/generate instance UUID
	if err := c.initDB(db); err != nil {
		slog.Error("telemetry: failed to initialize database, disabling", "error", err)
		c.enabled = false
		close(c.done)
		return c
	}

	slog.Info("telemetry enabled",
		"instance_id", c.instanceID,
		"endpoint", c.endpointURL,
	)

	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go c.loop(ctx)

	return c
}

// Record records a telemetry event. O(1) atomic operations. Safe on nil/disabled collector.
func (c *Collector) Record(e Event) {
	if c == nil || !c.enabled {
		return
	}

	switch e.Endpoint {
	case "scrape_sync":
		c.scrapeSync.Add(1)
	case "scrape_async":
		c.scrapeAsync.Add(1)
	case "scrape_batch":
		c.scrapeBatch.Add(1)
	}

	switch e.Handler {
	case "http":
		c.handlerHTTP.Add(1)
	case "browser":
		c.handlerBrowser.Add(1)
	}

	switch e.Status {
	case "success":
		c.statusSuccess.Add(1)
	case "failed":
		c.statusFailed.Add(1)
	}

	switch {
	case e.DurationMs < 1000:
		c.durationUnder1s.Add(1)
	case e.DurationMs < 5000:
		c.duration1to5s.Add(1)
	case e.DurationMs < 30000:
		c.duration5to30s.Add(1)
	default:
		c.durationOver30s.Add(1)
	}
}

// Status returns the current telemetry state for the transparency endpoint.
func (c *Collector) Status() StatusResponse {
	if c == nil || !c.enabled {
		return StatusResponse{Enabled: false}
	}

	p := c.snapshot(false) // peek without resetting
	var lastSent *time.Time
	if v := c.lastSentAt.Load(); v != nil {
		t := v.(time.Time)
		lastSent = &t
	}

	return StatusResponse{
		Enabled:     true,
		InstanceID:  c.instanceID,
		Version:     serverVersion,
		UptimeH:     time.Since(c.startedAt).Hours(),
		LastSentAt:  lastSent,
		SendCount:   c.sendCount.Load(),
		NextPayload: p,
	}
}

// Stop gracefully shuts down the collector. Attempts a final send with a 2s timeout.
func (c *Collector) Stop() {
	if c == nil || !c.enabled {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		slog.Warn("telemetry: shutdown timed out")
	}
}

// initDB creates the telemetry_instance table and loads or generates the instance UUID.
func (c *Collector) initDB(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := db.ExecContext(ctx, createTableQuery); err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	// Try to load existing UUID
	var id string
	err := db.QueryRowContext(ctx, "SELECT instance_id FROM telemetry_instance LIMIT 1").Scan(&id)
	if err == nil {
		c.instanceID = id
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("query instance_id: %w", err)
	}

	// Generate and persist new UUID
	id = uuid.New().String()
	if _, err := db.ExecContext(ctx, "INSERT INTO telemetry_instance (instance_id) VALUES ($1)", id); err != nil {
		return fmt.Errorf("insert instance_id: %w", err)
	}
	c.instanceID = id
	return nil
}

// loop runs the periodic send cycle.
func (c *Collector) loop(ctx context.Context) {
	defer close(c.done)
	ticker := time.NewTicker(sendInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Final send attempt on shutdown
			c.trySend()
			return
		case <-ticker.C:
			c.trySend()
		}
	}
}

// snapshot creates a payload from current counters. If reset is true, counters are zeroed.
func (c *Collector) snapshot(reset bool) *payload {
	p := &payload{
		InstanceID: c.instanceID,
		Version:    serverVersion,
		UptimeH:    time.Since(c.startedAt).Hours(),
		SentAt:     time.Now().UTC(),
	}

	if reset {
		p.Endpoints.ScrapeSync = c.scrapeSync.Swap(0)
		p.Endpoints.ScrapeAsync = c.scrapeAsync.Swap(0)
		p.Endpoints.ScrapeBatch = c.scrapeBatch.Swap(0)
		p.Handlers.HTTP = c.handlerHTTP.Swap(0)
		p.Handlers.Browser = c.handlerBrowser.Swap(0)
		p.Status.Success = c.statusSuccess.Swap(0)
		p.Status.Failed = c.statusFailed.Swap(0)
		p.Duration.Under1s = c.durationUnder1s.Swap(0)
		p.Duration.From1to5s = c.duration1to5s.Swap(0)
		p.Duration.From5to30s = c.duration5to30s.Swap(0)
		p.Duration.Over30s = c.durationOver30s.Swap(0)
	} else {
		p.Endpoints.ScrapeSync = c.scrapeSync.Load()
		p.Endpoints.ScrapeAsync = c.scrapeAsync.Load()
		p.Endpoints.ScrapeBatch = c.scrapeBatch.Load()
		p.Handlers.HTTP = c.handlerHTTP.Load()
		p.Handlers.Browser = c.handlerBrowser.Load()
		p.Status.Success = c.statusSuccess.Load()
		p.Status.Failed = c.statusFailed.Load()
		p.Duration.Under1s = c.durationUnder1s.Load()
		p.Duration.From1to5s = c.duration1to5s.Load()
		p.Duration.From5to30s = c.duration5to30s.Load()
		p.Duration.Over30s = c.durationOver30s.Load()
	}

	p.Features.GeminiEnabled = c.geminiEnabled
	p.Features.ProxyPoolSize = c.proxyPoolSize

	return p
}

// trySend snapshots counters and sends them to the telemetry endpoint.
func (c *Collector) trySend() {
	p := c.snapshot(true)

	// Skip send if no events were collected
	total := p.Endpoints.ScrapeSync + p.Endpoints.ScrapeAsync + p.Endpoints.ScrapeBatch
	if total == 0 {
		return
	}

	data, err := json.Marshal(p)
	if err != nil {
		slog.Error("telemetry: failed to marshal payload", "error", err, "event_count", total)
		return
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}

		resp, err := c.client.Post(c.endpointURL, "application/json", bytes.NewReader(data))
		if err != nil {
			slog.Warn("telemetry: send failed", "error", err, "attempt", attempt)
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			c.lastSentAt.Store(time.Now())
			c.sendCount.Add(1)
			slog.Debug("telemetry: sent", "events", total, "status", resp.StatusCode)
			return
		}

		// Don't retry client errors (4xx)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			slog.Warn("telemetry: endpoint returned client error",
				"status", resp.StatusCode, "events", total)
			return
		}

		// Retry server errors (5xx)
		slog.Warn("telemetry: endpoint returned server error",
			"status", resp.StatusCode, "attempt", attempt)
	}

	slog.Warn("telemetry: send failed after retries, dropping batch", "events", total)
}
