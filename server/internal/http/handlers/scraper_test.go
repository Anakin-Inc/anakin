// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"testing"
	"time"
)

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
