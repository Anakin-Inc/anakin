// SPDX-License-Identifier: AGPL-3.0-or-later

package processor

import (
	"testing"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// buildHandlerRequest used to hardcode Timeout to 60s. Nothing read the field,
// so it was inert — but once Chain.Execute honours it, a non-zero default would
// silently cap every attempt at 60s, including for users who run with a larger
// BROWSER_TIMEOUT and no domain config at all.
//
// Zero must mean "no per-attempt override".
func TestBuildHandlerRequest_LeavesTimeoutUnset(t *testing.T) {
	p := &Processor{}
	req := p.buildHandlerRequest(models.JobMessage{
		JobID:      "job-1",
		URL:        "https://example.com",
		Country:    "us",
		UseBrowser: true,
	}, "https://example.com")

	if req.Timeout != 0 {
		t.Errorf("Timeout = %v, want 0 (no override unless a domain config sets requestTimeoutMs)", req.Timeout)
	}

	// The rest of the mapping should be untouched.
	if req.JobID != "job-1" {
		t.Errorf("JobID = %q, want %q", req.JobID, "job-1")
	}
	if req.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", req.URL, "https://example.com")
	}
	if req.Country != "us" {
		t.Errorf("Country = %q, want %q", req.Country, "us")
	}
	if !req.UseBrowser {
		t.Error("UseBrowser = false, want true")
	}
}
