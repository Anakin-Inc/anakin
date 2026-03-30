// SPDX-License-Identifier: AGPL-3.0-or-later

package telemetry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecord_IncrementCounters(t *testing.T) {
	c := &Collector{enabled: true, startedAt: time.Now()}

	c.Record(Event{Endpoint: "scrape_sync", Handler: "http", Status: "success", DurationMs: 500})
	c.Record(Event{Endpoint: "scrape_async", Handler: "browser", Status: "failed", DurationMs: 3000})
	c.Record(Event{Endpoint: "scrape_batch", Handler: "http", Status: "success", DurationMs: 15000})
	c.Record(Event{Endpoint: "scrape_sync", Handler: "browser", Status: "success", DurationMs: 45000})

	if c.scrapeSync.Load() != 2 {
		t.Errorf("expected scrapeSync=2, got %d", c.scrapeSync.Load())
	}
	if c.scrapeAsync.Load() != 1 {
		t.Errorf("expected scrapeAsync=1, got %d", c.scrapeAsync.Load())
	}
	if c.scrapeBatch.Load() != 1 {
		t.Errorf("expected scrapeBatch=1, got %d", c.scrapeBatch.Load())
	}
	if c.handlerHTTP.Load() != 2 {
		t.Errorf("expected handlerHTTP=2, got %d", c.handlerHTTP.Load())
	}
	if c.handlerBrowser.Load() != 2 {
		t.Errorf("expected handlerBrowser=2, got %d", c.handlerBrowser.Load())
	}
	if c.statusSuccess.Load() != 3 {
		t.Errorf("expected statusSuccess=3, got %d", c.statusSuccess.Load())
	}
	if c.statusFailed.Load() != 1 {
		t.Errorf("expected statusFailed=1, got %d", c.statusFailed.Load())
	}
	if c.durationUnder1s.Load() != 1 {
		t.Errorf("expected durationUnder1s=1, got %d", c.durationUnder1s.Load())
	}
	if c.duration1to5s.Load() != 1 {
		t.Errorf("expected duration1to5s=1, got %d", c.duration1to5s.Load())
	}
	if c.duration5to30s.Load() != 1 {
		t.Errorf("expected duration5to30s=1, got %d", c.duration5to30s.Load())
	}
	if c.durationOver30s.Load() != 1 {
		t.Errorf("expected durationOver30s=1, got %d", c.durationOver30s.Load())
	}
}

func TestRecord_ConcurrentSafety(t *testing.T) {
	c := &Collector{enabled: true, startedAt: time.Now()}
	const goroutines = 100
	const eventsPerGoroutine = 1000

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				c.Record(Event{
					Endpoint:   "scrape_sync",
					Handler:    "http",
					Status:     "success",
					DurationMs: 500,
				})
			}
		}()
	}
	wg.Wait()

	expected := int64(goroutines * eventsPerGoroutine)
	if c.scrapeSync.Load() != expected {
		t.Errorf("expected scrapeSync=%d, got %d", expected, c.scrapeSync.Load())
	}
	if c.handlerHTTP.Load() != expected {
		t.Errorf("expected handlerHTTP=%d, got %d", expected, c.handlerHTTP.Load())
	}
	if c.statusSuccess.Load() != expected {
		t.Errorf("expected statusSuccess=%d, got %d", expected, c.statusSuccess.Load())
	}
	if c.durationUnder1s.Load() != expected {
		t.Errorf("expected durationUnder1s=%d, got %d", expected, c.durationUnder1s.Load())
	}
}

func TestRecord_NilCollector(t *testing.T) {
	var c *Collector
	// Must not panic
	c.Record(Event{Endpoint: "scrape_sync", Handler: "http", Status: "success", DurationMs: 100})
}

func TestRecord_DisabledCollector(t *testing.T) {
	c := &Collector{enabled: false, startedAt: time.Now()}
	c.Record(Event{Endpoint: "scrape_sync", Handler: "http", Status: "success", DurationMs: 100})

	if c.scrapeSync.Load() != 0 {
		t.Errorf("expected disabled collector to not increment, got %d", c.scrapeSync.Load())
	}
}

