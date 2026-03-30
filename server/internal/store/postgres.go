// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"fmt"
)

// PostgresStore is a PostgreSQL-backed job store.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new PostgreSQL store.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// DB returns the underlying *sql.DB for components that still need direct access
// (domain configs, proxy scores, telemetry).
func (s *PostgresStore) DB() *sql.DB {
	return s.db
}

func (s *PostgresStore) CreateJob(ctx context.Context, job JobRecord) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO scrape_requests (id, job_type, url, status, country, payload, force_fresh, parent_job_id)
		 VALUES ($1, $2, $3, 'pending', $4, $5, $6, $7)`,
		job.ID, job.JobType, job.URL, job.Country, job.Payload, job.ForceFresh, nilIfEmpty(job.ParentJobID))
	return err
}

func (s *PostgresStore) GetJob(ctx context.Context, id string) (*JobRecord, error) {
	var (
		j           JobRecord
		completedAt sql.NullTime
		errorMsg    sql.NullString
		result      sql.NullString
		durationMs  sql.NullInt64
		parentJobID sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, url, job_type, status, created_at, completed_at, error, result, duration_ms, parent_job_id
		 FROM scrape_requests WHERE id = $1`, id,
	).Scan(&j.ID, &j.URL, &j.JobType, &j.Status, &j.CreatedAt, &completedAt, &errorMsg, &result, &durationMs, &parentJobID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	if err != nil {
		return nil, err
	}
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		j.Error = errorMsg.String
	}
	if result.Valid {
		j.Result = result.String
	}
	if durationMs.Valid {
		j.DurationMs = int(durationMs.Int64)
	}
	if parentJobID.Valid {
		j.ParentJobID = parentJobID.String
	}
	return &j, nil
}

func (s *PostgresStore) GetChildJobs(ctx context.Context, parentJobID string) ([]JobRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, url, status, error, result, duration_ms
		 FROM scrape_requests WHERE parent_job_id = $1 ORDER BY created_at ASC`, parentJobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []JobRecord
	for rows.Next() {
		var (
			j          JobRecord
			errorMsg   sql.NullString
			result     sql.NullString
			durationMs sql.NullInt64
		)
		if err := rows.Scan(&j.ID, &j.URL, &j.Status, &errorMsg, &result, &durationMs); err != nil {
			continue
		}
		if errorMsg.Valid {
			j.Error = errorMsg.String
		}
		if result.Valid {
			j.Result = result.String
		}
		if durationMs.Valid {
			j.DurationMs = int(durationMs.Int64)
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (s *PostgresStore) UpdateStatus(ctx context.Context, id, status string, errMsg *string, durationMs *int) error {
	query := `UPDATE scrape_requests SET status = $1, error = $2, duration_ms = $3`
	args := []interface{}{status, errMsg, durationMs}

	if status == "completed" || status == "failed" {
		query += `, completed_at = NOW()`
	}

	query += fmt.Sprintf(` WHERE id = $%d`, len(args)+1)
	args = append(args, id)

	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *PostgresStore) UpdateCompleted(ctx context.Context, id string, durationMs, htmlLength int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE scrape_requests
		 SET status = 'completed', duration_ms = $1, html_length = $2, success = true, completed_at = NOW()
		 WHERE id = $3`,
		durationMs, htmlLength, id)
	return err
}

func (s *PostgresStore) StoreResult(ctx context.Context, id string, resultJSON string) error {
	_, err := s.db.ExecContext(ctx,
		"UPDATE scrape_requests SET result = $1 WHERE id = $2", resultJSON, id)
	return err
}

func (s *PostgresStore) UpdateParentBatchStatus(ctx context.Context, parentJobID string) error {
	if parentJobID == "" {
		return nil
	}

	var total, pending, processing int
	err := s.db.QueryRowContext(ctx,
		`SELECT
			COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status = 'pending') AS pending,
			COUNT(*) FILTER (WHERE status = 'processing') AS processing
		 FROM scrape_requests WHERE parent_job_id = $1`, parentJobID,
	).Scan(&total, &pending, &processing)
	if err != nil {
		return err
	}
	if total == 0 {
		return nil
	}

	status := "completed"
	if pending == total {
		status = "pending"
	} else if pending > 0 || processing > 0 {
		status = "processing"
	}

	if status == "completed" {
		_, err = s.db.ExecContext(ctx,
			`UPDATE scrape_requests
			 SET status = $1, duration_ms = GREATEST(0, CAST(EXTRACT(EPOCH FROM (NOW() - created_at)) * 1000 AS INTEGER)), completed_at = NOW()
			 WHERE id = $2`, status, parentJobID)
		return err
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE scrape_requests SET status = $1 WHERE id = $2`, status, parentJobID)
	return err
}

func (s *PostgresStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
