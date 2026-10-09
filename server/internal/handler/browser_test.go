// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"context"
	"testing"
)

func TestRequestGuard_Allowed(t *testing.T) {
	g := &requestGuard{ctx: context.Background(), check: hostGuard, seen: map[string]error{}}

	blocked := []string{
		"http://127.0.0.1:8080/health",
		"http://localhost/",
		"http://169.254.169.254/latest/meta-data/",
		"https://10.0.0.5/",
		"http://[::1]:9222/",
		"ws://192.168.1.1/socket",
		"file:///etc/passwd",
		"gopher://8.8.8.8/",
	}
	for _, u := range blocked {
		if err := g.allowed(u); err == nil {
			t.Errorf("allowed(%q) = nil, want an error", u)
		}
	}

	allowed := []string{
		"http://8.8.8.8/",
		"https://93.184.216.34/page",
		"wss://8.8.8.8/socket",
		"data:text/html,hi",
		"about:blank",
		"blob:https://8.8.8.8/0b6f",
	}
	for _, u := range allowed {
		if err := g.allowed(u); err != nil {
			t.Errorf("allowed(%q) = %v, want nil", u, err)
		}
	}
}