func TestSnapshot_CopiesAndResets(t *testing.T) {
	c := &Collector{
		enabled:       true,
		instanceID:    "test-uuid",
		startedAt:     time.Now(),
		geminiEnabled: true,
		proxyPoolSize: 3,
	}
	c.scrapeSync.Store(10)
	c.scrapeAsync.Store(5)
	c.handlerHTTP.Store(8)
	c.handlerBrowser.Store(7)
	c.statusSuccess.Store(12)
	c.statusFailed.Store(3)
	c.durationUnder1s.Store(6)
	c.duration1to5s.Store(4)
	c.duration5to30s.Store(3)
	c.durationOver30s.Store(2)

	p := c.snapshot(true)

	// Verify snapshot values
	if p.Endpoints.ScrapeSync != 10 {
		t.Errorf("expected ScrapeSync=10, got %d", p.Endpoints.ScrapeSync)
	}
	if p.Endpoints.ScrapeAsync != 5 {
		t.Errorf("expected ScrapeAsync=5, got %d", p.Endpoints.ScrapeAsync)
	}
	if p.Handlers.HTTP != 8 {
		t.Errorf("expected HTTP=8, got %d", p.Handlers.HTTP)
	}
	if p.Status.Success != 12 {
		t.Errorf("expected Success=12, got %d", p.Status.Success)
	}
	if p.Features.GeminiEnabled != true {
		t.Error("expected GeminiEnabled=true")
	}
	if p.Features.ProxyPoolSize != 3 {
		t.Errorf("expected ProxyPoolSize=3, got %d", p.Features.ProxyPoolSize)
	}
	if p.InstanceID != "test-uuid" {
		t.Errorf("expected InstanceID='test-uuid', got %q", p.InstanceID)
	}
	if p.Version != serverVersion {
		t.Errorf("expected Version=%q, got %q", serverVersion, p.Version)
	}

	// Verify counters were reset
	if c.scrapeSync.Load() != 0 {
		t.Errorf("expected scrapeSync reset to 0, got %d", c.scrapeSync.Load())
	}
	if c.scrapeAsync.Load() != 0 {
		t.Errorf("expected scrapeAsync reset to 0, got %d", c.scrapeAsync.Load())
	}
	if c.statusSuccess.Load() != 0 {
		t.Errorf("expected statusSuccess reset to 0, got %d", c.statusSuccess.Load())
	}
}

func TestSnapshot_PeekWithoutReset(t *testing.T) {
	c := &Collector{enabled: true, instanceID: "test-uuid", startedAt: time.Now()}
	c.scrapeSync.Store(10)

	p := c.snapshot(false)

	if p.Endpoints.ScrapeSync != 10 {
		t.Errorf("expected ScrapeSync=10, got %d", p.Endpoints.ScrapeSync)
	}
	// Counters should NOT be reset
	if c.scrapeSync.Load() != 10 {
		t.Errorf("expected scrapeSync still 10, got %d", c.scrapeSync.Load())
	}
}

func TestStatus_Enabled(t *testing.T) {
	c := &Collector{
		enabled:    true,
		instanceID: "test-uuid",
		startedAt:  time.Now(),
	}
	c.scrapeSync.Store(5)
	c.sendCount.Store(3)
	sentTime := time.Now().Add(-30 * time.Minute)
	c.lastSentAt.Store(sentTime)

	s := c.Status()

	if !s.Enabled {
		t.Error("expected Enabled=true")
	}
	if s.InstanceID != "test-uuid" {
		t.Errorf("expected InstanceID='test-uuid', got %q", s.InstanceID)
	}
	if s.SendCount != 3 {
		t.Errorf("expected SendCount=3, got %d", s.SendCount)
	}
	if s.LastSentAt == nil {
		t.Fatal("expected LastSentAt to be set")
	}
	if s.NextPayload == nil {
		t.Fatal("expected NextPayload to be set")
	}
	if s.NextPayload.Endpoints.ScrapeSync != 5 {
		t.Errorf("expected NextPayload.ScrapeSync=5, got %d", s.NextPayload.Endpoints.ScrapeSync)
	}
	// Status should NOT reset counters
	if c.scrapeSync.Load() != 5 {
		t.Errorf("expected scrapeSync still 5, got %d", c.scrapeSync.Load())
	}
}

func TestStatus_Disabled(t *testing.T) {
	c := &Collector{enabled: false}
	s := c.Status()
	if s.Enabled {
		t.Error("expected Enabled=false")
	}
	if s.InstanceID != "" {
		t.Errorf("expected empty InstanceID, got %q", s.InstanceID)
	}
}

