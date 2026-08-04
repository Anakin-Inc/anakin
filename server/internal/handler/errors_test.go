// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// The HTTP handler previously returned fmt.Errorf("HTTP %d: %s", code,
// http.StatusText(code)). StatusError replaces it and must render identically
// so error strings surfaced to API clients and logs do not change.
func TestStatusError_MessageIsUnchanged(t *testing.T) {
	for _, code := range []int{403, 404, 429, 500} {
		want := fmt.Sprintf("HTTP %d: %s", code, http.StatusText(code))
		got := (&StatusError{Code: code}).Error()
		if got != want {
			t.Errorf("StatusError{%d}.Error() = %q, want %q", code, got, want)
		}
	}
}

func TestIsBlockStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
		why  string
	}{
		{http.StatusForbidden, true, "403 is an outright refusal"},
		{http.StatusTooManyRequests, true, "429 means this exit IP is hot"},
		{http.StatusUnavailableForLegalReasons, true, "451 means the exit country is wrong"},
		{http.StatusServiceUnavailable, false, "503 is usually a real outage, not a block"},
		{http.StatusNotFound, false, "404 is about the URL, not the proxy"},
		{http.StatusInternalServerError, false, "500 is transient"},
		{http.StatusUnauthorized, false, "401 is about credentials, not the proxy"},
		{http.StatusOK, false, "200 is not a failure at all"},
	}
	for _, tt := range tests {
		if got := IsBlockStatus(tt.code); got != tt.want {
			t.Errorf("IsBlockStatus(%d) = %v, want %v — %s", tt.code, got, tt.want, tt.why)
		}
	}
}

// Chain.Execute aggregates handler failures into a single wrapped error. The
// status code has to survive that wrapping, otherwise the proxy pool cannot
// tell a block apart from a timeout.
func TestChain_StatusErrorSurvivesWrapping(t *testing.T) {
	chain := NewChain([]ScrapingHandler{
		&mockHandler{name: "http", canHandle: true, healthy: true,
			err: &StatusError{Code: http.StatusForbidden}},
	})

	_, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("Execute returned nil error for a failing handler")
	}

	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("errors.As could not recover *StatusError from %v", err)
	}
	if se.Code != http.StatusForbidden {
		t.Errorf("recovered code = %d, want 403", se.Code)
	}
}

// A non-status failure (timeout, DNS, connection refused) must not be mistaken
// for a block.
func TestChain_NonStatusErrorIsNotAStatusError(t *testing.T) {
	chain := NewChain([]ScrapingHandler{
		&mockHandler{name: "http", canHandle: true, healthy: true,
			err: errors.New("dial tcp: i/o timeout")},
	})

	_, err := chain.Execute(context.Background(), &models.HandlerRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("Execute returned nil error for a failing handler")
	}

	var se *StatusError
	if errors.As(err, &se) {
		t.Errorf("errors.As matched *StatusError on a plain error: %v", err)
	}
}
