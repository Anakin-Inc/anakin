// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// DefaultMaxJobs bounds how many jobs MemoryStore retains by default.
//
// Each completed job holds the serialised result — raw HTML, cleaned HTML and
// markdown, so roughly two to three copies of the page body, with raw HTML
// alone allowed up to 10 MB by the HTTP handler. A few hundred entries keeps
// the footprint bounded while still covering far more history than the
// dashboard shows.
const DefaultMaxJobs = 500

// MemoryStore is an in-memory job store for zero-config "try it" mode.
// Jobs are lost on restart. Not suitable for production.
//
// Retention is bounded: once maxJobs is exceeded, the oldest jobs are evicted.
// See evictLocked for the policy and its two caveats.
type MemoryStore struct {
	mu      sync.RWMutex
	jobs    map[string]*memJob
	seq     uint64
	maxJobs int
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

// NewMemoryStore creates a new in-memory store retaining DefaultMaxJobs jobs.
func NewMemoryStore() *MemoryStore {
	return NewMemoryStoreWithLimit(DefaultMaxJobs)
}

// NewMemoryStoreWithLimit creates a new in-memory store retaining at most
// maxJobs jobs. A value of zero or less disables eviction entirely, restoring
// the previous unbounded behaviour.
func NewMemoryStoreWithLimit(maxJobs int) *MemoryStore {
	return &MemoryStore{jobs: make(map[string]*memJob), maxJobs: maxJobs}
}

func (m *MemoryStore) CreateJob(_ context.Context, job JobRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := job // copy
	// PostgresStore's INSERT hardcodes 'pending' and ignores the caller's value
	// (postgres.go), so a new job starts pending in both backends. Without this
	// the status stayed "", which no branch of UpdateParentBatchStatus counts —
	// a fresh batch was reported completed before any child had run.
	j.Status = "pending"
	j.CreatedAt = time.Now().UTC()
	m.seq++
	m.jobs[job.ID] = &memJob{rec: j, seq: m.seq}
	m.evictLocked()
	return nil
}

// evictLocked drops the oldest jobs once the store exceeds maxJobs. Callers
// must hold the write lock.
//
// Jobs are evicted a batch family at a time — a parent together with its
// children — because dropping a parent while its children remain would strand
// them, and dropping children while the parent remains would silently shrink
// the batch response.
//
// Two deliberate limits:
//
//   - A family is only evictable once every member has reached a terminal
//     state. An in-flight job is still being written to by a worker and may be
//     polled by a sync request, so evicting it would surface as "not found".
//     maxJobs is therefore a target rather than a hard ceiling; the overshoot is
//     bounded by how many jobs can be in flight at once, which is
//     WORKER_POOL_SIZE + JOB_BUFFER_SIZE.
//   - Families are ordered by their most recently inserted member, so the newest
//     work is evicted last. A just-completed job can only be dropped before its
//     caller reads it if the store has since churned through an entire
//     eviction cycle, which at the 500ms sync poll interval means hundreds of
//     newer jobs.
func (m *MemoryStore) evictLocked() {
	if m.maxJobs <= 0 || len(m.jobs) <= m.maxJobs {
		return
	}

	// Shrink below the cap rather than exactly to it, so eviction runs once per
	// ~10% of capacity instead of on every insert once full.
	target := m.maxJobs * 9 / 10
	if target < 1 {
		target = 1
	}

	families := make(map[string]*jobFamily)
	for id, j := range m.jobs {
		key := j.rec.ParentJobID
		if key == "" {
			key = j.rec.ID
		}
		f, ok := families[key]
		if !ok {
			f = &jobFamily{evictable: true}
			families[key] = f
		}
		f.ids = append(f.ids, id)
		if j.seq > f.newestSeq {
			f.newestSeq = j.seq
		}
		if !isTerminal(j.rec.Status) {
			f.evictable = false
		}
	}

	ordered := make([]*jobFamily, 0, len(families))
	for _, f := range families {
		if f.evictable {
			ordered = append(ordered, f)
		}
	}
	sort.Slice(ordered, func(a, b int) bool { return ordered[a].newestSeq < ordered[b].newestSeq })

	evicted := 0
	for _, f := range ordered {
		if len(m.jobs) <= target {
			break
		}
		for _, id := range f.ids {
			delete(m.jobs, id)
			evicted++
		}
	}

	if evicted > 0 {
		slog.Debug("memory store evicted oldest jobs",
			"evicted", evicted, "retained", len(m.jobs), "max", m.maxJobs)
	}
}

// jobFamily is a batch parent and its children, or a single standalone job.
type jobFamily struct {
	ids       []string
	newestSeq uint64
	evictable bool
}

func isTerminal(status string) bool {
	return status == "completed" || status == "failed"
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
