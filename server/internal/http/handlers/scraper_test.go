// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"context"
	"testing"
	"time"
)

func TestResolveSyncTimeout(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected time.Duration
	}{
		{"zero uses default", 0, defaultSyncTimeout},
		{"negative uses default", -5, defaultSyncTimeout},
		{"custom 60s", 60, 60 * time.Second},
		{"exactly 120s", 120, maxSyncTimeout},
		{"above max is capped at 120s", 200, maxSyncTimeout},
		{"1s minimum valid", 1, 1 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSyncTimeout(tt.input)
			if got != tt.expected {
				t.Errorf("resolveSyncTimeout(%d) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	h := &ScraperHandler{
		allowPrivateTargets: true,
	}
	ctx := context.Background()

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"empty url", "", true},
		{"invalid format", "://missing-scheme", true},
		{"ftp scheme", "ftp://example.com/file", true},
		{"file scheme", "file:///etc/passwd", true},
		{"missing host", "http:///path", true},
		{"valid http", "http://example.com", false},
		{"valid https", "https://example.com/test?query=1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.validateURL(ctx, tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}
