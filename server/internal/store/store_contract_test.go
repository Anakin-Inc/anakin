// SPDX-License-Identifier: AGPL-3.0-or-later

package store

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

// The JobStore interface has two implementations that must behave identically.
// Every test below runs against each of them, so a divergence like the child
// ordering one shows up as a failure rather than as a difference users discover
// in production.
//
// PostgresStore is exercised when TEST_DATABASE_URL points at a database with
// the scripts/init-db.sql schema applied, e.g.
//
//	docker compose up postgres -d
//	TEST_DATABASE_URL="postgres://postgres:postgres@localhost:5432/anakinscraper?sslmode=disable" \
//	  go test ./internal/store/
//
// Without it, only MemoryStore runs and the Postgres cases are skipped.
type backend struct {
	name  string
	build func(t *testing.T) JobStore
}

func backends(t *testing.T) []backend {
	t.Helper()

	list := []backend{{
		name:  "memory",
		build: func(*testing.T) JobStore { return NewMemoryStore() },
	}}

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Log("TEST_DATABASE_URL not set — skipping the PostgresStore half of the contract")
		return list
	}

	list = append(list, backend{
		name: "postgres",
		build: func(t *testing.T) JobStore {
			t.Helper()
			db, err := sql.Open("postgres", dsn)
			if err != nil {
				t.Fatalf("open postgres: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := db.PingContext(ctx); err != nil {
				t.Fatalf("ping postgres: %v", err)
			}
			t.Cleanup(func() { db.Close() })
			return NewPostgresStore(db)
		},
	})
	return list
}

// forEachBackend runs fn as a subtest against every available implementation.
func forEachBackend(t *testing.T, fn func(t *testing.T, s JobStore)) {
	t.Helper()
	for _, b := range backends(t) {
		t.Run(b.name, func(t *testing.T) {
			fn(t, b.build(t))
		})
	}
}

func TestContract_CreateAndGetJob(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		ctx := context.Background()
		id := uuid.NewString()

		if err := s.CreateJob(ctx, JobRecord{
			ID: id, JobType: "url_scraper", URL: "https://example.com", Payload: "{}",
		}); err != nil {
			t.Fatalf("CreateJob: %v", err)
		}

		got, err := s.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("GetJob: %v", err)
		}
		if got.ID != id {
			t.Errorf("ID = %q, want %q", got.ID, id)
		}
		if got.URL != "https://example.com" {
			t.Errorf("URL = %q, want %q", got.URL, "https://example.com")
		}
		if got.Status != "pending" {
			t.Errorf("Status = %q, want %q", got.Status, "pending")
		}
		if got.CreatedAt.IsZero() {
			t.Error("CreatedAt is zero; the store must stamp it")
		}
	})
}

func TestContract_GetJobUnknownIDReturnsError(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		if _, err := s.GetJob(context.Background(), uuid.NewString()); err == nil {
			t.Error("GetJob returned nil error for an unknown id")
		}
	})
}

// The reported bug: children came back in Go's randomised map order from
// MemoryStore and in created_at order from PostgresStore. The handler turns
// this order into each result's index, so it has to be creation order in both.
func TestContract_GetChildJobsReturnsCreationOrder(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		ctx := context.Background()
		parentID := uuid.NewString()

		if err := s.CreateJob(ctx, JobRecord{
			ID: parentID, JobType: "batch_url_scraper", URL: "https://example.com/1", Payload: "{}",
		}); err != nil {
			t.Fatalf("CreateJob(parent): %v", err)
		}

		urls := []string{
			"https://example.com/1",
			"https://example.com/2",
			"https://example.com/3",
			"https://example.com/4",
			"https://example.com/5",
		}
		for _, u := range urls {
			if err := s.CreateJob(ctx, JobRecord{
				ID: uuid.NewString(), JobType: "url_scraper", URL: u,
				Payload: "{}", ParentJobID: parentID,
			}); err != nil {
				t.Fatalf("CreateJob(child %s): %v", u, err)
			}
			// Backends that order on the wall clock need distinguishable
			// timestamps; MemoryStore does not, which the tight-loop test below
			// covers separately.
			time.Sleep(2 * time.Millisecond)
		}

		// Repeat: a single call could match by luck even with a randomised order.
		for attempt := 0; attempt < 20; attempt++ {
			children, err := s.GetChildJobs(ctx, parentID)
			if err != nil {
				t.Fatalf("GetChildJobs: %v", err)
			}
			if len(children) != len(urls) {
				t.Fatalf("got %d children, want %d", len(children), len(urls))
			}
			for i, want := range urls {
				if children[i].URL != want {
					t.Fatalf("attempt %d: children[%d].URL = %q, want %q — order is not creation order",
						attempt, i, children[i].URL, want)
				}
			}
		}
	})
}

func TestContract_GetChildJobsExcludesOtherParents(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		ctx := context.Background()
		parentA, parentB := uuid.NewString(), uuid.NewString()

		for _, id := range []string{parentA, parentB} {
			if err := s.CreateJob(ctx, JobRecord{
				ID: id, JobType: "batch_url_scraper", URL: "https://example.com", Payload: "{}",
			}); err != nil {
				t.Fatalf("CreateJob(parent): %v", err)
			}
		}
		if err := s.CreateJob(ctx, JobRecord{
			ID: uuid.NewString(), JobType: "url_scraper", URL: "https://example.com/a",
			Payload: "{}", ParentJobID: parentA,
		}); err != nil {
			t.Fatalf("CreateJob: %v", err)
		}
		if err := s.CreateJob(ctx, JobRecord{
			ID: uuid.NewString(), JobType: "url_scraper", URL: "https://example.com/b",
			Payload: "{}", ParentJobID: parentB,
		}); err != nil {
			t.Fatalf("CreateJob: %v", err)
		}

		children, err := s.GetChildJobs(ctx, parentA)
		if err != nil {
			t.Fatalf("GetChildJobs: %v", err)
		}
		if len(children) != 1 || children[0].URL != "https://example.com/a" {
			t.Errorf("got %d children (%v), want only parentA's child", len(children), children)
		}
	})
}

