// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"testing"
	"time"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

func TestDeriveBatchStatus(t *testing.T) {
	tests := []struct {
		name                               string
		total, pending, processing, failed int
		expected                           string
	}{
		{"all failed is failed, not completed", 5, 0, 0, 5, models.JobStatusFailed},
		{"all succeeded is completed", 3, 0, 0, 0, models.JobStatusCompleted},
		{"mixed terminal is completed", 3, 0, 0, 2, models.JobStatusCompleted},
		{"nothing started yet is pending", 3, 3, 0, 0, models.JobStatusPending},
		{"some still pending is processing", 3, 1, 0, 2, models.JobStatusProcessing},
		{"any processing is processing", 3, 0, 1, 2, models.JobStatusProcessing},
		{"no children is pending", 0, 0, 0, 0, models.JobStatusPending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := models.DeriveBatchStatus(tt.total, tt.pending, tt.processing, tt.failed)
			if got != tt.expected {
				t.Errorf("DeriveBatchStatus(%d, %d, %d, %d) = %q, want %q",
					tt.total, tt.pending, tt.processing, tt.failed, got, tt.expected)
			}
		})
	}
}

func TestScrapeSyncTimeout(t *testing.T) {
	const (
		defaultTimeout = 30 * time.Second
		maxTimeout     = 120 * time.Second
	)

	resolveTimeout := func(reqTimeout int) time.Duration {
		if reqTimeout <= 0 {
			return defaultTimeout
		}
		d := time.Duration(reqTimeout) * time.Second
		if d > maxTimeout {
			return maxTimeout
		}
		return d
	}

	tests := []struct {
		name     string
		input    int
		expected time.Duration
	}{
		{"zero uses default", 0, 30 * time.Second},
		{"negative uses default", -5, 30 * time.Second},
		{"custom 60s", 60, 60 * time.Second},
		{"exactly 120s", 120, 120 * time.Second},
		{"above max is capped at 120s", 200, 120 * time.Second},
		{"1s minimum valid", 1, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTimeout(tt.input)
			if got != tt.expected {
				t.Errorf("resolveTimeout(%d) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
