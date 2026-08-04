// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// completedJob inserts a job already in a terminal state, which is what makes
// it eligible for eviction.
func completedJob(t *testing.T, s *MemoryStore, id, parentID string) {
	t.Helper()
	ctx := context.Background()
	if err := s.CreateJob(ctx, JobRecord{
		ID: id, JobType: "url_scraper", URL: "https://example.com/" + id, ParentJobID: parentID,
	}); err != nil {
		t.Fatalf("CreateJob(%s): %v", id, err)
	}
	if err := s.UpdateCompleted(ctx, id, 1, 1); err != nil {
		t.Fatalf("UpdateCompleted(%s): %v", id, err)
	}
}

func (m *MemoryStore) size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.jobs)
}

// The reported bug: nothing was ever removed, so the map grew for the life of
// the process. On the previous implementation this ends at 500 entries.
func TestMemoryStore_EvictsOldestOnceOverLimit(t *testing.T) {
	s := NewMemoryStoreWithLimit(20)

	for i := 0; i < 500; i++ {
		completedJob(t, s, fmt.Sprintf("job-%03d", i), "")
	}

	if got := s.size(); got > 20 {
		t.Errorf("retained %d jobs, want at most the 20 limit", got)
	}
	if s.size() == 0 {
		t.Fatal("everything was evicted")
	}

	// The most recent job must survive: eviction is oldest-first, and a sync
	// request polls for its own job right after it completes.
	if _, err := s.GetJob(context.Background(), "job-499"); err != nil {
		t.Errorf("most recent job was evicted: %v", err)
	}
	// And the oldest must be gone.
	if _, err := s.GetJob(context.Background(), "job-000"); err == nil {
		t.Error("oldest job survived; eviction is not oldest-first")
	}
}

// Retention has to hold the payload down, not just the entry count — the whole
// point is bounding memory.
func TestMemoryStore_EvictionReleasesStoredResults(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStoreWithLimit(10)
	body := strings.Repeat("x", 64*1024)

	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("job-%03d", i)
		completedJob(t, s, id, "")
		if err := s.StoreResult(ctx, id, body); err != nil {
			t.Fatalf("StoreResult: %v", err)
		}
	}

	s.mu.RLock()
	var retainedBytes int
	for _, j := range s.jobs {
		retainedBytes += len(j.rec.Result)
	}
	s.mu.RUnlock()

	// 200 jobs x 64 KB is ~12.8 MB unbounded; the cap should hold it near
	// 10 x 64 KB.
	if maxBytes := 20 * len(body); retainedBytes > maxBytes {
		t.Errorf("retained %d bytes of results, want at most %d", retainedBytes, maxBytes)
	}
}

// A parent and its children are evicted as a unit. Dropping only part of a
// batch would either strand children or silently shrink the batch response.
func TestMemoryStore_EvictsBatchFamiliesTogether(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStoreWithLimit(20)

	// One batch of 5, then enough standalone jobs to force eviction.
	completedJob(t, s, "batch-parent", "")
	for i := 0; i < 5; i++ {
		completedJob(t, s, fmt.Sprintf("batch-child-%d", i), "batch-parent")
	}
	for i := 0; i < 200; i++ {
		completedJob(t, s, fmt.Sprintf("filler-%03d", i), "")
	}

	_, parentErr := s.GetJob(ctx, "batch-parent")
	children, err := s.GetChildJobs(ctx, "batch-parent")
	if err != nil {
		t.Fatalf("GetChildJobs: %v", err)
	}

	if parentErr == nil && len(children) != 5 {
		t.Errorf("parent survived with %d of 5 children; the family was split", len(children))
	}
	if parentErr != nil && len(children) != 0 {
		t.Errorf("parent was evicted but %d children were stranded", len(children))
	}
}

// In-flight jobs are still being written to by a worker and may be polled by a
// sync request, so they are never evicted even when that means exceeding the
// cap.
func TestMemoryStore_NeverEvictsInFlightJobs(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStoreWithLimit(10)

	for i := 0; i < 50; i++ {
		id := fmt.Sprintf("pending-%02d", i)
		if err := s.CreateJob(ctx, JobRecord{ID: id, JobType: "url_scraper", URL: "https://example.com"}); err != nil {
			t.Fatalf("CreateJob: %v", err)
		}
	}

	for i := 0; i < 50; i++ {
		if _, err := s.GetJob(ctx, fmt.Sprintf("pending-%02d", i)); err != nil {
			t.Fatalf("in-flight job pending-%02d was evicted: %v", i, err)
		}
	}

	// Once they finish, they become evictable and the cap is enforced again.
	for i := 0; i < 50; i++ {
		if err := s.UpdateCompleted(ctx, fmt.Sprintf("pending-%02d", i), 1, 1); err != nil {
			t.Fatalf("UpdateCompleted: %v", err)
		}
	}
	for i := 0; i < 20; i++ {
		completedJob(t, s, fmt.Sprintf("after-%02d", i), "")
	}
	if got := s.size(); got > 10 {
		t.Errorf("retained %d jobs after the in-flight ones completed, want at most 10", got)
	}
}

// Zero or less restores the previous unbounded behaviour, for anyone who was
// relying on it.
func TestMemoryStore_LimitZeroDisablesEviction(t *testing.T) {
	s := NewMemoryStoreWithLimit(0)
	for i := 0; i < 300; i++ {
		completedJob(t, s, fmt.Sprintf("job-%03d", i), "")
	}
	if got := s.size(); got != 300 {
		t.Errorf("retained %d jobs, want all 300 with eviction disabled", got)
	}
}

func TestMemoryStore_DefaultConstructorIsBounded(t *testing.T) {
	s := NewMemoryStore()
	if s.maxJobs != DefaultMaxJobs {
		t.Errorf("maxJobs = %d, want DefaultMaxJobs (%d)", s.maxJobs, DefaultMaxJobs)
	}
}