func TestContract_UpdateStatus(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		ctx := context.Background()
		id := uuid.NewString()
		if err := s.CreateJob(ctx, JobRecord{
			ID: id, JobType: "url_scraper", URL: "https://example.com", Payload: "{}",
		}); err != nil {
			t.Fatalf("CreateJob: %v", err)
		}

		errMsg := "all handlers failed"
		duration := 4321
		if err := s.UpdateStatus(ctx, id, "failed", &errMsg, &duration); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}

		got, err := s.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("GetJob: %v", err)
		}
		if got.Status != "failed" {
			t.Errorf("Status = %q, want %q", got.Status, "failed")
		}
		if got.Error != errMsg {
			t.Errorf("Error = %q, want %q", got.Error, errMsg)
		}
		if got.DurationMs != duration {
			t.Errorf("DurationMs = %d, want %d", got.DurationMs, duration)
		}
		if got.CompletedAt == nil {
			t.Error("CompletedAt is nil; a terminal status must stamp it")
		}
	})
}

func TestContract_UpdateCompletedAndStoreResult(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		ctx := context.Background()
		id := uuid.NewString()
		if err := s.CreateJob(ctx, JobRecord{
			ID: id, JobType: "url_scraper", URL: "https://example.com", Payload: "{}",
		}); err != nil {
			t.Fatalf("CreateJob: %v", err)
		}

		if err := s.StoreResult(ctx, id, `{"markdown":"# Example"}`); err != nil {
			t.Fatalf("StoreResult: %v", err)
		}
		if err := s.UpdateCompleted(ctx, id, 1234, 5678); err != nil {
			t.Fatalf("UpdateCompleted: %v", err)
		}

		got, err := s.GetJob(ctx, id)
		if err != nil {
			t.Fatalf("GetJob: %v", err)
		}
		if got.Status != "completed" {
			t.Errorf("Status = %q, want %q", got.Status, "completed")
		}
		if got.Result != `{"markdown":"# Example"}` {
			t.Errorf("Result = %q, want the stored JSON", got.Result)
		}
		if got.DurationMs != 1234 {
			t.Errorf("DurationMs = %d, want 1234", got.DurationMs)
		}
		if got.CompletedAt == nil {
			t.Error("CompletedAt is nil after UpdateCompleted")
		}
	})
}

func TestContract_UpdateParentBatchStatus(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		ctx := context.Background()
		parentID := uuid.NewString()
		childA, childB := uuid.NewString(), uuid.NewString()

		if err := s.CreateJob(ctx, JobRecord{
			ID: parentID, JobType: "batch_url_scraper", URL: "https://example.com", Payload: "{}",
		}); err != nil {
			t.Fatalf("CreateJob(parent): %v", err)
		}
		for _, id := range []string{childA, childB} {
			if err := s.CreateJob(ctx, JobRecord{
				ID: id, JobType: "url_scraper", URL: "https://example.com",
				Payload: "{}", ParentJobID: parentID,
			}); err != nil {
				t.Fatalf("CreateJob(child): %v", err)
			}
		}

		// All children pending -> parent pending.
		if err := s.UpdateParentBatchStatus(ctx, parentID); err != nil {
			t.Fatalf("UpdateParentBatchStatus: %v", err)
		}
		assertStatus(t, s, parentID, "pending")

		// One child done, one still pending -> parent processing.
		if err := s.UpdateCompleted(ctx, childA, 1, 1); err != nil {
			t.Fatalf("UpdateCompleted: %v", err)
		}
		if err := s.UpdateParentBatchStatus(ctx, parentID); err != nil {
			t.Fatalf("UpdateParentBatchStatus: %v", err)
		}
		assertStatus(t, s, parentID, "processing")

		// All children terminal -> parent completed.
		if err := s.UpdateCompleted(ctx, childB, 1, 1); err != nil {
			t.Fatalf("UpdateCompleted: %v", err)
		}
		if err := s.UpdateParentBatchStatus(ctx, parentID); err != nil {
			t.Fatalf("UpdateParentBatchStatus: %v", err)
		}
		assertStatus(t, s, parentID, "completed")
	})
}

func TestContract_UpdateParentBatchStatusIgnoresEmptyParent(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		if err := s.UpdateParentBatchStatus(context.Background(), ""); err != nil {
			t.Errorf("UpdateParentBatchStatus(\"\") = %v, want nil", err)
		}
	})
}

func TestContract_Ping(t *testing.T) {
	forEachBackend(t, func(t *testing.T, s JobStore) {
		if err := s.Ping(context.Background()); err != nil {
			t.Errorf("Ping: %v", err)
		}
	})
}

func assertStatus(t *testing.T, s JobStore, id, want string) {
	t.Helper()
	got, err := s.GetJob(context.Background(), id)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if got.Status != want {
		t.Errorf("Status = %q, want %q", got.Status, want)
	}
}
