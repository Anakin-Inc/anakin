// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DefaultMemoryStoreMaxJobs is the default maximum number of jobs retained in memory.
const DefaultMemoryStoreMaxJobs = 1000

// MemoryStore is an in-memory job store for zero-config "try it" mode.
// Jobs are lost on restart. Not suitable for production.
type MemoryStore struct {
	mu      sync.RWMutex
	maxJobs int
	jobs    map[string]*JobRecord
}

// NewMemoryStore creates a new in-memory store with default capacity.
func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithCapacity(DefaultMemoryStoreMaxJobs)
}

// NewMemoryStoreWithCapacity creates a new in-memory store with specified max jobs capacity.
func NewMemoryStoreWithCapacity(maxJobs int) *MemoryStore {
	if maxJobs <= 0 {
		maxJobs = DefaultMemoryStoreMaxJobs
	}
	return &MemoryStore{
		maxJobs: maxJobs,
		jobs:    make(map[string]*JobRecord),
	}
}

// evictOldestLocked evicts the oldest job, prioritizing completed or failed jobs first.
func (m *MemoryStore) evictOldestLocked() {
	var oldestID string
	var oldestTime time.Time
	var oldestFinishedID string
	var oldestFinishedTime time.Time

	for id, j := range m.jobs {
		if oldestID == "" || j.CreatedAt.Before(oldestTime) {
			oldestTime = j.CreatedAt
			oldestID = id
		}
		if (j.Status == "completed" || j.Status == "failed") && (oldestFinishedID == "" || j.CreatedAt.Before(oldestFinishedTime)) {
			oldestFinishedTime = j.CreatedAt
			oldestFinishedID = id
		}
	}

	target := oldestFinishedID
	if target == "" {
		target = oldestID
	}
	if target != "" {
		delete(m.jobs, target)
	}
}

func (m *MemoryStore) CreateJob(_ context.Context, job JobRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.jobs) >= m.maxJobs {
		m.evictOldestLocked()
	}
	j := job // copy
	j.CreatedAt = time.Now().UTC()
	m.jobs[job.ID] = &j
	return nil
}

func (m *MemoryStore) GetJob(_ context.Context, id string) (*JobRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	copy := *j
	return &copy, nil
}

func (m *MemoryStore) GetChildJobs(_ context.Context, parentJobID string) ([]JobRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var children []JobRecord
	for _, j := range m.jobs {
		if j.ParentJobID == parentJobID {
			children = append(children, *j)
		}
	}
	return children, nil
}

func (m *MemoryStore) UpdateStatus(_ context.Context, id, status string, errMsg *string, durationMs *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	j.Status = status
	if errMsg != nil {
		j.Error = *errMsg
	}
	if durationMs != nil {
		j.DurationMs = *durationMs
	}
	if status == "completed" || status == "failed" {
		now := time.Now().UTC()
		j.CompletedAt = &now
	}
	return nil
}

func (m *MemoryStore) UpdateCompleted(_ context.Context, id string, durationMs, htmlLength int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	j.Status = "completed"
	j.DurationMs = durationMs
	j.HTMLLength = htmlLength
	j.Success = true
	now := time.Now().UTC()
	j.CompletedAt = &now
	return nil
}

func (m *MemoryStore) StoreResult(_ context.Context, id string, resultJSON string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	j.Result = resultJSON
	return nil
}

func (m *MemoryStore) UpdateParentBatchStatus(_ context.Context, parentJobID string) error {
	if parentJobID == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	parent, ok := m.jobs[parentJobID]
	if !ok {
		return nil
	}

	var total, pending, processing int
	for _, j := range m.jobs {
		if j.ParentJobID == parentJobID {
			total++
			switch j.Status {
			case "pending":
				pending++
			case "processing":
				processing++
			}
		}
	}
	if total == 0 {
		return nil
	}

	if pending == total {
		parent.Status = "pending"
	} else if pending > 0 || processing > 0 {
		parent.Status = "processing"
	} else {
		parent.Status = "completed"
		now := time.Now().UTC()
		parent.CompletedAt = &now
		parent.DurationMs = int(now.Sub(parent.CreatedAt).Milliseconds())
	}
	return nil
}

func (m *MemoryStore) Ping(_ context.Context) error {
	return nil
}
