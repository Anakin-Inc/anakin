// SPDX-License-Identifier: AGPL-3.0-or-later

package telemetry

import (
	"testing"
	"time"
)

// TestRecord_ErrorCategories verifies failed events are bucketed by category
// and surfaced (then reset) in the payload.
func TestRecord_ErrorCategories(t *testing.T) {
	c := &Collector{enabled: true, startedAt: time.Now()}

	c.Record(Event{Endpoint: "scrape_sync", Status: "failed", DurationMs: 100, ErrorCategory: "timeout"})
	c.Record(Event{Endpoint: "scrape_sync", Status: "failed", DurationMs: 100, ErrorCategory: "timeout"})
	c.Record(Event{Endpoint: "scrape_async", Status: "failed", DurationMs: 100, ErrorCategory: "blocked"})

	p := c.snapshot(false) // peek, no reset
	if got := p.ErrorCategories["timeout"]; got != 2 {
		t.Errorf("timeout = %d, want 2", got)
	}
	if got := p.ErrorCategories["blocked"]; got != 1 {
		t.Errorf("blocked = %d, want 1", got)
	}

	// A reset snapshot must zero the counters.
	_ = c.snapshot(true)
	p2 := c.snapshot(false)
	if len(p2.ErrorCategories) != 0 {
		t.Errorf("expected error categories cleared after reset, got %v", p2.ErrorCategories)
	}
}

// TestRecord_ErrorCategoryEmpty verifies success events and failures without a
// category do not create buckets.
func TestRecord_ErrorCategoryEmpty(t *testing.T) {
	c := &Collector{enabled: true, startedAt: time.Now()}
	c.Record(Event{Endpoint: "scrape_sync", Status: "success", DurationMs: 100})
	c.Record(Event{Endpoint: "scrape_sync", Status: "failed", DurationMs: 100}) // no category

	p := c.snapshot(false)
	if len(p.ErrorCategories) != 0 {
		t.Errorf("expected no error categories, got %v", p.ErrorCategories)
	}
}
