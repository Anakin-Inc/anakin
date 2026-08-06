// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// Batch children are created in a tight loop with no delay
// (http/handlers/scraper.go), so several can land on the same CreatedAt on
// platforms with coarse wall-clock resolution. Ordering has to hold anyway,
// which is why MemoryStore sorts on its insertion sequence rather than on the
// timestamp.
//
// This is the direct regression test for the reported bug: on the previous
// map-iteration implementation the order was randomised on every call.
func TestMemoryStore_ChildOrderIsStableWithoutTimestampGaps(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	const parentID = "parent"
	if err := s.CreateJob(ctx, JobRecord{ID: parentID, JobType: "batch_url_scraper"}); err != nil {
		t.Fatalf("CreateJob(parent): %v", err)
	}

	// Eight children, created back to back with no sleep between them.
	var want []string
	for i := 0; i < 8; i++ {
		url := fmt.Sprintf("https://example.com/%d", i)
		want = append(want, url)
		if err := s.CreateJob(ctx, JobRecord{
			ID: fmt.Sprintf("child-%d", i), JobType: "url_scraper",
			URL: url, ParentJobID: parentID,
		}); err != nil {
			t.Fatalf("CreateJob(child %d): %v", i, err)
		}
	}

	// With 8 children there are 8! orderings, so repeated calls make an
	// accidental pass vanishingly unlikely.
	for attempt := 0; attempt < 50; attempt++ {
		children, err := s.GetChildJobs(ctx, parentID)
		if err != nil {
			t.Fatalf("GetChildJobs: %v", err)
		}
		if len(children) != len(want) {
			t.Fatalf("got %d children, want %d", len(children), len(want))
		}
		for i, w := range want {
			if children[i].URL != w {
				t.Fatalf("attempt %d: children[%d].URL = %q, want %q", attempt, i, children[i].URL, w)
			}
		}
	}
}

func TestMemoryStore_GetChildJobsNoChildren(t *testing.T) {
	children, err := NewMemoryStore().GetChildJobs(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("GetChildJobs: %v", err)
	}
	if len(children) != 0 {
		t.Errorf("got %d children, want 0", len(children))
	}
}

// GetJob must hand back a copy: mutating the result cannot reach stored state.
func TestMemoryStore_GetJobReturnsCopy(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()
	if err := s.CreateJob(ctx, JobRecord{ID: "job", JobType: "url_scraper", URL: "https://example.com"}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := s.GetJob(ctx, "job")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	got.Status = "tampered"

	fresh, err := s.GetJob(ctx, "job")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if fresh.Status == "tampered" {
		t.Error("GetJob returned a value aliasing stored state")
	}
}

// The store is written by the worker pool and read by request handlers at the
// same time.
func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	s := NewMemoryStore()

	const parentID = "parent"
	if err := s.CreateJob(ctx, JobRecord{ID: parentID, JobType: "batch_url_scraper"}); err != nil {
		t.Fatalf("CreateJob(parent): %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				id := fmt.Sprintf("child-%d-%d", worker, j)
				_ = s.CreateJob(ctx, JobRecord{
					ID: id, JobType: "url_scraper",
					URL: "https://example.com", ParentJobID: parentID,
				})
				_ = s.StoreResult(ctx, id, `{"markdown":"x"}`)
				_ = s.UpdateCompleted(ctx, id, 1, 1)
				_, _ = s.GetJob(ctx, id)
				_, _ = s.GetChildJobs(ctx, parentID)
				_ = s.UpdateParentBatchStatus(ctx, parentID)
			}
		}(i)
	}
	wg.Wait()
}
