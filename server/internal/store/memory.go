// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory job store for zero-config "try it" mode.
// Jobs are lost on restart. Not suitable for production.
type MemoryStore struct {
	mu   sync.RWMutex
	jobs map[string]*memJob
	seq  uint64
}

// memJob pairs a stored record with the sequence number it was inserted at.
//
// GetChildJobs has to return children in creation order to match the
// ORDER BY created_at ASC that PostgresStore applies. CreatedAt alone is not a
// dependable sort key here: time.Now().UTC() strips the monotonic clock
// reading, so the stored value is wall-clock only — subject to NTP adjustment,
// and coarse enough on some platforms that batch children created in a tight
// loop share a timestamp. The sequence is assigned inside the same critical
// section as the insert, so it is exactly creation order.
type memJob struct {
	rec JobRecord
	seq uint64
}

// NewMemoryStore creates a new in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{jobs: make(map[string]*memJob)}
}

func (m *MemoryStore) CreateJob(_ context.Context, job JobRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := job // copy
	j.CreatedAt = time.Now().UTC()
	m.seq++
	m.jobs[job.ID] = &memJob{rec: j, seq: m.seq}
	return nil
}

func (m *MemoryStore) GetJob(_ context.Context, id string) (*JobRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[id]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	rec := j.rec
	return &rec, nil
}

// GetChildJobs returns the children of a batch job in creation order, matching
// PostgresStore. The caller turns this order into the index field of each
// BatchResult, so it has to be stable across repeated polls of the same batch.
func (m *MemoryStore) GetChildJobs(_ context.Context, parentJobID string) ([]JobRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var children []*memJob
	for _, j := range m.jobs {
		if j.rec.ParentJobID == parentJobID {
			children = append(children, j)
		}
	}

	// Go randomises map iteration order by design, so without this the response
	// is reshuffled on every call and index identifies nothing.
	sort.Slice(children, func(a, b int) bool {
		return children[a].seq < children[b].seq
	})

	out := make([]JobRecord, 0, len(children))
	for _, j := range children {
		out = append(out, j.rec)
	}
	return out, nil
}

func (m *MemoryStore) UpdateStatus(_ context.Context, id, status string, errMsg *string, durationMs *int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	j.rec.Status = status
	if errMsg != nil {
		j.rec.Error = *errMsg
	}
	if durationMs != nil {
		j.rec.DurationMs = *durationMs
	}
	if status == "completed" || status == "failed" {
		now := time.Now().UTC()
		j.rec.CompletedAt = &now
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
	j.rec.Status = "completed"
	j.rec.DurationMs = durationMs
	j.rec.HTMLLength = htmlLength
	j.rec.Success = true
	now := time.Now().UTC()
	j.rec.CompletedAt = &now
	return nil
}

func (m *MemoryStore) StoreResult(_ context.Context, id string, resultJSON string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok {
		return fmt.Errorf("not found")
	}
	j.rec.Result = resultJSON
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
		if j.rec.ParentJobID == parentJobID {
			total++
			switch j.rec.Status {
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
		parent.rec.Status = "pending"
	} else if pending > 0 || processing > 0 {
		parent.rec.Status = "processing"
	} else {
		parent.rec.Status = "completed"
		now := time.Now().UTC()
		parent.rec.CompletedAt = &now
		parent.rec.DurationMs = int(now.Sub(parent.rec.CreatedAt).Milliseconds())
	}
	return nil
}

func (m *MemoryStore) Ping(_ context.Context) error {
	return nil
}
