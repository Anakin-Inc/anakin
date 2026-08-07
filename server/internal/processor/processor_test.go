// SPDX-License-Identifier: AGPL-3.0-or-later

package processor

import "testing"

func TestCategorizeError(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want string
	}{
		{"timeout", "context deadline exceeded (Client.Timeout)", "timeout"},
		{"rate limited", "unexpected status 429 Too Many Requests", "rate_limited"},
		{"api rate limited", "anakin API returned HTTP 429", "rate_limited"},
		{"blocked status", "request failed: 403 Forbidden", "blocked"},
		{"api blocked", "anakin API returned HTTP 403", "blocked"},
		{"blocked captcha", "content validation: captcha detected", "blocked"},
		{"admin blocked", "domain is blocked: manual blocklist", "blocked"},
		{"connection refused", "dial tcp 1.2.3.4:443: connection refused", "connection_refused"},
		{"dns failure", "dial tcp: lookup example.com: no such host", "dns_failure"},
		{"browser crash", "playwright: browser closed unexpectedly", "browser_crash"},
		{"browser service down", "failed to connect to browser at ws://localhost:9222/camoufox: dial tcp: connect: connection refused", "browser_crash"},
		{"browser nav timeout", "navigation failed: Timeout 60000ms exceeded", "timeout"},
		{"browser nav dns", "navigation failed: net::ERR_NAME_NOT_RESOLVED", "dns_failure"},
		{"browser nav generic", "navigation failed: net::ERR_ABORTED", "browser_crash"},
		{"parse error", "failed to parse response: unexpected end of JSON", "parse_error"},
		{"json syntax error", "invalid character '<' looking for beginning of value", "parse_error"},
		{"url token not misbucketed", `HTTP request failed: Get "http://cdns.example.com/": dial tcp: connection refused`, "connection_refused"},
		{"url keyword not misbucketed", `HTTP request failed: Get "http://site.com/forbidden": dial tcp: no such host`, "dns_failure"},
		{"unknown", "something entirely unexpected happened", "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := categorizeError(tc.msg); got != tc.want {
				t.Errorf("categorizeError(%q) = %q, want %q", tc.msg, got, tc.want)
			}
		})
	}
}
