// SPDX-License-Identifier: AGPL-3.0-or-later

package config

import (
	"os"
	"testing"
)

// TestLoadRateLimitDefault verifies RATE_LIMIT defaults to 60 when unset.
func TestLoadRateLimitDefault(t *testing.T) {
	if orig, ok := os.LookupEnv("RATE_LIMIT"); ok {
		os.Unsetenv("RATE_LIMIT")
		t.Cleanup(func() { os.Setenv("RATE_LIMIT", orig) })
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.RateLimit != 60 {
		t.Fatalf("RateLimit = %d, want 60", cfg.RateLimit)
	}
}

// TestLoadRateLimitDisabled verifies RATE_LIMIT=0 disables rate limiting.
func TestLoadRateLimitDisabled(t *testing.T) {
	t.Setenv("RATE_LIMIT", "0")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.RateLimit != 0 {
		t.Fatalf("RateLimit = %d, want 0 (disabled)", cfg.RateLimit)
	}
}