func TestStatus_NilCollector(t *testing.T) {
	var c *Collector
	s := c.Status()
	if s.Enabled {
		t.Error("expected Enabled=false for nil collector")
	}
}

func TestStop_NilCollector(t *testing.T) {
	var c *Collector
	// Must not panic
	c.Stop()
}

func TestStop_DisabledCollector(t *testing.T) {
	c := &Collector{enabled: false, done: make(chan struct{})}
	close(c.done)
	// Must not panic
	c.Stop()
}

func TestTrySend_Success(t *testing.T) {
	var received atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p payload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("failed to decode payload: %v", err)
		}
		received.Store(p)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	c := &Collector{
		enabled:     true,
		instanceID:  "test-uuid",
		endpointURL: server.URL,
		startedAt:   time.Now(),
		client:      server.Client(),
	}
	c.scrapeSync.Store(10)
	c.statusSuccess.Store(10)

	c.trySend()

	if c.sendCount.Load() != 1 {
		t.Errorf("expected sendCount=1, got %d", c.sendCount.Load())
	}
	if c.scrapeSync.Load() != 0 {
		t.Errorf("expected counters reset after send, got %d", c.scrapeSync.Load())
	}

	p := received.Load().(payload)
	if p.Endpoints.ScrapeSync != 10 {
		t.Errorf("expected received ScrapeSync=10, got %d", p.Endpoints.ScrapeSync)
	}
	if p.InstanceID != "test-uuid" {
		t.Errorf("expected received InstanceID='test-uuid', got %q", p.InstanceID)
	}
}

func TestTrySend_SkipsEmptyPayload(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &Collector{
		enabled:     true,
		instanceID:  "test-uuid",
		endpointURL: server.URL,
		startedAt:   time.Now(),
		client:      server.Client(),
	}
	// All counters are zero

	c.trySend()

	if called.Load() {
		t.Error("expected no HTTP call for empty payload")
	}
}

func TestTrySend_NoRetryOn4xx(t *testing.T) {
	var callCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	c := &Collector{
		enabled:     true,
		instanceID:  "test-uuid",
		endpointURL: server.URL,
		startedAt:   time.Now(),
		client:      server.Client(),
	}
	c.scrapeSync.Store(5)

	c.trySend()

	if callCount.Load() != 1 {
		t.Errorf("expected exactly 1 call for 4xx (no retry), got %d", callCount.Load())
	}
}

func TestTrySend_RetriesOn5xx(t *testing.T) {
	var callCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &Collector{
		enabled:     true,
		instanceID:  "test-uuid",
		endpointURL: server.URL,
		startedAt:   time.Now(),
		client:      server.Client(),
	}
	c.scrapeSync.Store(5)

	c.trySend()

	// 1 initial + 2 retries = 3 total
	if callCount.Load() != 3 {
		t.Errorf("expected 3 calls (1 + 2 retries) for 5xx, got %d", callCount.Load())
	}
}

func TestPayload_JSONStructure(t *testing.T) {
	c := &Collector{
		enabled:       true,
		instanceID:    "550e8400-e29b-41d4-a716-446655440000",
		startedAt:     time.Now(),
		geminiEnabled: true,
		proxyPoolSize: 2,
	}
	c.scrapeSync.Store(42)
	c.handlerHTTP.Store(30)
	c.handlerBrowser.Store(12)
	c.statusSuccess.Store(38)
	c.statusFailed.Store(4)

	p := c.snapshot(false)

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	// Verify it deserializes correctly
	var decoded payload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}

	if decoded.InstanceID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("unexpected InstanceID: %q", decoded.InstanceID)
	}
	if decoded.Version != serverVersion {
		t.Errorf("unexpected Version: %q", decoded.Version)
	}
	if decoded.Endpoints.ScrapeSync != 42 {
		t.Errorf("unexpected ScrapeSync: %d", decoded.Endpoints.ScrapeSync)
	}
	if decoded.Features.GeminiEnabled != true {
		t.Error("expected GeminiEnabled=true")
	}
	if decoded.Features.ProxyPoolSize != 2 {
		t.Errorf("unexpected ProxyPoolSize: %d", decoded.Features.ProxyPoolSize)
	}

	// Verify JSON keys match expected schema
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal as map: %v", err)
	}
	requiredKeys := []string{"instance_id", "version", "uptime_hours", "sent_at", "endpoints", "handlers", "status", "duration", "features"}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing required key %q in payload", key)
		}
	}
}

