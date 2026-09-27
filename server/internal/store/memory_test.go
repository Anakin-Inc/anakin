// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"testing"
)

func TestMemoryStore_GetChildJobs_PreservesOrder(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()

	parentID := "batch-parent-1"
	numChildren := 30
	expectedIDs := make([]string, numChildren)

	for i := 0; i < numChildren; i++ {
		childID := fmt.Sprintf("child-%02d", i)
		expectedIDs[i] = childID
		err := m.CreateJob(ctx, JobRecord{
			ID:          childID,
			ParentJobID: parentID,
			URL:         fmt.Sprintf("https://example.com/page/%d", i),
		})
		if err != nil {
			t.Fatalf("failed to create job: %v", err)
		}
	}

	// Verify that repeated calls return children in the exact same creation order
	for trial := 0; trial < 50; trial++ {
		children, err := m.GetChildJobs(ctx, parentID)
		if err != nil {
			t.Fatalf("trial %d: GetChildJobs returned error: %v", trial, err)
		}
		if len(children) != numChildren {
			t.Fatalf("trial %d: expected %d children, got %d", trial, numChildren, len(children))
		}
		for i, child := range children {
			if child.ID != expectedIDs[i] {
				t.Fatalf("trial %d: at index %d expected %s, got %s", trial, i, expectedIDs[i], child.ID)
			}
		}
	}
}
