// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"fmt"
	"net/http"
)

// StatusError is returned by a handler when the target responded with an HTTP
// status that prevents a successful scrape.
//
// It carries the status code through the error path so callers can tell an
// outright refusal apart from a transient failure. Chain.Execute wraps handler
// errors with %w, so errors.As recovers a *StatusError from the aggregated
// "all handlers failed" error.
type StatusError struct {
	Code int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.Code, http.StatusText(e.Code))
}

// IsBlockStatus reports whether code means the target refused the request
// outright, rather than failing in a way that might succeed on a retry.
//
// This is deliberately narrow. A false positive costs a healthy proxy a
// severePenalty score hit plus a five-minute exclusion for that host, so only
// statuses that are unambiguous refusals qualify. In particular 503 is
// excluded: it is far more often a genuine upstream outage — which any proxy
// would have hit — than an anti-bot interstitial.
func IsBlockStatus(code int) bool {
	switch code {
	case http.StatusForbidden, // 403 — outright refusal, covers most WAF blocks
		http.StatusTooManyRequests,            // 429 — rate limited, this exit IP is hot
		http.StatusUnavailableForLegalReasons: // 451 — geo/legal block, wrong exit country
		return true
	}
	return false
}