func TestRecord_FailedDomains(t *testing.T) {
	c := &Collector{enabled: true, startedAt: time.Now()}

	c.Record(Event{Endpoint: "scrape_sync", Status: "failed", DurationMs: 5000, FailedDomain: "linkedin.com"})
	c.Record(Event{Endpoint: "scrape_sync", Status: "failed", DurationMs: 3000, FailedDomain: "linkedin.com"})
	c.Record(Event{Endpoint: "scrape_async", Status: "failed", DurationMs: 2000, FailedDomain: "glassdoor.com"})
	c.Record(Event{Endpoint: "scrape_sync", Status: "success", DurationMs: 500}) // success — no FailedDomain

	p := c.snapshot(false)
	if p.FailedDomains == nil {
		t.Fatal("expected FailedDomains to be set")
	}
	if p.FailedDomains["linkedin.com"] != 2 {
		t.Errorf("expected linkedin.com=2, got %d", p.FailedDomains["linkedin.com"])
	}
	if p.FailedDomains["glassdoor.com"] != 1 {
		t.Errorf("expected glassdoor.com=1, got %d", p.FailedDomains["glassdoor.com"])
	}
}

func TestRecord_FailedDomains_ResetOnSnapshot(t *testing.T) {
	c := &Collector{enabled: true, startedAt: time.Now()}

	c.Record(Event{Endpoint: "scrape_sync", Status: "failed", DurationMs: 1000, FailedDomain: "example.com"})

	p := c.snapshot(true) // reset
	if p.FailedDomains["example.com"] != 1 {
		t.Errorf("expected example.com=1 in snapshot, got %d", p.FailedDomains["example.com"])
	}

	// After reset, should be empty
	p2 := c.snapshot(false)
	if len(p2.FailedDomains) != 0 {
		t.Errorf("expected empty FailedDomains after reset, got %v", p2.FailedDomains)
	}
}

func TestRecord_FailedDomains_NotSetOnSuccess(t *testing.T) {
	c := &Collector{enabled: true, startedAt: time.Now()}

	c.Record(Event{Endpoint: "scrape_sync", Status: "success", DurationMs: 100})

	p := c.snapshot(false)
	if len(p.FailedDomains) != 0 {
		t.Errorf("expected no FailedDomains for success-only events, got %v", p.FailedDomains)
	}
}

func TestRecord_FailedDomains_OmittedFromJSON(t *testing.T) {
	c := &Collector{enabled: true, instanceID: "test", startedAt: time.Now()}
	c.Record(Event{Endpoint: "scrape_sync", Status: "success", DurationMs: 100})

	p := c.snapshot(false)
	data, _ := json.Marshal(p)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	if _, exists := raw["failed_domains"]; exists {
		t.Error("expected failed_domains to be omitted from JSON when empty")
	}
}

func TestRecord_FailedDomains_InJSON(t *testing.T) {
	c := &Collector{enabled: true, instanceID: "test", startedAt: time.Now()}
	c.Record(Event{Endpoint: "scrape_sync", Status: "failed", DurationMs: 1000, FailedDomain: "blocked-site.com"})

	p := c.snapshot(false)
	data, _ := json.Marshal(p)
	var raw map[string]interface{}
	json.Unmarshal(data, &raw)

	fd, exists := raw["failed_domains"]
	if !exists {
		t.Fatal("expected failed_domains in JSON")
	}
	domains := fd.(map[string]interface{})
	if domains["blocked-site.com"] != float64(1) {
		t.Errorf("expected blocked-site.com=1, got %v", domains["blocked-site.com"])
	}
}

func TestTelemetryDisabledNoOutbound(t *testing.T) {
	var called atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := &Collector{
		enabled:     false,
		endpointURL: server.URL,
		startedAt:   time.Now(),
		client:      server.Client(),
	}

	// Record 100 events on a disabled collector
	for i := 0; i < 100; i++ {
		c.Record(Event{Endpoint: "scrape_sync", Handler: "http", Status: "success", DurationMs: 100})
	}

	if c.scrapeSync.Load() != 0 {
		t.Errorf("expected 0 events on disabled collector, got %d", c.scrapeSync.Load())
	}
	if called.Load() {
		t.Error("expected no outbound HTTP calls when telemetry is disabled")
	}
}
