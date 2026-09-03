// SPDX-License-Identifier: AGPL-3.0-or-later

package handler

import (
	"testing"

	"github.com/Anakin-Inc/anakinscraper-oss/server/internal/models"
)

// A domain config with handlerChain ["browser"] is the documented way to force
// browser-only scraping (docs/domain-configs.md), and the same config can carry a
// customUserAgent and customHeaders. The processor puts both on the handler request,
// so the browser context has to pick them up — otherwise those two fields are a no-op
// for exactly the domains that most need them.
func TestBrowserHandler_ContextOptionsApplyRequestOverrides(t *testing.T) {
	req := &models.HandlerRequest{
		URL:             "https://example.com",
		CustomUserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36",
		CustomHeaders:   map[string]string{"Cookie": "session=abc", "Accept-Language": "en-US"},
	}

	opts := contextOptions(req)

	if opts.UserAgent == nil {
		t.Fatal("custom user-agent was dropped")
	}
	if *opts.UserAgent != req.CustomUserAgent {
		t.Errorf("user-agent = %q, want %q", *opts.UserAgent, req.CustomUserAgent)
	}

	if len(opts.ExtraHttpHeaders) != len(req.CustomHeaders) {
		t.Fatalf("headers = %v, want %v", opts.ExtraHttpHeaders, req.CustomHeaders)
	}
	for k, want := range req.CustomHeaders {
		if got := opts.ExtraHttpHeaders[k]; got != want {
			t.Errorf("header %q = %q, want %q", k, got, want)
		}
	}
}

// The headers map must be a copy: the request's map comes straight from the shared
// domain config cache entry, which is handed to every concurrent job for that domain.
func TestBrowserHandler_ContextOptionsCopiesHeaders(t *testing.T) {
	headers := map[string]string{"Cookie": "session=abc"}
	req := &models.HandlerRequest{URL: "https://example.com", CustomHeaders: headers}

	opts := contextOptions(req)
	opts.ExtraHttpHeaders["Cookie"] = "tampered"

	if headers["Cookie"] != "session=abc" {
		t.Errorf("the caller's header map was mutated: %v", headers)
	}
}

// A request with no overrides must produce empty options, so the browser keeps its
// own realistic Camoufox fingerprint instead of being pinned to a blank user-agent.
func TestBrowserHandler_ContextOptionsEmptyWithoutOverrides(t *testing.T) {
	opts := contextOptions(&models.HandlerRequest{URL: "https://example.com"})

	if opts.UserAgent != nil {
		t.Errorf("user-agent = %q, want unset", *opts.UserAgent)
	}
	if opts.ExtraHttpHeaders != nil {
		t.Errorf("headers = %v, want unset", opts.ExtraHttpHeaders)
	}
}
