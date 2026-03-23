// SPDX-License-Identifier: AGPL-3.0-or-later

// Package store provides job storage backends.
//
// Two implementations:
//   - PostgresStore: persistent storage using *sql.DB (default when DATABASE_URL is set)
//   - MemoryStore: in-memory storage for zero-config "try it" mode (no DATABASE_URL)
//
// STORAGE FLOW:
//
//	Handler ──▶ store.CreateJob()
//	Processor ──▶ store.UpdateJobStatus() / store.StoreResult()
//	Handler ──▶ store.GetJob() / store.GetBatchJob()
package store

import (
	"context"
	"time"
)

// JobRecord represents a job stored in the database or memory.
type JobRecord struct {
	ID          string
	URL         string
	JobType     string
	Status      string
	Country     string
	Payload     string
	ForceFresh  bool
	ParentJobID string
	Result      string
	Error       string
	DurationMs  int
	HTMLLength  int
	Success     bool
	CreatedAt   time.Time
	CompletedAt *time.Time
}

// JobStore is the interface for job persistence.
type JobStore interface {
	// CreateJob inserts a new job.
	CreateJob(ctx context.Context, job JobRecord) error

	// GetJob returns a job by ID.
	GetJob(ctx context.Context, id string) (*JobRecord, error)

	// GetChildJobs returns all child jobs for a parent batch job.
	GetChildJobs(ctx context.Context, parentJobID string) ([]JobRecord, error)

	// UpdateStatus updates a job's status, error, and duration.
	UpdateStatus(ctx context.Context, id, status string, errMsg *string, durationMs *int) error

	// UpdateCompleted marks a job as successfully completed.
	UpdateCompleted(ctx context.Context, id string, durationMs, htmlLength int) error

	// StoreResult stores the JSON result for a job.
	StoreResult(ctx context.Context, id string, resultJSON string) error

	// UpdateParentBatchStatus recalculates and updates a parent batch job's status.
	UpdateParentBatchStatus(ctx context.Context, parentJobID string) error

	// Ping checks if the store is healthy.
	Ping(ctx context.Context) error
}
