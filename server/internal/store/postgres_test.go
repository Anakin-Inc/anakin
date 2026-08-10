// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"testing"
)

type childJobRowsDriver struct{}

func (childJobRowsDriver) Open(string) (driver.Conn, error) {
	return childJobRowsConn{}, nil
}

type childJobRowsConn struct{}

func (childJobRowsConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (childJobRowsConn) Close() error { return nil }

func (childJobRowsConn) Begin() (driver.Tx, error) { return nil, driver.ErrSkip }

func (childJobRowsConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &childJobRows{}, nil
}

type childJobRows struct {
	index int
}

func (*childJobRows) Columns() []string {
	return []string{"id", "url", "status", "error", "result", "duration_ms"}
}

func (*childJobRows) Close() error { return nil }

func (r *childJobRows) Next(dest []driver.Value) error {
	rows := [][]driver.Value{
		{"child-1", "https://example.com/1", "completed", nil, nil, int64(10)},
		{"child-2", "https://example.com/2", "completed", nil, nil, "not-an-integer"},
	}
	if r.index >= len(rows) {
		return io.EOF
	}
	copy(dest, rows[r.index])
	r.index++
	return nil
}

func TestPostgresStoreGetChildJobsReturnsScanError(t *testing.T) {
	sql.Register("child_job_scan_error", childJobRowsDriver{})
	db, err := sql.Open("child_job_scan_error", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	jobs, err := NewPostgresStore(db).GetChildJobs(context.Background(), "parent-1")
	if err == nil {
		t.Fatal("GetChildJobs returned nil error for a row scan failure")
	}
	if jobs != nil {
		t.Fatalf("GetChildJobs returned partial jobs: %#v", jobs)
	}
}
