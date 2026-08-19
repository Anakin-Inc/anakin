// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestIsBlockedErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"plain error", errors.New("connection refused"), false},
		{"403 status", &StatusError{Handler: "http", StatusCode: 403}, true},
		{"429 status", &StatusError{Handler: "http", StatusCode: 429}, true},
		{"404 status", &StatusError{Handler: "http", StatusCode: 404}, false},
		{"500 status", &StatusError{Handler: "api", StatusCode: 500}, false},
		{"wrapped 403 (chain style)", fmt.Errorf("all handlers failed: %w", &StatusError{Handler: "http", StatusCode: 403}), true},
		{"wrapped 500", fmt.Errorf("all handlers failed: %w", &StatusError{Handler: "http", StatusCode: 500}), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBlockedErr(tt.err); got != tt.want {
				t.Errorf("IsBlockedErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestStatusErrorMessageContainsCode(t *testing.T) {
	// hostedHint and logs match on the numeric code in the message, so the
	// typed error must keep the status code in its text.
	err := &StatusError{Handler: "http", StatusCode: 403}
	if got := err.Error(); !strings.Contains(got, "403") {
		t.Errorf("error message %q does not contain %q", got, "403")
	}
}
