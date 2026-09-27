// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestMemoryStore_EvictionOnCapacity(t *testing.T) {
	ctx := context.Background()
	capacity := 5
	m := NewMemoryStoreWithCapacity(capacity)

	// Insert 10 completed jobs
	for i := 0; i < 10; i++ {
		job := JobRecord{
			ID:        fmt.Sprintf("job-%d", i),
			URL:       fmt.Sprintf("https://example.com/%d", i),
			Status:    "completed",
			CreatedAt: time.Now().UTC().Add(time.Duration(i) * time.Minute),
		}
		if err := m.CreateJob(ctx, job); err != nil {
			t.Fatalf("CreateJob %d failed: %v", i, err)
		}
	}

	m.mu.RLock()
	count := len(m.jobs)
	m.mu.RUnlock()

	if count != capacity {
		t.Fatalf("expected store to cap at %d jobs, got %d", capacity, count)
	}

	// Jobs 0..4 should have been evicted; jobs 5..9 should be present
	for i := 0; i < 5; i++ {
		if _, err := m.GetJob(ctx, fmt.Sprintf("job-%d", i)); err == nil {
			t.Errorf("expected job-%d to be evicted", i)
		}
	}
	for i := 5; i < 10; i++ {
		if _, err := m.GetJob(ctx, fmt.Sprintf("job-%d", i)); err != nil {
			t.Errorf("expected job-%d to be present, got error: %v", i, err)
		}
	}
}

func TestMemoryStore_EvictionPrefersFinishedJobs(t *testing.T) {
	ctx := context.Background()
	capacity := 2
	m := NewMemoryStoreWithCapacity(capacity)

	// Insert an older processing job
	err := m.CreateJob(ctx, JobRecord{
		ID:        "active-job",
		URL:       "https://example.com/active",
		Status:    "processing",
		CreatedAt: time.Now().UTC().Add(-10 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to create active job: %v", err)
	}

	// Insert a finished job
	err = m.CreateJob(ctx, JobRecord{
		ID:        "finished-job",
		URL:       "https://example.com/finished",
		Status:    "completed",
		CreatedAt: time.Now().UTC().Add(-5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("failed to create finished job: %v", err)
	}

	// Insert a 3rd job to trigger eviction
	err = m.CreateJob(ctx, JobRecord{
		ID:        "new-job",
		URL:       "https://example.com/new",
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("failed to create new job: %v", err)
	}

	// Finished job should be evicted first even though active-job is older
	if _, err := m.GetJob(ctx, "finished-job"); err == nil {
		t.Errorf("expected finished-job to be evicted")
	}
	if _, err := m.GetJob(ctx, "active-job"); err != nil {
		t.Errorf("expected active-job to be retained")
	}
	if _, err := m.GetJob(ctx, "new-job"); err != nil {
		t.Errorf("expected new-job to be present")
	}
}
