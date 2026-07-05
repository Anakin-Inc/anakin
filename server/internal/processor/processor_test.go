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
		{"blocked status", "request failed: 403 Forbidden", "blocked"},
		{"blocked captcha", "content validation: captcha detected", "blocked"},
		{"connection refused", "dial tcp 1.2.3.4:443: connection refused", "connection_refused"},
		{"dns failure", "dial tcp: lookup example.com: no such host", "dns_failure"},
		{"browser crash", "playwright: browser closed unexpectedly", "browser_crash"},
		{"parse error", "failed to parse response: unexpected end of JSON", "parse_error"},
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
